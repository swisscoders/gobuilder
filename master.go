package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"./exec"
	pb "./proto"
	"./utils/proto/any"
)

var address = flag.String("address", ":3900", "Address of the RPC service")
var config = flag.String("config", "", "Path to the config file")
var certFile = flag.String("cert_file", "", "Path to the TLS certificate file")
var keyFile = flag.String("key_file", "", "Path to the TLS key file")
var logDir = flag.String("build_log_base_dir", "", "Path to log directory")
var webViewerUrl = flag.String("webviewer", "", "URL of the webviewer (ie. http://mywebviewer:8080)")

var (
	notificationErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gobuilder",
		Subsystem: "notification",
		Name:      "errors",
		Help:      "Number of failed notifications",
	},
		[]string{"project", "builder"})

	notificationTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gobuilder",
		Subsystem: "notification",
		Name:      "total",
		Help:      "Number of total notifications handled",
	},
		[]string{"project", "builder"})

	schedulerErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gobuilder",
		Subsystem: "scheduler",
		Name:      "errors",
		Help:      "Number of scheduler errors",
	},
		[]string{"project"})

	schedulerTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gobuilder",
		Subsystem: "scheduler",
		Name:      "requests",
		Help:      "Total schedules",
	},
		[]string{"project"})

	reportProcessingErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gobuilder",
		Subsystem: "report",
		Name:      "errors",
		Help:      "Errors while processing reports",
	},
		[]string{"project"})

	reportProcessingTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gobuilder",
		Subsystem: "report",
		Name:      "total",
		Help:      "Total reports processed",
	},
		[]string{"project"})
)

func init() {
	prometheus.MustRegister(notificationErrors)
	prometheus.MustRegister(notificationTotal)

	prometheus.MustRegister(schedulerErrors)
	prometheus.MustRegister(schedulerTotal)

	prometheus.MustRegister(reportProcessingErrors)
	prometheus.MustRegister(reportProcessingTotal)
}

type SourceNotifier interface {
	Start() error
	Report() <-chan *pb.ChangeRequest
	Notify(ctx context.Context, req *pb.ChangeRequest) (*pb.ChangeResponse, error)
	Stop() error
}

type rpcSourceNotifier struct {
	reporter chan *pb.ChangeRequest
}

func NewRpcSourceNotifier() *rpcSourceNotifier {
	return new(rpcSourceNotifier)
}

func (self *rpcSourceNotifier) Notify(ctx context.Context, req *pb.ChangeRequest) (*pb.ChangeResponse, error) {
	self.reporter <- req
	return &pb.ChangeResponse{}, nil
}

func (self *rpcSourceNotifier) Start() error {
	self.reporter = make(chan *pb.ChangeRequest, 100)
	return nil
}

func (self *rpcSourceNotifier) Stop() error {
	close(self.reporter)
	return nil
}

func (self *rpcSourceNotifier) Report() <-chan *pb.ChangeRequest {
	return self.reporter
}

type Scheduler interface {
	// Returns true if this change should be scheduled
	ShouldSchedule(change *pb.ChangeRequest) bool

	// Schedules the actual work (according to the internals)
	Schedule(change *pb.ChangeRequest) error
}

type BuildSlave interface {
	Name() string

	Acquire()
	Setup() (*pb.BuildEnv, error)
	Execute(bp *pb.Blueprint, change *pb.ChangeRequest, ctx *ExecutionContext) error
	Teardown(env *pb.BuildEnv) error
	Release()
}

type BuildSlaveRegistry struct {
	registry map[string]BuildSlave
	mu       sync.RWMutex
}

func NewBuildSlaveRegistry() *BuildSlaveRegistry {
	return &BuildSlaveRegistry{registry: make(map[string]BuildSlave)}
}

func (self *BuildSlaveRegistry) Add(name string, slave BuildSlave) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	// TODO(rn): Make liveness probe (reregister?)
	if _, has := self.registry[name]; has {
		return fmt.Errorf("Builder with name %s already exists", name)
	}
	self.registry[name] = slave
	return nil
}

// Finds a builder with given name.
// Returns nil if none is found.
func (self *BuildSlaveRegistry) Find(name string) BuildSlave {
	self.mu.RLock()
	defer self.mu.RUnlock()

	return self.registry[name]
}

type rpcBuildSlave struct {
	mu      sync.Mutex
	locked  bool
	address string
	conn    *grpc.ClientConn
	client  pb.BuilderClient

	name string
}

func newRpcBuildSlave(name, address string) *rpcBuildSlave {
	return &rpcBuildSlave{name: name, address: address}
}

func (self *rpcBuildSlave) Name() string {
	return self.name
}

func (self *rpcBuildSlave) Acquire() {
	self.mu.Lock()
	self.locked = true
}

func (self *rpcBuildSlave) Release() {
	self.locked = false
	self.mu.Unlock()
}

func (self *rpcBuildSlave) connect() (err error) {
	// TODO(rn): Create TLS auth
	self.conn, err = grpc.Dial(self.address)
	if err != nil {
		return err
	}
	self.client = pb.NewBuilderClient(self.conn)
	return
}

func (self *rpcBuildSlave) replaceVariables(argv []string, change *pb.ChangeRequest) (newArgv []string) {
	vars := make(map[string]string)
	vars["project"] = change.Project
	vars["repo"] = change.Repo
	vars["commit"] = change.Commit
	vars["branch"] = change.Branch

	for _, arg := range argv {
		newArgv = append(newArgv, os.Expand(arg, func(variable string) string {
			if v, has := vars[variable]; has {
				return v
			}

			return "${" + variable + "}"
		}))
	}
	return
}

func (self *rpcBuildSlave) Setup() (*pb.BuildEnv, error) {
	if self.conn == nil {
		if err := self.connect(); err != nil {
			return nil, err
		}
	}
	cmd := new(exec.RemoteCmd)
	return cmd.Setup(self.client)
}

func (self *rpcBuildSlave) Teardown(env *pb.BuildEnv) error {
	if self.conn == nil {
		if err := self.connect(); err != nil {
			return err
		}
	}
	cmd := new(exec.RemoteCmd)
	return cmd.Teardown(self.client, env)
}

func (self *rpcBuildSlave) Execute(bp *pb.Blueprint, change *pb.ChangeRequest, ctx *ExecutionContext) (err error) {
	if self.conn == nil {
		err = self.connect()
		if err != nil {
			return err
		}
	}

	if !self.locked {
		return errors.New("Slave hasn't been acquired before execution of commands")
	}

	for _, step := range bp.Step {
		cmd := new(exec.RemoteCmd)
		cmd.Args = self.replaceVariables(step.Argv, change)
		cmd.Env = self.replaceVariables(step.Env, change)

		cmd.Stdout = ctx.Stdout
		cmd.Stderr = ctx.Stderr
		cmd.BuildEnv = ctx.BuildEnv
		err = cmd.Run(self.client)
		ctx.Status = cmd.Status

		if err != nil && err != io.EOF {
			return fmt.Errorf("Failed to execute command %s: %s", strings.Join(cmd.Args, ", "), err)
		}
	}
	return
}

type Notifier interface {
	Notify(change *pb.ChangeRequest, results []*pb.BuildResult) error
}

type nullNotifier struct {
}

func (self *nullNotifier) Notify(change *pb.ChangeRequest, results []*pb.BuildResult) error {
	return nil
}

func NewNullNotifier() *nullNotifier {
	return &nullNotifier{}
}

type emailNotifier struct {
	smtp string
	from string
	auth smtp.Auth
	cc   []mail.Address

	webViewerUrl string
}

var mailTmpl = template.Must(template.New("mailtmpl").Parse(`Dear {{.Change.Name}}

It appears as if you broke one or more slave in {{.Change.Project}} with {{.Change.Commit}}:
{{range $result := .Results}}- {{$result.Slave}}
{{end}}
More details: {{.URL}}/project/{{.Change.Project}}`))

func (self *emailNotifier) Notify(change *pb.ChangeRequest, results []*pb.BuildResult) error {
	// TODO(rn): Is AuthoEmail []string? (how are merges of multiple commits handled?)

	var AllBuildsSuccessful bool = true
	for _, result := range results {
		if result.Status.ExitStatus != 0 || result.Status.CoreDump {
			AllBuildsSuccessful = false
		}
	}

	if AllBuildsSuccessful {
		return nil
	}

	m := NewEmail(mailTmpl)
	m.SetTo(mail.Address{Name: change.Name, Address: change.Email})

	m.SetCc(self.cc...)
	m.SetSubject(fmt.Sprintf("%s: %s broke some builds", change.Project, change.Commit))
	to := append(self.cc, mail.Address{Name: change.Name, Address: change.Email})
	msg, err := m.Construct(struct {
		Change  *pb.ChangeRequest
		Results []*pb.BuildResult
		URL     string
	}{change, results, self.webViewerUrl})
	if err != nil {
		return err
	}

	var tos []string
	for _, mail := range to {
		tos = append(tos, mail.Address)
	}

	if err := smtp.SendMail(self.smtp, self.auth, self.from, tos, msg); err != nil {
		return fmt.Errorf("Sending mail using %s to %v: %s", self.smtp, to, err)
	}
	return nil
}

type Email struct {
	header mail.Header
	tmpl   *template.Template
}

func NewEmail(t *template.Template) *Email {
	return &Email{tmpl: t, header: mail.Header{}}
}

func (self *Email) SetFrom(from mail.Address) {
	self.header["From"] = []string{from.String()}
}

func (self *Email) SetTo(to ...mail.Address) {
	var tos = self.header["To"]
	for _, t := range to {
		tos = append(tos, t.String())
	}
	self.header["To"] = tos
}

func (self *Email) SetCc(cc ...mail.Address) {
	var ccs = self.header["Cc"]
	for _, c := range cc {
		ccs = append(ccs, c.String())
	}
	self.header["Cc"] = ccs
}

func mailAddrFromStringSlice(mailAddrs []string) (newMailAddrs []mail.Address) {
	for _, email := range mailAddrs {
		newMailAddrs = append(newMailAddrs, mail.Address{Address: email})
	}
	return
}

func (self *Email) SetSubject(subject string) {
	self.header["Subject"] = []string{subject}
}

func (self *Email) fillHeaders() {
	self.addIfNotExist("Date", time.Now().Format(time.RFC822))
	self.addIfNotExist("Content-Type", `text/plain; charset="utf-8"`)
	self.addIfNotExist("User-Agent", "GoBuilder")
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}
	self.addIfNotExist("Message-ID", fmt.Sprintf("%x.%d@%s", time.Now().UnixNano(), os.Getpid(), host))
}

func (self *Email) addIfNotExist(name, value string) {
	if _, ok := self.header[name]; ok {
		return
	}
	self.header[name] = []string{value}
}

func (self *Email) Construct(data interface{}) ([]byte, error) {
	var msg bytes.Buffer

	for key, values := range self.header {
		msg.WriteString(key)
		msg.WriteString(": ")
		msg.WriteString(strings.Join(values, ", "))
		msg.WriteString("\r\n")
	}
	msg.WriteString("\r\n")

	if err := mailTmpl.Execute(&msg, data); err != nil {
		return nil, fmt.Errorf("While executing template for message body: %s", err)
	}
	return msg.Bytes(), nil
}

type ExecutionContext struct {
	Stdout   io.WriteCloser
	Stderr   io.WriteCloser
	Status   *pb.Status
	BuildEnv *pb.BuildEnv
}

type Builder interface {
	Build(change *pb.ChangeRequest) error
}

type builderImpl struct {
	slaves    []BuildSlave
	blueprint *pb.Blueprint

	project string
	builder string

	logDir string

	store    BuildResultStorer
	notifier Notifier
}

type BuildResultStorer interface {
	StoreMeta(meta *pb.MetaResult) error
	StoreResult(key []byte, res *pb.BuildResult) error
	GetResult(keyPrefix []byte) (*pb.GetResultResponse, error)
	ResultKey(builder string, slave string, timestamp time.Time) []byte
}

type ldbResultStorage struct {
	db *leveldb.DB
}

func (self *ldbResultStorage) Close() {
	self.db.Close()
}

func newLdbResultStorage(fullPath string) *ldbResultStorage {
	db, err := leveldb.OpenFile(path.Join(fullPath, "buildresult.db"), nil)
	if err != nil {
		panic(err)
	}
	return &ldbResultStorage{db: db}
}

func (self *ldbResultStorage) ResultKey(builder, slave string, timestamp time.Time) []byte {
	var key bytes.Buffer
	key.WriteString(builder)
	key.WriteString("_")
	key.WriteString(slave)
	key.WriteString("_")
	key.WriteString(strconv.Itoa(int(timestamp.UnixNano())))

	return key.Bytes()
}

func (self *ldbResultStorage) StoreMeta(meta *pb.MetaResult) (err error) {
	v, err := proto.Marshal(meta)
	if err != nil {
		return fmt.Errorf("Error while marshalling the meta result proto: %s", err)
	}

	return self.db.Put([]byte("_meta"), v, nil)
}

func (self *ldbResultStorage) StoreResult(key []byte, res *pb.BuildResult) (err error) {
	v, err := proto.Marshal(res)
	if err != nil {
		return fmt.Errorf("Error while marshalling the build result proto: %s", err)
	}

	return self.db.Put(key, v, nil)
}

func (self *ldbResultStorage) GetResult(keyPrefix []byte) (*pb.GetResultResponse, error) {
	res := new(pb.GetResultResponse)
	it := self.db.NewIterator(util.BytesPrefix(keyPrefix), nil)
	defer it.Release()

	it.Last()
	for {
		k := make([]byte, len(it.Key()))
		copy(k, it.Key())
		res.Key = append(res.Key, k)

		br := new(pb.BuildResult)
		if err := proto.Unmarshal(it.Value(), br); err != nil {
			return nil, fmt.Errorf("While unmarshaling proto for key record %s: %s", string(it.Key()), err)
		}
		res.Result = append(res.Result, br)

		if !it.Prev() {
			break
		}
	}
	if it.Error() != nil {
		return nil, fmt.Errorf("Iteration over %s prefix failed: %s", string(keyPrefix), it.Error())
	}
	return res, nil
}

func (self *builderImpl) Build(change *pb.ChangeRequest) error {
	var wg sync.WaitGroup
	var errs []error
	var results []*pb.BuildResult

	builderLogDir := path.Join(self.logDir, self.project, self.builder)
	if err := os.MkdirAll(builderLogDir, 0750); err != nil {
		return fmt.Errorf("Failed to create logdir %s: %s", logDir, err)
	}

	for _, s := range self.slaves {
		wg.Add(1)
		go func(s BuildSlave, bp *pb.Blueprint) {
			defer wg.Done()

			s.Acquire()
			defer s.Release()

			start := time.Now()
			buildEnv, err := s.Setup()
			if err != nil {
				errs = append(errs, fmt.Errorf("Slave %s failed to setup build environment: %s", s.Name(), err))
				return
			}

			defer func() {
				if err := s.Teardown(buildEnv); err != nil {
					errs = append(errs, fmt.Errorf("Slave %s failed to teardown the build environment: %s", s.Name(), err))
				}
			}()

			// Create the stdout/stderr per slave
			stdout, stderr, err := self.createOutputPair(builderLogDir, fmt.Sprintf("%s.%s", change.Commit, s.Name()))
			if err != nil {
				errs = append(errs, fmt.Errorf("Unable to create the output pair for logging: %s", err))
				return
			}
			defer stdout.Close()
			defer stderr.Close()

			ctx := &ExecutionContext{Stdout: stdout, Stderr: stderr, BuildEnv: buildEnv}
			if err := s.Execute(bp, change, ctx); err != nil && err != io.EOF {
				errs = append(errs, fmt.Errorf("While executing on slave %s: %s", s.Name(), err))
			}
			duration := time.Since(start)

			result := &pb.BuildResult{
				Duration:      duration.Nanoseconds(),
				ChangeRequest: change,
				Status:        ctx.Status,
				Builder:       self.builder,
				Slave:         s.Name(),
				Timestamp:     start.Unix(),
			}
			results = append(results, result)
			self.store.StoreResult(self.store.ResultKey(self.builder, s.Name(), start), result)

		}(s, self.blueprint)
	}
	wg.Wait()

	notificationTotal.WithLabelValues(self.project, self.builder).Inc()
	if err := self.notifier.Notify(change, results); err != nil {
		glog.Errorf("Failed to send notification: %s", err)
		notificationErrors.WithLabelValues(self.project, self.builder).Inc()
	}

	if len(errs) > 0 {
		return fmt.Errorf("Build slave failures: %v", errs)
	}
	return nil
}

func (self *builderImpl) createOutputPair(fullPath, prefix string) (stdout, stderr io.WriteCloser, err error) {
	stdout, err = os.Create(path.Join(fullPath, prefix+".stdout.log"))
	if err != nil {
		err = fmt.Errorf("Failed to create file for stdout: %s", err)
		return
	}

	stderr, err = os.Create(path.Join(fullPath, prefix+".stderr.log"))
	if err != nil {
		err = fmt.Errorf("Failed to create file for stderr: %s", err)
		return
	}

	return stdout, stderr, nil
}

type PeriodicScheduler struct {
	builder []Builder
	config  *pb.PeriodicScheduler

	change chan *pb.ChangeRequest
}

func (self *PeriodicScheduler) ShouldSchedule(change *pb.ChangeRequest) bool {
	return true
}

func (self *PeriodicScheduler) Schedule(change *pb.ChangeRequest) error {
	self.change <- change
	return nil
}

func (self *PeriodicScheduler) executionLoop(interval int32) {
	var latestRequest *pb.ChangeRequest
	ticker := time.NewTicker(time.Second * time.Duration(interval))
	for {
		select {
		case <-ticker.C:
			if latestRequest == nil {
				// Nothing to do
				continue
			}
			glog.Info("Run latest scheduled change")

			if err := runBuilder(self.builder, latestRequest); err != nil && err != io.EOF {
				glog.Errorf("Failed to run builders: %s", err)
			}
			// Even if the above run wasn't successful, until we can
			// distengiuesh between internal and external error we should not
			// keep the request around to avoid running endlessly.
			// We should rather relay on good retry behaviour in runBuilder
			latestRequest = nil

		case latestRequest = <-self.change:
			glog.Info("Received new change")
		}
	}
}

func runBuilder(builder []Builder, change *pb.ChangeRequest) (err error) {
	var wg sync.WaitGroup
	var errs []error

	for _, b := range builder {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := b.Build(change); err != nil {
				errs = append(errs, err)
			}
		}()
	}
	wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("Some builder have failed: %v", errs)
	}

	return
}

func NewPeriodicScheduler(builder []Builder, config *pb.PeriodicScheduler) Scheduler {
	s := &PeriodicScheduler{builder: builder, config: config, change: make(chan *pb.ChangeRequest)}
	go s.executionLoop(config.Interval)
	return s
}

// Map of scheduler instances from project
type SchedulerMap map[string][]Scheduler

type reportProcessor struct {
	reporter  <-chan *pb.ChangeRequest
	config    *pb.GoBuilder
	scheduler SchedulerMap
}

func NewReportProcessor(config *pb.GoBuilder, reporter <-chan *pb.ChangeRequest, scheduler SchedulerMap) *reportProcessor {
	return &reportProcessor{reporter: reporter, config: config, scheduler: scheduler}
}

func (self *reportProcessor) Process() {
	for rep := range self.reporter {
		reportProcessingTotal.WithLabelValues(rep.Project).Inc()
		_, ok := self.findProject(rep.Project)
		if !ok {
			glog.Error("Received report for unknown project: %s", rep.Project)
			reportProcessingErrors.WithLabelValues(rep.Project).Inc()
		}
		glog.Infof("Process report for project %s", rep.Project)

		for i, si := range self.scheduler[rep.Project] {
			if si.ShouldSchedule(rep) {
				glog.Infof("Scheduler %d wants to schedule change", i)
				go func(change *pb.ChangeRequest) {
					schedulerTotal.WithLabelValues(rep.Project).Inc()
					if err := si.Schedule(change); err != nil {
						glog.Errorf("Failed to schedule on scheduler %d in project %s: %s", i, rep.Project, err)
						schedulerErrors.WithLabelValues(rep.Project).Inc()
					}
				}(rep)
			} else {
				glog.Infof("Scheduler %d refused to schedule change", i)
			}
		}
	}
}

func (self *reportProcessor) findProject(proj string) (*pb.Project, bool) {
	// Maybe consider using sort and binary search, but there should be sufficient few projects defined anyway
	for _, p := range self.config.GetProject() {
		if p.Name == proj {
			return p, true
		}
	}
	return nil, false
}

func findBlueprint(blueprints []*pb.Blueprint, name string) (result *pb.Blueprint) {
	// Maybe consider using sort and binary search, but there should be sufficient few blueprints defined anyway
	for _, bp := range blueprints {
		if name == bp.Name {
			return bp
		}
	}
	return nil
}

type resultServer struct {
	DBs map[string]BuildResultStorer
}

func newResultServer() *resultServer {
	return &resultServer{DBs: make(map[string]BuildResultStorer)}
}

func (self *resultServer) GetResult(ctx context.Context, req *pb.GetResultRequest) (*pb.GetResultResponse, error) {
	db, ok := self.DBs[req.Project]
	if !ok {
		return nil, fmt.Errorf("No such project %s", req.Project)
	}
	return db.GetResult(req.KeyPrefix)
}

func setupFromConfigOrDie(conf *pb.GoBuilder, server *grpc.Server, registry *BuildSlaveRegistry, logDir string, webViewerUrl string) (processor *reportProcessor, cleanupClosure func()) {
	var sourceNotifier *rpcSourceNotifier
	var schedulerMap = make(SchedulerMap)
	var cleanupFns []func()
	cleanupCb := func() {
		if cleanupFns == nil {
			return
		}

		for _, f := range cleanupFns {
			f()
		}
	}

	for _, s := range conf.GetSlave() {
		registry.Add(s.Name, newRpcBuildSlave(s.Name, s.Address))
	}

	resultSrv := newResultServer()
	for _, proj := range conf.GetProject() {
		db := newLdbResultStorage(path.Join(logDir, proj.Name))
		resultSrv.DBs[proj.Name] = db
		cleanupFns = append(cleanupFns, db.Close)

		if proj.GetSource() != nil && any.IsType(proj.GetSource(), "projects.SourceNotifier") {
			sourceNotifier = NewRpcSourceNotifier()
			sourceNotifier.Start()

			sourceNotifierStopFn := func() {
				if err := sourceNotifier.Stop(); err != nil {
					glog.Errorf("Failed to stop source notifier: %s", err)
				}
			}

			pb.RegisterChangeSourceServer(server, sourceNotifier)

			cleanupFns = append(cleanupFns, sourceNotifierStopFn)
		} else {
			glog.Fatalf("No known source notifiers found for project: %s", proj.Name)
		}

		builder := make(map[string]Builder)

		for _, b := range proj.GetBuilder() {
			var slaves []BuildSlave
			for _, slaveName := range b.Slave {
				if sl := registry.Find(slaveName); sl == nil {
					glog.Fatalf("Slave %s referenced from builder %s in %s could not be found", slaveName, b.Name, proj.Name)
				} else {
					slaves = append(slaves, sl)
				}
			}
			blueprint := findBlueprint(proj.GetBlueprint(), b.Blueprint)
			if blueprint == nil {
				glog.Fatalf("Blueprint %s referenced in builder %s not found in project %s", b.Blueprint, b.Name, proj.Name)
			}

			var notifier Notifier = NewNullNotifier()
			switch {
			case any.IsType(b.GetNotifier(), "projects.EmailNotifier"):
				emailNotifierProto := new(pb.EmailNotifier)
				if err := any.UnpackTextTo(b.GetNotifier(), emailNotifierProto); err != nil {
					glog.Fatalf("Failed to unpack email notifier: %s", err)
				}

				smtpAddress := fmt.Sprintf("%s:%d", emailNotifierProto.GetSmtp().Address, emailNotifierProto.GetSmtp().Port)
				notifier = &emailNotifier{
					cc:           mailAddrFromStringSlice(emailNotifierProto.GetSmtp().Cc),
					smtp:         smtpAddress,
					from:         emailNotifierProto.From,
					auth:         smtp.PlainAuth("", emailNotifierProto.GetSmtp().Username, emailNotifierProto.GetSmtp().Password, emailNotifierProto.GetSmtp().Address),
					webViewerUrl: webViewerUrl,
				}
				glog.V(1).Infof("Notifier smtp (address=%s,from=%s) registered for builder: %s", smtpAddress, emailNotifierProto.From, b.Name)
			}

			builder[b.Name] = &builderImpl{logDir: logDir, slaves: slaves, blueprint: blueprint, project: proj.Name, builder: b.Name, store: db, notifier: notifier}

		}

		for i, sched := range proj.GetScheduler() {
			var builders []Builder
			for _, b := range sched.Builder {
				bi, ok := builder[b]
				if !ok {
					glog.Fatalf("Builder %s referenced from scheduler %d in %s could not be found", b, i, proj.Name)
				}
				builders = append(builders, bi)
			}
			switch {
			case any.IsType(sched.Scheduler, "projects.PeriodicScheduler"):
				schedProto := new(pb.PeriodicScheduler)
				if err := any.UnpackTextTo(sched.Scheduler, schedProto); err != nil {
					glog.Fatalf("Failed to load scheduler (Project=%s): %s", proj.Name, err)
				}

				schedulerMap[proj.Name] = append(schedulerMap[proj.Name], NewPeriodicScheduler(builders, schedProto))
			default:
				glog.Fatalf("%s defines unknown scheduler: %s", proj.Name, sched.Scheduler.TypeUrl)
			}
		}
	}

	pb.RegisterResultServer(server, resultSrv)

	return NewReportProcessor(conf, sourceNotifier.Report(), schedulerMap), cleanupCb
}

var tls = flag.Bool("tls", false, "Enable tls")

func main() {
	flag.Parse()

	if *config == "" {
		glog.Fatal("Need to specify --config")
	}
	conf := ReadConfigOrDie(*config)

	if err := ValidateConfig(conf); err != nil {
		glog.Fatalf("Error while validating config: %s", err)
	}

	// Verifying if the log directory exists:
	if exists, err := dirExists(*logDir); !exists || err != nil {
		glog.Fatalf("Log directory %s does not exist: %s", *logDir, err)
	}

	http.Handle("/varz", prometheus.Handler())
	go http.ListenAndServe(":8080", nil)

	registry := NewBuildSlaveRegistry()
	rpcServer := grpc.NewServer()
	processor, cleanupCb := setupFromConfigOrDie(conf, rpcServer, registry, *logDir, *webViewerUrl)
	defer cleanupCb()
	go processor.Process()

	lis, err := net.Listen("tcp", *address)
	if err != nil {
		glog.Fatalf("Failed to listen: %s", err)
	}

	if *tls {
		/*creds, err := credentials.NewServerTLSFromFile(*certFile, *keyFile)
		if err != nil {
			glog.Fatalf("Failed to load credentials from (Cert=%s,Key=%s): %s", *certFile, *keyFile, err)
		}
		rpcServer.Serve(creds.NewListener(lis))*/
	} else {
		rpcServer.Serve(lis)
	}
	return
}

type Entity interface {
	GetName() string
}

func buildEntitySet(entities []Entity) (map[string]struct{}, error) {
	entitySet := make(map[string]struct{})
	var duplicates []string
	for _, e := range entities {
		if _, has := entitySet[e.GetName()]; has {
			duplicates = append(duplicates, e.GetName())
		}
		entitySet[e.GetName()] = struct{}{}
	}
	if duplicates != nil {
		return nil, fmt.Errorf("Duplicate entry names: %s", strings.Join(duplicates, ", "))
	}
	return entitySet, nil
}

type blueprintEntity pb.Blueprint

func (self blueprintEntity) GetName() string {
	return self.Name
}

type builderEntity pb.Builder

func (self builderEntity) GetName() string {
	return self.Name
}

func ValidateConfig(conf *pb.GoBuilder) (err error) {
	if projs := conf.GetProject(); projs == nil || len(projs) == 0 {
		return fmt.Errorf("No project defined")
	}

	if ss := conf.GetSlave(); ss == nil || len(ss) == 0 {
		return errors.New("No build slaves defined")
	}

	for _, proj := range conf.GetProject() {
		bps := proj.GetBlueprint()
		if bps == nil || len(bps) == 0 {
			return fmt.Errorf("Project %s has no blueprints defined", proj.Name)
		}

		// Test for unique blueprint ids (per project)
		entitySet := make([]Entity, len(bps))
		for i, bp := range bps {
			entitySet[i] = blueprintEntity(*bp)
		}
		_, err := buildEntitySet(entitySet)
		if err != nil {
			return fmt.Errorf("While validating blueprints for %s: %s", proj.Name, err)
		}

		// Test for unique builder ids (per project)
		entitySet = make([]Entity, len(proj.GetBuilder()))
		for i, b := range proj.GetBuilder() {
			if b.Slave == nil || len(b.Slave) == 0 {
				return fmt.Errorf("Builder %s in project %s does not reference any slaves", b.Name, proj.Name)
			}
			entitySet[i] = builderEntity(*b)
		}
		builderNames, err := buildEntitySet(entitySet)
		if err != nil {
			return fmt.Errorf("While validating builders for %s: %s", proj.Name, err)
		}

		if scheds := proj.GetScheduler(); scheds == nil || len(scheds) == 0 {
			return fmt.Errorf("Project %s has no schedulers defined", proj.Name)
		} else {
			for i, sched := range scheds {
				if sched.Builder == nil || len(sched.Builder) == 0 {
					return fmt.Errorf("Scheduler %d in project %s does not reference any builder", i, proj.Name)
				}
				for _, b := range sched.Builder {
					if _, has := builderNames[b]; !has {
						return fmt.Errorf("Scheduler %d in project %s references unknown builder: %s", i, proj.Name, b)
					}
				}
			}
		}

		// Validating blueprints (empty blueprints will be rejected)
		for i, bp := range bps {
			if steps := bp.GetStep(); steps == nil || len(steps) == 0 {
				return fmt.Errorf("Blueprint %d named %s must contain at least one step (Project=%s)", i, bp.Name, proj.Name)
			} else {
				for i, step := range steps {
					if argvs := step.Argv; argvs == nil || len(argvs) == 0 {
						return fmt.Errorf("Step %d has no argv defined (Project=%s,Blueprint=%s)", i, proj.Name, bp.Name)
					}
				}
			}
		}

		if proj.GetSource() == nil {
			return fmt.Errorf("Project %s has no source defined", proj.Name)
		}
	}

	return nil
}

func ReadConfigOrDie(path string) *pb.GoBuilder {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		glog.Fatalf("Could not read file %s: %s", path, err)
	}

	var conf pb.GoBuilder
	err = proto.UnmarshalText(string(data), &conf)
	if err != nil {
		glog.Fatalf("Could not unmarshal config %s: %s", path, err)
	}
	return &conf
}

func dirExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return !os.IsNotExist(err), err
	}

	if !info.IsDir() {
		return false, fmt.Errorf("%s is not a directory", path)
	}

	return true, nil
}

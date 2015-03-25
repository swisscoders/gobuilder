package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"

	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"./exec"

	pb "./proto"
)

var address = flag.String("address", ":3900", "Address of the RPC service")
var config = flag.String("config", "", "Path to the config file")
var certFile = flag.String("cert_file", "", "Path to the TLS certificate file")
var keyFile = flag.String("key_file", "", "Path to the TLS key file")
var logDir = flag.String("build_log_base_dir", "", "Path to log directory")

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

// FIXME(an): Where do we call this function?
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

	for _, step := range bp.GetStep() {
		cmd := new(exec.RemoteCmd)
		cmd.Args = self.replaceVariables(step.GetArgv(), change)
		cmd.Env = self.replaceVariables(step.GetEnv(), change)

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

	store BuildResultStorer
}

type BuildResultStorer interface {
	StoreMeta(meta *pb.MetaResult) (err error)
	StoreResult(key []byte, res *pb.BuildResult) (err error)
	ResultKey(builder, slave string, timestamp time.Time) []byte
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

func (self *builderImpl) Build(change *pb.ChangeRequest) error {
	var wg sync.WaitGroup
	var errs []error

	builderLogDir := path.Join(self.logDir, self.project, self.builder)
	if err := os.MkdirAll(builderLogDir, 0750); err != nil {
		return fmt.Errorf("Failed to create logdir %s: %s", logDir, err)
	}

	for _, s := range self.slaves {
		wg.Add(1)
		go func(s BuildSlave, bp *pb.Blueprint) {
			defer wg.Done()
			//lock_start := time.Now()
			s.Acquire()
			//lock_wait := time.Since(lock_start)
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
			}
			self.store.StoreResult(self.store.ResultKey(self.builder, s.Name(), start), result)

		}(s, self.blueprint)
	}
	wg.Wait()

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

// FIXME(an): When do we run this function actually?
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
	go s.executionLoop(config.GetInterval())
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
		_, ok := self.findProject(rep.Project)
		if !ok {
			glog.Error("Received report for unknown project: %s", rep.Project)
			// TODO(rn): Increment error metric
		}
		glog.Infof("Process report for project %s", rep.Project)

		for i, si := range self.scheduler[rep.Project] {
			if si.ShouldSchedule(rep) {
				glog.Infof("Scheduler %d wants to schedule change", i)
				go func(change *pb.ChangeRequest) {
					if err := si.Schedule(change); err != nil {
						glog.Errorf("Failed to schedule on scheduler %d in project %s: %s", i, rep.Project, err)
						// TODO(rn): Increment error metric
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
		if p.GetName() == proj {
			return p, true
		}
	}
	return nil, false
}

func findBlueprint(blueprints []*pb.Blueprint, name string) (result *pb.Blueprint) {
	// Maybe consider using sort and binary search, but there should be sufficient few blueprints defined anyway
	for _, bp := range blueprints {
		if name == bp.GetName() {
			return bp
		}
	}
	return nil
}

func setupFromConfigOrDie(conf *pb.GoBuilder, server *grpc.Server, registry *BuildSlaveRegistry, logDir string) (processor *reportProcessor, cleanupClosure func()) {
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
		registry.Add(s.GetName(), newRpcBuildSlave(s.GetName(), s.GetAddress()))
	}

	for _, proj := range conf.GetProject() {
		db := newLdbResultStorage(path.Join(logDir, proj.GetName()))
		cleanupFns = append(cleanupFns, db.Close)

		if sourceNotifier == nil && proto.HasExtension(proj.GetSource(), pb.E_SourceNotifier_Source) {
			sourceNotifier = NewRpcSourceNotifier()
			sourceNotifier.Start()

			sourceNotifierStopFn := func() {
				if err := sourceNotifier.Stop(); err != nil {
					glog.Errorf("Failed to stop source notifier: %s", err)
				}
			}
			pb.RegisterChangeSourceServer(server, sourceNotifier)

			cleanupFns = append(cleanupFns, sourceNotifierStopFn)
		}

		builder := make(map[string]Builder)

		for _, b := range proj.GetBuilder() {
			var slaves []BuildSlave
			for _, slaveName := range b.GetSlave() {
				if sl := registry.Find(slaveName); sl == nil {
					glog.Fatalf("Slave %s referenced from builder %s in %s could not be found", slaveName, b.GetName(), proj.GetName())
				} else {
					slaves = append(slaves, sl)
				}
			}
			blueprint := findBlueprint(proj.GetBlueprint(), b.GetBlueprint())
			if blueprint == nil {
				glog.Fatalf("Blueprint %s referenced in builder %s not found in project %s", b.GetBlueprint(), b.GetName(), proj.GetName())
			}
			builder[b.GetName()] = &builderImpl{logDir: logDir, slaves: slaves, blueprint: blueprint, project: proj.GetName(), builder: b.GetName(), store: db}
		}

		for i, sched := range proj.GetScheduler() {
			var builders []Builder
			for _, b := range sched.GetBuilder() {
				bi, ok := builder[b]
				if !ok {
					glog.Fatalf("Builder %s referenced from scheduler %d in %s could not be found", b, i, proj.GetName())
				}
				builders = append(builders, bi)
			}
			switch {
			case proto.HasExtension(sched, pb.E_PeriodicScheduler_Scheduler):
				ext, err := proto.GetExtension(sched, pb.E_PeriodicScheduler_Scheduler)
				if err != nil {
					glog.Fatalf("Failed to load extension (Project=%s): %s", proj.GetName(), err)
				}

				schedulerMap[proj.GetName()] = append(schedulerMap[proj.GetName()], NewPeriodicScheduler(builders, ext.(*pb.PeriodicScheduler)))
			default:
				glog.Fatalf("%s defines unknown scheduler", proj.GetName())
			}
		}
	}

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

	registry := NewBuildSlaveRegistry()
	rpcServer := grpc.NewServer()
	processor, cleanupCb := setupFromConfigOrDie(conf, rpcServer, registry, *logDir)
	defer cleanupCb()
	go processor.Process()

	lis, err := net.Listen("tcp", *address)
	if err != nil {
		glog.Fatalf("Failed to listen: %s", err)
	}

	if *tls {
		creds, err := credentials.NewServerTLSFromFile(*certFile, *keyFile)
		if err != nil {
			glog.Fatalf("Failed to load credentials from (Cert=%s,Key=%s): %s", *certFile, *keyFile, err)
		}
		rpcServer.Serve(creds.NewListener(lis))
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
			return fmt.Errorf("Project %s has no blueprints defined", proj.GetName())
		}

		// Test for unique blueprint ids (per project)
		entitySet := make([]Entity, len(bps))
		for i, bp := range bps {
			entitySet[i] = bp
		}
		_, err := buildEntitySet(entitySet)
		if err != nil {
			return fmt.Errorf("While validating blueprints for %s: %s", proj.GetName(), err)
		}

		// Test for unique builder ids (per project)
		entitySet = make([]Entity, len(proj.GetBuilder()))
		for i, b := range proj.GetBuilder() {
			if b.GetSlave() == nil || len(b.GetSlave()) == 0 {
				return fmt.Errorf("Builder %s in project %s does not reference any slaves", b.GetName(), proj.GetName())
			}
			entitySet[i] = b
		}
		builderNames, err := buildEntitySet(entitySet)
		if err != nil {
			return fmt.Errorf("While validating builders for %s: %s", proj.GetName(), err)
		}

		if scheds := proj.GetScheduler(); scheds == nil || len(scheds) == 0 {
			return fmt.Errorf("Project %s has no schedulers defined", proj.GetName())
		} else {
			for i, sched := range scheds {
				if sched.GetBuilder() == nil || len(sched.GetBuilder()) == 0 {
					return fmt.Errorf("Scheduler %d in project %s does not reference any builder", i, proj.GetName())
				}
				for _, b := range sched.GetBuilder() {
					if _, has := builderNames[b]; !has {
						return fmt.Errorf("Scheduler %d in project %s references unknown builder: %s", i, proj.GetName(), b)
					}
				}
			}
		}

		// Validating blueprints (empty blueprints will be rejected)
		for i, bp := range bps {
			if steps := bp.GetStep(); steps == nil || len(steps) == 0 {
				return fmt.Errorf("Blueprint %d named %s must contain at least one step (Project=%s)", i, bp.GetName(), proj.GetName())
			} else {
				for i, step := range steps {
					if argvs := step.GetArgv(); argvs == nil || len(argvs) == 0 {
						return fmt.Errorf("Step %d has no argv defined (Project=%s,Blueprint=%s)", i, proj.GetName(), bp.GetName())
					}
				}
			}
		}

		if proj.GetSource() == nil {
			return fmt.Errorf("Project %s has no source defined", proj.GetName())
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

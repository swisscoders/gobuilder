package repo

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	pb "../../proto"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type Projects struct {
	conf *pb.GoBuilder
	logs string
}

func (self *Projects) Projects() []*pb.Project {
	return self.conf.GetProject()
}

func (self *Projects) FindProject(name string) *pb.Project {
	for _, proj := range self.conf.GetProject() {
		if proj.GetName() == name {
			return proj
		}
	}
	return nil
}

func ProjectsFromProto(conf *pb.GoBuilder, logsDir string) *Projects {
	return &Projects{conf: conf, logs: logsDir}
}

type BuildResultRepo interface {
	Results(project string) (keys []Key, results []*pb.BuildResult, err error)
}

type buildResultRepo struct {
	client pb.ResultClient
}

func NewBuildResultRepo(conn *grpc.ClientConn) BuildResultRepo {
	return &buildResultRepo{client: pb.NewResultClient(conn)}
}

func (self *buildResultRepo) Results(project string) (keys []Key, results []*pb.BuildResult, err error) {
	resp, err := self.client.GetResult(context.Background(), &pb.GetResultRequest{Project: project})
	if err != nil {
		err = fmt.Errorf("Failed to obtain status of project %s: %s", project, err)
		return
	}

	for _, k := range resp.Key {
		keys = append(keys, Key(strings.Split(string(k), "_")))
	}

	return keys, resp.Result, nil
}

func LatestBuild(results ...*pb.BuildResult) *pb.BuildResult {
	if len(results) == 0 {
		return nil
	}

	return results[len(results)-1]
}

func IsBuildStatusOK(results ...*pb.BuildResult) (succeess bool) {
	if len(results) == 0 {
		return true
	}

	latest_build := results[len(results)-1]
	if latest_build.Status.CoreDump == true || latest_build.Status.ExitStatus != 0 {
		return false
	}
	return true
}

type Key []string

func (self Key) Builder() string {
	return self[0]
}

func (self Key) Slave() string {
	return self[1]
}

func (self Key) Timestamp() time.Time {
	t, _ := strconv.Atoi(self[2])
	return time.Unix(0, int64(t))
}

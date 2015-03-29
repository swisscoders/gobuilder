package main

import (
	"flag"
	"io/ioutil"

	pb "../proto"
	"github.com/golang/glog"
	"github.com/golang/protobuf/proto"

	"./repo"
	"./usecase"
	"./web"
	"./console"
	"google.golang.org/grpc"
)

var logsDir = flag.String("build_log_dir", "./../logs", "path to project logs directory")
var buildConf = flag.String("build_conf", "./../build.conf", "path to build.conf")
var masterAddress = flag.String("master", "127.0.0.1:3900", "address of master")
var viewer = flag.String("viewer", "console", "Choose the viewer [web|console]")

func main() {
	flag.Parse()

	conn, err := grpc.Dial(*masterAddress)
	if err != nil {
		panic(err)
	}

	projectsRepo := repo.ProjectsFromProto(ReadProjectConfigOrDie(*buildConf), *logsDir)
	buildRepo := repo.NewBuildResultRepo(conn)

	if *viewer == "web" {
		web.StartViewer(projectsRepo, buildRepo, usecase.NewBuildOutputReader(*logsDir))
	} else {
		console.StartViewer(projectsRepo, buildRepo, usecase.NewBuildOutputReader(*logsDir))
	}
}

func ReadProjectConfigOrDie(path string) *pb.GoBuilder {
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

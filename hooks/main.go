// commit hash, branch ...
package main

import (
	"flag"
	"os/exec"
	"strings"

	pb "./proto"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

/*
echo $(git rev-parse HEAD)
echo $(git rev-parse --symbolic --abbrev-ref HEAD)
echo $(git diff-index HEAD^1 --name-only)
*/

var address = flag.String("address", "localhost:50052", "address of the master")

func main() {
	flag.Parse()

	req, err := createChangeRequest()
	if err != nil {
		glog.Errorf("Unable to create the request: %s", err)
		return
	}

	conn, err := grpc.Dial(*address)
	if err != nil {
		glog.Errorf("Cannot dial the master %s: %s", *address, err)
		return
	}
	defer conn.Close()

	_, err = pb.NewChangeSourceClient(conn).Notify(context.Background(), req)
	if err != nil {
		glog.Errorf("Failed to notify the master: %s", err)
		return
	}
	glog.Infof("Notified the master (at %s) successfully", *address)
}

func createChangeRequest() (req *pb.ChangeRequest, err error) {
	req = new(pb.ChangeRequest)
	g := new(Git)

	req.Commithash, err = g.Revision()
	if err != nil {
		return
	}

	req.Branch, err = g.Branch()
	if err != nil {
		return
	}

	req.Files, err = g.Files()
	if err != nil {
		return
	}

	req.Dir, err = g.Directory()
	if err != nil {
		return
	}
	return
}

type Git struct{}

func (self *Git) Revision() (hash string, err error) {
	output, err := self.executeGit("rev-parse", "HEAD")
	return strings.TrimSpace(output), err
}

func (self *Git) Branch() (branch string, err error) {
	output, err := self.executeGit("rev-parse", "--symbolic", "--abbrev-ref", "HEAD")
	return strings.TrimSpace(output), err
}

func (self *Git) Files() (files []string, err error) {
	output, err := self.executeGit("diff-index", "HEAD^1", "--name-only")
	if err != nil {
		return
	}

	return strings.Split(strings.TrimSpace(output), "\n"), nil
}

func (self *Git) Directory() (dirname string, err error) {
	output, err := self.executeGit("rev-parse", "--show-toplevel")
	return strings.TrimSpace(output), err
}

func (self *Git) executeGit(args ...string) (out string, err error) {
	cmd := exec.Command("git", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		glog.Errorf("Command exited with failure: %s\n%s", err, string(output))
	} else {
		glog.V(1).Infof("Command exited successfully: %s", string(output))
	}

	return string(output), err
}

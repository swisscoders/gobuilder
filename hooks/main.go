// commit hash, branch ...
package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	pb "./proto"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

/*
post commit:
echo $(git rev-parse HEAD)
echo $(git rev-parse --symbolic --abbrev-ref HEAD)
echo $(git diff-index HEAD^1 --name-only)

bare post-receive:
git diff --name-only 2684f4499fc90bf92382dd0569d22e4300dfb1f2 12ca53c375a030b81c796b12a6c83dfe4ad95782
bab28f2c50267bbfda0c3ec93d5b1f19cc3a943d 2684f4499fc90bf92382dd0569d22e4300dfb1f2 refs/heads/master
*/

var address = flag.String("address", "localhost:50052", "address of the master")
var local = flag.Bool("local", false, "Use the hook binary as a post-commit hook")
var gitRepoPath = flag.String("path", "", "full path to the git repository")

func main() {
	flag.Parse()

	req, ignore, err := createChangeRequest(*gitRepoPath)
	if err != nil {
		glog.Errorf("Unable to create the post commit request: %s", err)
		os.Exit(1)
	}

	if ignore {
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

func createChangeRequest(bareRepoPath string) (req *pb.ChangeRequest, ignore bool, err error) {
	req = new(pb.ChangeRequest)

	// Read the post-receive information from stdin
	g := new(GitPostReceive)
	err = g.Parse(os.Stdin)
	if err != nil {
		return
	}

	req.Commithash, err = g.Revision()
	if err != nil {
		return
	}

	// branch is optional (there could be tags later)
	req.Branch, _ = g.Branch()
	if req.Branch == "" {
		ignore = true
		return
	}

	req.Files, err = g.Files()
	if err != nil {
		return
	}

	req.Repo = bareRepoPath
	if req.Repo == "" {
		err = fmt.Errorf("Repo path needs to be specified")
		return
	}

	return
}

type GitPostReceive struct {
	branch        string
	oldCommitHash string
	newCommitHash string
}

func (self *GitPostReceive) Parse(r io.Reader) error {
	updateInfo, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	line := strings.SplitN(strings.TrimSpace(string(updateInfo)), " ", 3)
	if len(line) != 3 {
		return fmt.Errorf("Expected to extract old hash, new hash and branch. Got: %#v\n", line)
	}

	self.oldCommitHash = strings.TrimSpace(line[0])
	self.newCommitHash = strings.TrimSpace(line[1])

	// We ignore everything else than branch information:
	if !strings.HasPrefix(line[2], "refs/heads") {
		return nil
	}

	self.branch = strings.TrimPrefix(line[2], "refs/heads/")
	return nil
}

func (self *GitPostReceive) Revision() (string, error) {
	if self.newCommitHash == "" {
		return "", fmt.Errorf("Commit Hash is empty")
	}

	return self.newCommitHash, nil
}

func (self *GitPostReceive) Branch() (string, error) {
	if self.branch == "" {
		return "", fmt.Errorf("Branch is empty")
	}

	return self.branch, nil
}

func (self *GitPostReceive) Files() (files []string, err error) {
	output, err := executeGit("diff", "--name-only", self.oldCommitHash, self.newCommitHash)
	if err != nil {
		return
	}

	return strings.Split(strings.TrimSpace(output), "\n"), nil
}

func executeGit(args ...string) (out string, err error) {
	cmd := exec.Command("git", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		glog.Errorf("Command exited with failure: %s\n%s", err, string(output))
	} else {
		glog.V(1).Infof("Command exited successfully: %s", string(output))
	}

	return string(output), err
}

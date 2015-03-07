// commit hash, branch ...
package main

import (
	"flag"
	"fmt"
	"os"

	"./git"

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

=======================
Push + branch creation:
3d976f56eb5d8c419c7b7263855fa218a8d93683 43a70237d6d7ed51e920e792dc83f60b0c4eb52e refs/heads/master
0000000000000000000000000000000000000000 43a70237d6d7ed51e920e792dc83f60b0c4eb52e refs/heads/foobar
=======
Push + tags creation:
b28f2c50267bbfda0c3ec93d5b1f19cc3a943d 2684f4499fc90bf92382dd0569d22e4300dfb1f2 refs/heads/master
0000000000000000000000000000000000000000 3756c13a37bb8307eafa7a7212b9d5317e49667b refs/tags/v0.100
0000000000000000000000000000000000000000 c376faf8d6350d265d490b59bc860fb69d0645f6 refs/tags/v0.101

=======
git rev-list f25367f7692e3a2f3e1abed44597c8ceb3a9e218...
a74dbfea4d9daf37e39b23181d19020538e6f9ba
57d6a4e06956fdc3e23e2684a2fd43e1c0b58445
5065a57740dfc7ca06479aa1f6521763b0793636

then we need to:
git diff --name-only 5065a57740dfc7ca06479aa1f6521763b0793636 57d6a4e06956fdc3e23e2684a2fd43e1c0b58445
git diff --name-only 57d6a4e06956fdc3e23e2684a2fd43e1c0b58445 a74dbfea4d9daf37e39b23181d19020538e6f9ba
git diff --name-only a74dbfea4d9daf37e39b23181d19020538e6f9ba f25367f7692e3a2f3e1abed44597c8ceb3a9e218

and send each commit via RPC with the appropriate fields filled in.
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

const emptyCommitHash = "0000000000000000000000000000000000000000"

func createChangeRequest(bareRepoPath string) (req *pb.ChangeRequest, ignore bool, err error) {
	req = new(pb.ChangeRequest)

	// Read the post-receive information from stdin
	g := new(git.PostReceive)
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

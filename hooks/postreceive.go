// commit hash, branch ...
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"./git"

	pb "../proto"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	//"google.golang.org/grpc/credentials"
)

/*
post commit:
echo $(git rev-parse HEAD)
echo $(git rev-parse --symbolic --abbrev-ref HEAD)
echo $(git diff-index HEAD^1 --name-only)

bare post-receive:
git diff --name-only 2684f4499fc90bf92382dd0569d22e4300dfb1f2 12ca53c375a030b81c796b12a6c83dfe4ad95782
bab28f2c50267bbfda0c3ec93d5b1f19cc3a943d 2684f4499fc90bf92382dd0569d22e4300dfb1f2 refs/heads/master

====
post receive on initial git repo (sample):
0000000000000000000000000000000000000000 37f4ba53dc295b344333f1060102bcac56f79837 refs/heads/master

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
var gitRepoPath = flag.String("repo", "", "full path to the git repository")
var projectName = flag.String("project", "", "project identifier")

var caFile = flag.String("ca_file", "cert/ca.pem", "The file containning the CA root cert file")
var tls = flag.Bool("tls", false, "Enable tls")

func main() {
	flag.Parse()

	commits, err := parsePostReceiveRequest(os.Stdin)
	if err != nil {
		panic(err)
	}

	var opts []grpc.DialOption
	if *tls {
		/*var sn string
		var creds credentials.TransportAuthenticator
		if *caFile != "" {
			var err error
			creds, err = credentials.NewClientTLSFromFile(*caFile, sn)
			if err != nil {
				log.Fatalf("Failed to create TLS credentials %v", err)
			}
		} else {
			creds = credentials.NewClientTLSFromCert(nil, sn)
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))*/
		panic("Does not work anyway - dont use it")
	}

	conn, err := grpc.Dial(*address, opts...)
	if err != nil {
		glog.Errorf("Cannot dial the master %s: %s", *address, err)
		return
	}
	defer conn.Close()

	client := pb.NewChangeSourceClient(conn)

	reqs, err := createRequestsFromCommits(*gitRepoPath, *projectName, commits)
	if err != nil {
		panic(err)
	}

	for _, req := range reqs {
		_, err = client.Notify(context.Background(), req)
		if err != nil {
			glog.Errorf("Failed to notify the master: %s", err)
			return
		}
		glog.Infof("Notified the master (at %s) successfully", *address)
	}
}

func parsePostReceiveRequest(r io.Reader) (commits []*git.Commit, err error) {
	lines := git.ParsePostReceiveLine(r)

	for _, line := range lines {
		if !line.IsBranch() {
			continue
		}

		// Handling the first commit (ie. OldCommitHash is empty)
		if line.OldCommitHash == "" {
			var files []string
			files, err = fetchFilesFromInitialCommit()
			if err != nil {
				glog.Errorf("Error trying to get list of files from initial commit: %s", err)
				return
			}
			commits = append(commits, &git.Commit{CommitHash: line.NewCommitHash, Branch: line.RefName(), Files: files})
			continue
		}

		pairs, err := extractCommitPairs(line.OldCommitHash, line.NewCommitHash)
		if err != nil {
			glog.Errorf("Failed to extract commit pairs: %s", err)
			return nil, err
		}

		comms, err := createCommitsFromCommitPairs(line.RefName(), pairs)
		if err != nil {
			glog.Errorf("Failed to create commits from the commit pairs: %s", err)
			return nil, err
		}

		commits = append(commits, comms...)
	}
	return
}

func createCommitsFromCommitPairs(refName string, pairs []*git.RevListCommitPair) (commits []*git.Commit, err error) {
	for _, pair := range pairs {
		// Collect files
		output, err := git.Execute("diff", "--name-only", pair.OldCommitHash, pair.NewCommitHash)
		if err != nil {
			err = fmt.Errorf("Error while Executing git diff --name-only %s %s, got: %s", pair.OldCommitHash, pair.NewCommitHash, output)
			return commits, err
		}

		files := strings.Split(strings.TrimSpace(output), "\n")
		commits = append(commits, &git.Commit{CommitHash: pair.NewCommitHash, Branch: refName, Files: files})
	}
	return
}

func extractCommitPairs(oldCommitHash, newCommitHash string) (pairs []*git.RevListCommitPair, err error) {
	output, err := git.Execute("rev-list", oldCommitHash+"..."+newCommitHash)
	if err != nil {
		return nil, fmt.Errorf("Error while executing git rev-list %s...%s : %s", oldCommitHash, newCommitHash, err)
	}

	pairs = git.ParseRevList(oldCommitHash, strings.NewReader(output))
	return
}

func fetchFilesFromInitialCommit() ([]string, error) {
	output, err := git.Execute("ls-tree", "--name-only", "-r", "HEAD")
	if err != nil {
		return nil, err
	}

	return strings.Split(strings.TrimSpace(output), "\n"), nil
}

func createRequestsFromCommits(bareRepoPath string, projectName string, commits []*git.Commit) (reqs []*pb.ChangeRequest, err error) {
	for _, commit := range commits {
		reqs = append(reqs, &pb.ChangeRequest{
			RepoType: pb.ChangeRequest_GIT,
			Project:  projectName,
			Commit:   commit.CommitHash,
			Branch:   commit.Branch,
			Files:    commit.Files,
			Repo:     bareRepoPath,
		})
	}
	return
}

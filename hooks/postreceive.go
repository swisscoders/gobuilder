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
)

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

	conn, err := grpc.Dial(*address)
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

		name, email, err := extractAuthorInfo(pair.NewCommitHash)
		if err != nil {
			err = fmt.Errorf("Failed to extract author info: %s", err)
			return commits, err
		}

		commits = append(commits, &git.Commit{CommitHash: pair.NewCommitHash, Branch: refName, Files: files, Name: name, Email: email})
	}
	return
}

func extractAuthorInfo(commitHash string) (name string, email string, err error) {
	output, err := git.Execute("log", "-1", commitHash, "--format=%an:%ae")
	if err != nil {
		return "", "", err
	}

	nameEmail := strings.Split(strings.TrimSpace(output), ":")
	return nameEmail[0], nameEmail[1], nil
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
			Name:     commit.Name,
			Email:    commit.Email,
		})
	}
	return
}

package git

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/golang/glog"
)

type Commit struct {
	CommitHash string
	Branch     string
	Tag        string

	Name  string
	Email string

	Files []string
}

type PostReceiveLine struct {
	OldCommitHash string
	NewCommitHash string

	Ref string
}

func (self *PostReceiveLine) IsTag() bool {
	return strings.HasPrefix(self.Ref, "refs/tags/")
}

func (self *PostReceiveLine) IsBranch() bool {
	return strings.HasPrefix(self.Ref, "refs/heads/")
}

func (self *PostReceiveLine) RefName() string {
	parts := strings.Split(self.Ref, "/")
	if len(parts) < 1 {
		return ""
	}

	return parts[len(parts)-1]
}

// Sample: bab28f2c50267bbfda0c3ec93d5b1f19cc3a943d 2684f4499fc90bf92382dd0569d22e4300dfb1f2 refs/heads/master
func ParsePostReceiveLine(r io.Reader) (lines []*PostReceiveLine) {
	readEachLine(r, func(line string) {
		// fmt.Println("Got: " + line)
		item := splitLine(line)
		if len(item) < 3 {
			return
		}

		lines = append(lines, &PostReceiveLine{OldCommitHash: replaceEmptyHash(item[0]), NewCommitHash: replaceEmptyHash(item[1]), Ref: item[2]})
	})
	return
}

type RevListCommitPair struct {
	OldCommitHash string
	NewCommitHash string
}

// ParseRevList parses the output of git rev-list CommitHash...
func ParseRevList(latestCommitHash string, r io.Reader) (pairs []*RevListCommitPair) {
	var hashes []string

	readEachLine(r, func(line string) {
		hashes = append([]string{line}, hashes...)
	})

	if len(hashes) == 0 {
		return
	}

	previousHash := latestCommitHash
	for _, hash := range hashes {
		pairs = append(pairs, &RevListCommitPair{OldCommitHash: previousHash, NewCommitHash: hash})
		previousHash = hash
	}

	return
}

func readEachLine(r io.Reader, fn func(line string)) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) != "" {
			fn(line)
		}
	}
	return
}

func replaceEmptyHash(hash string) (newHash string) {
	if hash == "0000000000000000000000000000000000000000" {
		return ""
	}
	return hash
}

func splitLine(line string) []string {
	return strings.Split(line, " ")
}

type PostReceive struct {
	branch        string
	oldCommitHash string
	newCommitHash string
}

func (self *PostReceive) Revision() (string, error) {
	if self.newCommitHash == "" {
		return "", fmt.Errorf("Commit Hash is empty")
	}

	return self.newCommitHash, nil
}

func (self *PostReceive) Branch() (string, error) {
	if self.branch == "" {
		return "", fmt.Errorf("Branch is empty")
	}

	return self.branch, nil
}

func (self *PostReceive) Files() (files []string, err error) {
	output, err := Execute("diff", "--name-only", self.oldCommitHash, self.newCommitHash)
	if err != nil {
		return
	}

	return strings.Split(strings.TrimSpace(output), "\n"), nil
}

func Execute(args ...string) (out string, err error) {
	cmd := exec.Command("git", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		glog.Errorf("Command exited with failure: %s\n%s", err, string(output))
	} else {
		glog.V(1).Infof("Command exited successfully: %s", string(output))
	}

	return string(output), err
}

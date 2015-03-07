package git

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os/exec"
	"strings"

	"github.com/golang/glog"
)

type Commit struct {
	CommitHash string
	Branch     string
	Tag        string

	Files []string
}

type DiffLine struct {
	OldCommitHash string
	NewCommitHash string

	Ref string
}

func (self *DiffLine) IsTag() bool {
	return strings.HasPrefix(self.Ref, "refs/tags/")
}

func (self *DiffLine) IsBranch() bool {
	return strings.HasPrefix(self.Ref, "refs/heads/")
}

func (self *DiffLine) RefName() string {
	parts := strings.Split(self.Ref, "/")
	if len(parts) < 1 {
		return ""
	}

	return parts[len(parts)-1]
}

// ParseDiff parses the output of: git diff --name-only 2684f4499fc90bf92382dd0569d22e4300dfb1f2 12ca53c375a030b81c796b12a6c83dfe4ad95782
// Sample: bab28f2c50267bbfda0c3ec93d5b1f19cc3a943d 2684f4499fc90bf92382dd0569d22e4300dfb1f2 refs/heads/master
func ParseDiff(r io.Reader) (lines []*DiffLine) {
	readEachLine(r, func(line string) {
		item := splitLine(line)
		lines = append(lines, &DiffLine{OldCommitHash: replaceEmptyHash(item[0]), NewCommitHash: replaceEmptyHash(item[1]), Ref: item[2]})
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

	previousHash := hashes[0]
	for _, hash := range hashes[1:] {
		pairs = append(pairs, &RevListCommitPair{OldCommitHash: previousHash, NewCommitHash: hash})
		previousHash = hash
	}
	pairs = append(pairs, &RevListCommitPair{OldCommitHash: previousHash, NewCommitHash: latestCommitHash})

	return
}

// ArgsFromRevListPairs is a helper and parses the RevListCommitPairs and returns:
// diff --name-only OldCommitHash NewCommitHash
// without the "git".
func ArgsFromRevListPairs(pairs []*RevListCommitPair) (argsList [][]string) {
	for _, pair := range pairs {
		argsList = append(argsList, []string{"diff", "--name-only", pair.OldCommitHash, pair.NewCommitHash})
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

func (self *PostReceive) Parse(r io.Reader) error {
	updateInfo, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	fmt.Println("Got: " + string(updateInfo))

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

package git

import (
	"strings"
	"testing"
)

func TestDiffParseNothing(t *testing.T) {
	out := ParseDiff(strings.NewReader(""))
	if len(out) != 0 {
		t.Errorf("Parsing nothing should return no entry")
	}
}

func TestDiffParseOneLine(t *testing.T) {
	var oneLineGitDiff = "bab28f2c50267bbfda0c3ec93d5b1f19cc3a943d 2684f4499fc90bf92382dd0569d22e4300dfb1f2 refs/heads/master"

	out := ParseDiff(strings.NewReader(oneLineGitDiff))
	if len(out) != 1 {
		t.Errorf("Parsing one line should return one entry")
		return
	}
	verifyEntry(t, out[0], "bab28f2c50267bbfda0c3ec93d5b1f19cc3a943d", "2684f4499fc90bf92382dd0569d22e4300dfb1f2", "refs/heads/master")
}

func TestDiffParseTwoLine(t *testing.T) {
	var multiLineGitDiffs = "bab28f2c50267bbfda0c3ec93d5b1f19cc3a943d 2684f4499fc90bf92382dd0569d22e4300dfb1f2 refs/heads/master\nbab28f2c50267bbfda0c3ec93d5b1f19cc3a943d 2684f4499fc90bf92382dd0569d22e4300dfb1f2 refs/heads/else"

	out := ParseDiff(strings.NewReader(multiLineGitDiffs))
	if len(out) != 2 {
		t.Errorf("Parsing two line should return two entry")
		return
	}
	verifyEntry(t, out[0], "bab28f2c50267bbfda0c3ec93d5b1f19cc3a943d", "2684f4499fc90bf92382dd0569d22e4300dfb1f2", "refs/heads/master")
	verifyEntry(t, out[1], "bab28f2c50267bbfda0c3ec93d5b1f19cc3a943d", "2684f4499fc90bf92382dd0569d22e4300dfb1f2", "refs/heads/else")
}

func TestDiffParseThreeLineWithEmptyHashes(t *testing.T) {
	multiLines := "b28f2c50267bbfda0c3ec93d5b1f19cc3a943d 2684f4499fc90bf92382dd0569d22e4300dfb1f2 refs/heads/master\n"
	multiLines = multiLines + "0000000000000000000000000000000000000000 3756c13a37bb8307eafa7a7212b9d5317e49667b refs/tags/v0.100\n"
	multiLines = multiLines + "0000000000000000000000000000000000000000 c376faf8d6350d265d490b59bc860fb69d0645f6 refs/tags/v0.101\n"

	out := ParseDiff(strings.NewReader(multiLines))
	if len(out) != 3 {
		t.Errorf("Parsing three line should return three entry")
		return
	}
	verifyEntry(t, out[0], "b28f2c50267bbfda0c3ec93d5b1f19cc3a943d", "2684f4499fc90bf92382dd0569d22e4300dfb1f2", "refs/heads/master")
	verifyEntry(t, out[1], "", "3756c13a37bb8307eafa7a7212b9d5317e49667b", "refs/tags/v0.100")
	verifyEntry(t, out[2], "", "c376faf8d6350d265d490b59bc860fb69d0645f6", "refs/tags/v0.101")
}

func verifyEntry(t *testing.T, gotEntry *DiffLine, expectedOldHash string, expectedNewHash string, expectedRef string) {
	if gotEntry.OldCommitHash != expectedOldHash {
		t.Errorf("Old commit hash expected to be %s, got: %s", expectedOldHash, gotEntry.OldCommitHash)
		return
	}

	if gotEntry.NewCommitHash != expectedNewHash {
		t.Errorf("New commit hash expected to be %s, got: %s", expectedNewHash, gotEntry.NewCommitHash)
		return
	}

	if gotEntry.Ref != expectedRef {
		t.Errorf("Ref expected to be %s, got: %s", expectedRef, gotEntry.Ref)
		return
	}
}

func TestDiffRefBranch(t *testing.T) {
	diff := &DiffLine{OldCommitHash: "", NewCommitHash: "", Ref: "refs/heads/master"}

	if diff.IsBranch() != true {
		t.Errorf("Expected Ref %s to be a branch (IsBranch() -> false)", diff.Ref)
		return
	}

	if diff.IsTag() == true {
		t.Errorf("Expected Ref %s to be not a tag (IsTag() -> true)", diff.Ref)
		return
	}

	if diff.RefName() != "master" {
		t.Errorf("Expected Ref Name to be master, got: %s", diff.RefName())
		return
	}
}

func TestDiffRefTag(t *testing.T) {
	diff := &DiffLine{OldCommitHash: "", NewCommitHash: "", Ref: "refs/tags/v0.100"}

	if diff.IsBranch() == true {
		t.Errorf("Expected Ref %s to be a tag (IsBranch() -> true)", diff.Ref)
		return
	}

	if diff.IsTag() != true {
		t.Errorf("Expected Ref %s to be a tag (IsTag() -> false)", diff.Ref)
		return
	}

	if diff.RefName() != "v0.100" {
		t.Errorf("Expected Ref Name to be v0.100, got: %s", diff.RefName())
		return
	}
}

func TestRevListParsing(t *testing.T) {
	inp := "a74dbfea4d9daf37e39b23181d19020538e6f9ba\n"
	inp = inp + "57d6a4e06956fdc3e23e2684a2fd43e1c0b58445\n"
	inp = inp + "5065a57740dfc7ca06479aa1f6521763b0793636\n"

	latestCommitHash := "f25367f7692e3a2f3e1abed44597c8ceb3a9e218"
	pairs := ParseRevList(latestCommitHash, strings.NewReader(inp))
	if len(pairs) != 3 {
		t.Errorf("Expected to find 3 pairs, got: %d", len(pairs))
		return
	}

	verifyPair(t, pairs[0], "5065a57740dfc7ca06479aa1f6521763b0793636", "57d6a4e06956fdc3e23e2684a2fd43e1c0b58445")
	verifyPair(t, pairs[1], "57d6a4e06956fdc3e23e2684a2fd43e1c0b58445", "a74dbfea4d9daf37e39b23181d19020538e6f9ba")
	verifyPair(t, pairs[2], "a74dbfea4d9daf37e39b23181d19020538e6f9ba", "f25367f7692e3a2f3e1abed44597c8ceb3a9e218")
}

func verifyPair(t *testing.T, gotPair *RevListCommitPair, expectedOldCommitHash string, expectedNewCommitHash string) {
	if gotPair.OldCommitHash != expectedOldCommitHash {
		t.Errorf("Old commit hash expected to be %s, got: %s", expectedOldCommitHash, gotPair.NewCommitHash)
		return
	}

	if gotPair.NewCommitHash != expectedNewCommitHash {
		t.Errorf("New commit hash expected to be %s, got: %s", expectedNewCommitHash, gotPair.NewCommitHash)
		return
	}
}

func TestCreatingArgsFromRevAList(t *testing.T) {
	inp := "a74dbfea4d9daf37e39b23181d19020538e6f9ba\n"
	inp = inp + "57d6a4e06956fdc3e23e2684a2fd43e1c0b58445\n"
	inp = inp + "5065a57740dfc7ca06479aa1f6521763b0793636\n"

	latestCommitHash := "f25367f7692e3a2f3e1abed44597c8ceb3a9e218"
	pairs := ParseRevList(latestCommitHash, strings.NewReader(inp))

	argList := ArgsFromRevListPairs(pairs)
	if len(argList) != 3 {
		t.Errorf("Expected to find 3 argLists, got: %d", len(argList))
		return
	}

	got := strings.Join(argList[0], " ")
	expected := "diff --name-only 5065a57740dfc7ca06479aa1f6521763b0793636 57d6a4e06956fdc3e23e2684a2fd43e1c0b58445"
	if expected != got {
		t.Errorf("Expected first argList to be %s, got: %s", expected, got)
		return
	}

	got = strings.Join(argList[1], " ")
	expected = "diff --name-only 57d6a4e06956fdc3e23e2684a2fd43e1c0b58445 a74dbfea4d9daf37e39b23181d19020538e6f9ba"
	if expected != got {
		t.Errorf("Expected second argList to be %s, got: %s", expected, got)
		return
	}

	got = strings.Join(argList[2], " ")
	expected = "diff --name-only a74dbfea4d9daf37e39b23181d19020538e6f9ba f25367f7692e3a2f3e1abed44597c8ceb3a9e218"
	if expected != got {
		t.Errorf("Expected third argList to be %s, got: %s", expected, got)
		return
	}
}

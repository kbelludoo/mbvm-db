package impact

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitFixtureRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, string(output))
	}
}

func TestCompareGitRefsCommittedTreesOnly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git executable not available")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg"), 0755); err != nil {
		t.Fatal(err)
	}
	gitFixtureRun(t, root, "init")
	filename := filepath.Join(root, "pkg", "example.go")
	write := func(src string) {
		t.Helper()
		if err := os.WriteFile(filename, []byte(src), 0600); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(msg string) {
		t.Helper()
		gitFixtureRun(t, root, "add", "pkg/example.go")
		gitFixtureRun(t, root, "-c", "user.name=MBVM Test", "-c", "user.email=mbvm@example.invalid", "commit", "-m", msg)
	}
	write("package example\nfunc F(a int) int {return a}\nfunc Call() int{return F(1)}\n")
	commit("v1")
	write("package example\nfunc F(a,b int) int {return a+b}\nfunc Call() int{return F(1,2)}\n")
	commit("v2")
	dirty := filepath.Join(root, "untracked.txt")
	if err := os.WriteFile(dirty, []byte("do not touch"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(dirty)
	if err != nil {
		t.Fatal(err)
	}
	report, err := CompareGitRefs(root, "HEAD~1", "HEAD", "pkg")
	if err != nil {
		t.Fatal(err)
	}
	changed := changeByName(t, report, "F")
	if changed.Kind != SignatureChanged || changed.Compatibility == nil ||
		!changed.Compatibility.Supported || len(changed.Compatibility.BrokenOldDirectCallers) != 1 {
		t.Fatalf("not a verified backward-compatibility break: %+v", changed)
	}
	after, err := os.ReadFile(dirty)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("dirty working tree was changed")
	}
	if _, err := CompareGitRefs(root, "--output=evil", "HEAD", "pkg"); err == nil {
		t.Fatal("accepted leading-dash Git ref")
	}
	if _, err := CompareGitRefs(root, "HEAD~1", "HEAD", "../escape"); err == nil {
		t.Fatal("accepted unsafe package path")
	}
	t.Logf("Git ref comparison found %d changed symbols without checkout or source-file writes", len(report.Changes))
}

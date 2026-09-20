package instfile

import (
	"os"
	"path/filepath"
	"testing"
)

type fakeRunner struct {
	calls [][]string
	rc    int
}

func (f *fakeRunner) Run(name string, args ...string) int {
	f.calls = append(f.calls, append([]string{name}, args...))
	return f.rc
}

func TestWritableTargetGoesThroughGo(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "sub", "file")

	target, err := Open(dest, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()

	if _, err := target.File().WriteString("payload"); err != nil {
		t.Fatal(err)
	}

	run := &fakeRunner{}
	if err := target.Commit(run); err != nil {
		t.Fatal(err)
	}
	if len(run.calls) != 0 {
		t.Fatalf("escalated to sudo for a writable directory: %v", run.calls)
	}

	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "payload" {
		t.Fatalf("got %q", body)
	}

	st, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o755 {
		t.Fatalf("mode=%o, want 755", st.Mode().Perm())
	}
}

func TestTempFileIsRemovedWhenNotCommitted(t *testing.T) {
	dir := t.TempDir()
	target, err := Open(filepath.Join(dir, "file"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	name := target.File().Name()
	target.Close()

	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Fatalf("the temporary file survived: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("left %d entries behind", len(entries))
	}
}

func TestUnwritableTargetEscalates(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}

	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(locked, "file")

	target, err := Open(dest, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()

	if _, err := target.File().WriteString("x"); err != nil {
		t.Fatal(err)
	}

	run := &fakeRunner{}
	if err := target.Commit(run); err != nil {
		t.Fatal(err)
	}
	if len(run.calls) != 1 {
		t.Fatalf("calls=%v", run.calls)
	}
	got := run.calls[0]
	if got[0] != "sudo" || got[1] != "install" || got[2] != "-D" || got[3] != "-m" || got[4] != "0600" {
		t.Fatalf("unexpected command: %v", got)
	}
	if got[len(got)-1] != dest {
		t.Fatalf("destination=%q, want %q", got[len(got)-1], dest)
	}
}

func TestSudoFailurePropagates(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}

	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o555); err != nil {
		t.Fatal(err)
	}

	target, err := Open(filepath.Join(locked, "file"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()

	if err := target.Commit(&fakeRunner{rc: 1}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestDirectoryDestinationIsRejected(t *testing.T) {
	if _, err := Open(t.TempDir(), 0o644); err == nil {
		t.Fatal("expected an error for a directory destination")
	}
}

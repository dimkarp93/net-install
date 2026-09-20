package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func quiet(t *testing.T) {
	t.Helper()
	t.Setenv("NO_PROXY", "*")
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = devnull
	t.Cleanup(func() {
		os.Stderr = old
		devnull.Close()
	})
}

func TestScriptRunsAndPropagatesTheExitCode(t *testing.T) {
	quiet(t)
	marker := filepath.Join(t.TempDir(), "argv")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "#!/bin/sh\nprintf '%%s' \"$*\" > %s\nexit 7\n", marker)
	}))
	defer srv.Close()

	rc := Run([]string{"script", "--shell", "/bin/sh", srv.URL, "--unattended", "--keep-zshrc"})
	if rc != 7 {
		t.Fatalf("rc=%d, want 7", rc)
	}

	argv, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(argv) != "--unattended --keep-zshrc" {
		t.Fatalf("argv=%q", argv)
	}
}

func TestScriptLeavesNoTemporaryFile(t *testing.T) {
	quiet(t)
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "#!/bin/sh\nexit 0\n")
	}))
	defer srv.Close()

	if rc := Run([]string{"script", "--shell", "/bin/sh", srv.URL}); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("left %d entries in TMPDIR", len(entries))
	}
}

func TestFetchToFileAndDownloadWithMode(t *testing.T) {
	quiet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "payload")
	}))
	defer srv.Close()

	dir := t.TempDir()

	dest := filepath.Join(dir, "fetched")
	if rc := Run([]string{"fetch", srv.URL, dest}); rc != 0 {
		t.Fatalf("fetch rc=%d", rc)
	}
	if body, _ := os.ReadFile(dest); string(body) != "payload" {
		t.Fatalf("fetch wrote %q", body)
	}

	dest = filepath.Join(dir, "nested", "installed")
	if rc := Run([]string{"download", "--mode", "0755", srv.URL, dest}); rc != 0 {
		t.Fatalf("download rc=%d", rc)
	}
	st, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o755 {
		t.Fatalf("mode=%o, want 755", st.Mode().Perm())
	}
}

func TestFetchOfMissingUrlReturnsTwentyTwo(t *testing.T) {
	quiet(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "x")
	if rc := Run([]string{"fetch", srv.URL, dest}); rc != 22 {
		t.Fatalf("rc=%d, want 22", rc)
	}
}

func TestCloneAndSkipExisting(t *testing.T) {
	quiet(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}

	root := t.TempDir()
	bare := filepath.Join(root, "origin.git")
	if out, err := exec.Command("git", "init", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}

	work := filepath.Join(root, "work")
	if out, err := exec.Command("git", "clone", bare, work).CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v: %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(work, "readme"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-C", work, "add", "readme"},
		{"-C", work, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "init"},
		{"-C", work, "push", "origin", "HEAD"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}

	dest := filepath.Join(root, "cloned")
	if rc := Run([]string{"clone", "file://" + bare, dest}); rc != 0 {
		t.Fatalf("clone rc=%d", rc)
	}
	if _, err := os.Stat(filepath.Join(dest, "readme")); err != nil {
		t.Fatal(err)
	}

	marker := filepath.Join(dest, "marker")
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if rc := Run([]string{"clone", "file://" + bare, dest}); rc != 0 {
		t.Fatalf("second clone rc=%d", rc)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("an existing destination was not skipped")
	}

	if rc := Run([]string{"clone", "--force", "file://" + bare, dest}); rc != 0 {
		t.Fatalf("forced clone rc=%d", rc)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("--force did not replace the destination")
	}
}

func TestDangerousCloneDestinationsAreRefused(t *testing.T) {
	quiet(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	for _, dest := range []string{"/", home, ""} {
		if rc := Run([]string{"clone", "--force", "https://example.com/x", dest}); rc != 2 {
			t.Fatalf("destination %q gave rc=%d, want 2", dest, rc)
		}
	}
}

func TestUsageErrors(t *testing.T) {
	quiet(t)
	cases := [][]string{
		{},
		{"nope"},
		{"fetch"},
		{"fetch", "only-one"},
		{"log"},
		{"env", "extra"},
		{"download", "--mode", "0999", "u", "d"},
	}
	for _, args := range cases {
		if rc := Run(args); rc != 2 {
			t.Fatalf("args %v gave rc=%d, want 2", args, rc)
		}
	}
}

func TestBadEnvIsAUsageError(t *testing.T) {
	quiet(t)
	t.Setenv("NET_RETRIES", "many")
	if rc := Run([]string{"env"}); rc != 2 {
		t.Fatalf("rc=%d, want 2", rc)
	}
}

package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLayout(t *testing.T) {
	c := &Cache{root: "/c"}
	cases := map[string]string{
		"https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/install.sh": "/c/files/raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/install.sh",
		"https://just.systems/install.sh":                                 "/c/files/just.systems/install.sh",
		"https://example.com/":                                            "/c/files/example.com/index",
		"https://example.com:8443/a/../../etc/passwd":                     "/c/files/example.com_8443/etc/passwd",
	}
	for url, want := range cases {
		got, err := c.FilePath(url)
		if err != nil || got != want {
			t.Fatalf("%s: got %q, %v; want %q", url, got, err, want)
		}
	}

	q, _ := c.FilePath("https://example.com/f?v=1")
	q2, _ := c.FilePath("https://example.com/f?v=2")
	if !strings.HasPrefix(q, "/c/files/example.com/f@") || q == q2 {
		t.Fatalf("query paths %q %q", q, q2)
	}

	for _, url := range []string{"https://github.com/tmux-plugins/tpm", "https://github.com/tmux-plugins/tpm.git"} {
		got, err := c.GitPath(url)
		if err != nil || got != "/c/git/github.com/tmux-plugins/tpm" {
			t.Fatalf("%s: got %q, %v", url, got, err)
		}
	}
}

func TestOpen(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache")
	if _, err := Open(root); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(t.TempDir(), "a", "b")); err == nil {
		t.Fatal("a cache without a parent was accepted")
	}
}

func TestStoreAndValid(t *testing.T) {
	file := filepath.Join(t.TempDir(), "x", "f")
	tmp, err := Temp(file)
	if err != nil {
		t.Fatal(err)
	}
	tmp.WriteString("data")
	if err := Store(tmp, file); err != nil {
		t.Fatal(err)
	}
	if !Valid(file) {
		t.Fatal("stored file is not valid")
	}
	os.WriteFile(file, []byte("other"), 0o644)
	if Valid(file) {
		t.Fatal("a changed file is still valid")
	}
}

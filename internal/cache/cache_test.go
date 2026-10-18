package cache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilePath(t *testing.T) {
	c := &Cache{root: "/c"}
	cases := map[string]string{
		"https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/install.sh": "/c/files/raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/install.sh",
		"https://just.systems/install.sh":                                 "/c/files/just.systems/install.sh",
		"https://example.com/":                                            "/c/files/example.com/index",
		"https://example.com:8443/a/../../etc/passwd":                     "/c/files/example.com_8443/etc/passwd",
		"file:///x/y":                                                     "/c/files/local/x/y",
	}
	for url, want := range cases {
		t.Run(url, func(t *testing.T) {
			got, err := c.FilePath(url)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestFilePathQuery(t *testing.T) {
	c := &Cache{root: "/c"}
	q, _ := c.FilePath("https://example.com/f?v=1")
	q2, _ := c.FilePath("https://example.com/f?v=2")

	t.Run("hashed suffix", func(t *testing.T) {
		assert.Regexp(t, `^/c/files/example\.com/f@[0-9a-f]{12}$`, q)
	})
	t.Run("differs per query", func(t *testing.T) {
		assert.NotEqual(t, q, q2)
	})
}

func TestGitPath(t *testing.T) {
	c := &Cache{root: "/c"}
	for _, url := range []string{"https://github.com/tmux-plugins/tpm", "https://github.com/tmux-plugins/tpm.git"} {
		t.Run(url, func(t *testing.T) {
			got, err := c.GitPath(url)
			require.NoError(t, err)
			assert.Equal(t, "/c/git/github.com/tmux-plugins/tpm", got)
		})
	}
}

func TestGitPathWithoutRepository(t *testing.T) {
	_, err := (&Cache{root: "/c"}).GitPath("https://github.com/")

	assert.Error(t, err)
}

func TestOpen(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, nil, 0o644))

	cases := []struct {
		name  string
		root  string
		ro    bool
		fails bool
	}{
		{name: "creates missing root", root: filepath.Join(t.TempDir(), "cache")},
		{name: "no parent", root: filepath.Join(t.TempDir(), "a", "b"), fails: true},
		{name: "read-only missing root", root: filepath.Join(t.TempDir(), "cache"), ro: true, fails: true},
		{name: "not a directory", root: file, fails: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Open(tc.root, tc.ro)
			assert.Equal(t, tc.fails, err != nil, err)
		})
	}
}

func stored(t *testing.T) string {
	t.Helper()

	file := filepath.Join(t.TempDir(), "x", "f")
	tmp, err := Temp(file)
	require.NoError(t, err)
	_, err = tmp.WriteString("data")
	require.NoError(t, err)
	require.NoError(t, Store(tmp, file))

	return file
}

func TestStoredFileIsValid(t *testing.T) {
	assert.True(t, Valid(stored(t)))
}

func TestChangedFileIsInvalid(t *testing.T) {
	file := stored(t)
	require.NoError(t, os.WriteFile(file, []byte("other"), 0o644))

	assert.False(t, Valid(file))
}

func TestRelease(t *testing.T) {
	old := OSRelease
	t.Cleanup(func() { OSRelease = old })
	dir := t.TempDir()

	cases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "version id", content: "PRETTY_NAME=\"Debian GNU/Linux 12\"\nID=debian\nVERSION_ID=\"12\"\n", want: "debian-12-"},
		{name: "codename", content: "ID=debian\nVERSION_CODENAME=forky\n", want: "debian-forky-"},
		{name: "missing file", want: "unknown-"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			OSRelease = filepath.Join(dir, tc.name)
			if tc.content != "" {
				require.NoError(t, os.WriteFile(OSRelease, []byte(tc.content), 0o644))
			}
			assert.Equal(t, tc.want+debArch(), Release())
		})
	}
}

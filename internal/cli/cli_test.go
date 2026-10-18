package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func quiet(t *testing.T) {
	t.Helper()

	t.Setenv("NO_PROXY", "*")
	t.Setenv("NET_CACHE_DIR", t.TempDir())
	t.Setenv("NET_DELAY", "0")

	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	require.NoError(t, err)
	old := os.Stderr
	os.Stderr = devnull
	t.Cleanup(func() {
		os.Stderr = old
		devnull.Close()
	})
}

func serveText(t *testing.T, body string) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)

	return srv.URL
}

func captureStdout(t *testing.T, args ...string) (int, string) {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })

	old := os.Stdout
	os.Stdout = w
	rc := Run(args)
	os.Stdout = old
	require.NoError(t, w.Close())

	out, err := io.ReadAll(r)
	require.NoError(t, err)

	return rc, string(out)
}

func TestScriptRunsAndPropagatesTheExitCode(t *testing.T) {
	quiet(t)
	marker := filepath.Join(t.TempDir(), "argv")
	url := serveText(t, fmt.Sprintf("#!/bin/sh\nprintf '%%s' \"$*\" > %s\nexit 7\n", marker))

	rc := Run([]string{"script", "--shell", "/bin/sh", url, "--unattended", "--keep-zshrc"})

	t.Run("exit code", func(t *testing.T) {
		assert.Equal(t, 7, rc)
	})
	t.Run("arguments", func(t *testing.T) {
		argv, err := os.ReadFile(marker)
		require.NoError(t, err)
		assert.Equal(t, "--unattended --keep-zshrc", string(argv))
	})
}

func TestScriptLeavesNoTemporaryFile(t *testing.T) {
	quiet(t)
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)

	rc := Run([]string{"script", "--shell", "/bin/sh", serveText(t, "#!/bin/sh\nexit 0\n")})

	require.Equal(t, 0, rc)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestFetchToFile(t *testing.T) {
	quiet(t)
	dest := filepath.Join(t.TempDir(), "fetched")

	rc := Run([]string{"fetch", serveText(t, "payload"), dest})

	require.Equal(t, 0, rc)
	body, _ := os.ReadFile(dest)
	assert.Equal(t, "payload", string(body))
}

func TestDownloadWithMode(t *testing.T) {
	quiet(t)
	dest := filepath.Join(t.TempDir(), "nested", "installed")

	rc := Run([]string{"download", "--mode", "0755", serveText(t, "payload"), dest})

	require.Equal(t, 0, rc)
	st, err := os.Stat(dest)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), st.Mode().Perm())
}

func TestFetchOfMissingUrlReturnsTwentyTwo(t *testing.T) {
	quiet(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)

	rc := Run([]string{"fetch", srv.URL, filepath.Join(t.TempDir(), "x")})

	assert.Equal(t, 22, rc)
}

func runGit(t *testing.T, args ...string) {
	t.Helper()

	out, err := exec.Command("git", args...).CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

func originRepo(t *testing.T) (string, string) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}

	root := t.TempDir()
	bare := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")
	runGit(t, "init", "--bare", bare)
	runGit(t, "clone", bare, work)

	require.NoError(t, os.WriteFile(filepath.Join(work, "readme"), []byte("hi"), 0o644))
	runGit(t, "-C", work, "add", "readme")
	runGit(t, "-C", work, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "init")
	runGit(t, "-C", work, "push", "origin", "HEAD")

	return root, bare
}

func clonedWithMarker(t *testing.T) (string, string, string) {
	t.Helper()

	quiet(t)
	root, bare := originRepo(t)
	dest := filepath.Join(root, "cloned")
	require.Equal(t, 0, Run([]string{"clone", "file://" + bare, dest}))

	marker := filepath.Join(dest, "marker")
	require.NoError(t, os.WriteFile(marker, nil, 0o644))

	return bare, dest, marker
}

func TestCloneFetchesTheRepository(t *testing.T) {
	_, dest, _ := clonedWithMarker(t)

	assert.FileExists(t, filepath.Join(dest, "readme"))
}

func TestCloneSkipsExisting(t *testing.T) {
	bare, dest, marker := clonedWithMarker(t)

	rc := Run([]string{"clone", "file://" + bare, dest})

	require.Equal(t, 0, rc)
	assert.FileExists(t, marker)
}

func TestForcedCloneReplacesExisting(t *testing.T) {
	bare, dest, marker := clonedWithMarker(t)

	rc := Run([]string{"clone", "--force", "file://" + bare, dest})

	require.Equal(t, 0, rc)
	assert.NoFileExists(t, marker)
}

func TestDangerousCloneDestinationsAreRefused(t *testing.T) {
	quiet(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}

	for _, dest := range []string{"/", home, ""} {
		t.Run(dest, func(t *testing.T) {
			assert.Equal(t, 2, Run([]string{"clone", "--force", "https://example.com/x", dest}))
		})
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
		{"--envs", "extra"},
		{"download", "--mode", "0999", "u", "d"},
		{"fetch", "--cache-ro", "--force-update", "u", "d"},
	}
	for _, args := range cases {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			assert.Equal(t, 2, Run(args))
		})
	}
}

func TestBadEnvIsAUsageError(t *testing.T) {
	quiet(t)
	t.Setenv("NET_RETRIES", "many")

	assert.Equal(t, 2, Run([]string{"--envs"}))
}

func TestEnvsReportsSourceAndValue(t *testing.T) {
	quiet(t)
	t.Setenv("NET_RETRIES", "9")
	rc, out := captureStdout(t, "--envs")

	t.Run("exit code", func(t *testing.T) {
		assert.Equal(t, 0, rc)
	})
	t.Run("retries from env", func(t *testing.T) {
		assert.Contains(t, out, "NET_RETRIES=9 (env)")
	})
	t.Run("delay from env", func(t *testing.T) {
		assert.Contains(t, out, "NET_DELAY=0 (env)")
	})
}

func TestCompletionScriptMentionsTheBinary(t *testing.T) {
	quiet(t)

	rc, out := captureStdout(t, "completion", "bash")

	require.Equal(t, 0, rc)
	assert.Contains(t, out, "net-install __complete")
}

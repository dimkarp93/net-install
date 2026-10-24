package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dimkarp93/net-install/internal/cache"
	"github.com/dimkarp93/net-install/internal/config"
	"github.com/dimkarp93/net-install/internal/netlog"
)

type counter struct {
	srv  *httptest.Server
	hits atomic.Int64
	body func(n int64) string
}

func (c *counter) serveHTTP(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprint(w, c.body(c.hits.Add(1)))
}

func counting(t *testing.T, body func(n int64) string) *counter {
	t.Helper()

	c := &counter{body: body}
	c.srv = httptest.NewServer(http.HandlerFunc(c.serveHTTP))
	t.Cleanup(c.srv.Close)

	return c
}

func numbered(n int64) string { return fmt.Sprintf("v%d", n) }

func constant(body string) func(int64) string {
	return func(int64) string { return body }
}

func read(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(b)
}

func runOK(t *testing.T, args ...string) {
	t.Helper()

	require.Equal(t, 0, Run(args), "net-install %v", args)
}

func TestFetchIsServedFromTheCacheOnTheSecondRun(t *testing.T) {
	quiet(t)
	srv := counting(t, numbered)
	dir := t.TempDir()

	for i := range 2 {
		dest := filepath.Join(dir, fmt.Sprintf("out%d", i))
		runOK(t, "fetch", srv.srv.URL+"/tool.sh", dest)
		assert.Equal(t, "v1", read(t, dest), "run %d", i)
	}

	assert.Equal(t, int64(1), srv.hits.Load())
}

func TestForceUpdateDownloadsAgain(t *testing.T) {
	quiet(t)
	srv := counting(t, numbered)
	dest := filepath.Join(t.TempDir(), "out")
	Run([]string{"fetch", srv.srv.URL + "/f", dest})

	t.Run("flag", func(t *testing.T) {
		runOK(t, "fetch", "--force-update", srv.srv.URL+"/f", dest)
		assert.Equal(t, "v2", read(t, dest))
	})
	t.Run("env", func(t *testing.T) {
		t.Setenv("NET_FORCE_UPDATE", "1")
		Run([]string{"fetch", srv.srv.URL + "/f", dest})
		assert.Equal(t, "v3", read(t, dest))
	})
}

func corruptCached(t *testing.T, url string) {
	t.Helper()

	c, err := cache.Open(os.Getenv("NET_CACHE_DIR"), false)
	require.NoError(t, err)
	file, err := c.FilePath(url)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(file, []byte("broken"), 0o644))
}

func TestCorruptedCacheEntryIsDownloadedAgain(t *testing.T) {
	quiet(t)
	srv := counting(t, constant("payload"))
	dest := filepath.Join(t.TempDir(), "out")
	Run([]string{"fetch", srv.srv.URL + "/f", dest})
	corruptCached(t, srv.srv.URL+"/f")

	Run([]string{"fetch", srv.srv.URL + "/f", dest})

	t.Run("content", func(t *testing.T) {
		assert.Equal(t, "payload", read(t, dest))
	})
	t.Run("hits", func(t *testing.T) {
		assert.Equal(t, int64(2), srv.hits.Load())
	})
}

func TestForceUpdateFallsBackToTheCachedCopy(t *testing.T) {
	quiet(t)
	t.Setenv("NET_RETRIES", "1")
	srv := counting(t, constant("old"))
	dest := filepath.Join(t.TempDir(), "out")
	url := srv.srv.URL + "/f"
	Run([]string{"fetch", url, dest})
	srv.srv.Close()

	runOK(t, "fetch", "--force-update", url, dest)

	assert.Equal(t, "old", read(t, dest))
}

func TestNoCacheLeavesTheCacheEmpty(t *testing.T) {
	quiet(t)
	srv := counting(t, constant("x"))
	dest := filepath.Join(t.TempDir(), "out")

	for range 2 {
		runOK(t, "fetch", "--no-cache", srv.srv.URL+"/f", dest)
	}

	t.Run("hits", func(t *testing.T) {
		assert.Equal(t, int64(2), srv.hits.Load())
	})
	t.Run("cache empty", func(t *testing.T) {
		entries, _ := os.ReadDir(os.Getenv("NET_CACHE_DIR"))
		assert.Empty(t, entries)
	})
}

func TestUnavailableCacheDirIsAnError(t *testing.T) {
	quiet(t)
	srv := counting(t, constant("x"))
	missing := filepath.Join(t.TempDir(), "no", "such")

	rc := Run([]string{"fetch", "--cache-dir", missing, srv.srv.URL, filepath.Join(t.TempDir(), "out")})

	t.Run("exit code", func(t *testing.T) {
		assert.Equal(t, 1, rc)
	})
	t.Run("no download", func(t *testing.T) {
		assert.Equal(t, int64(0), srv.hits.Load())
	})
}

func TestScriptAndDownloadShareTheCache(t *testing.T) {
	quiet(t)
	srv := counting(t, constant("#!/bin/sh\nexit 3\n"))
	url := srv.srv.URL + "/install.sh"

	runOK(t, "download", url, filepath.Join(t.TempDir(), "i.sh"))
	rc := Run([]string{"script", url})

	t.Run("script exit code", func(t *testing.T) {
		assert.Equal(t, 3, rc)
	})
	t.Run("hits", func(t *testing.T) {
		assert.Equal(t, int64(1), srv.hits.Load())
	})
}

type origin struct {
	t    *testing.T
	url  string
	work string
}

func (o origin) commit(content string) {
	o.t.Helper()

	require.NoError(o.t, os.WriteFile(filepath.Join(o.work, "readme"), []byte(content), 0o644))
	runGit(o.t, "-C", o.work, "add", "readme")
	runGit(o.t, "-C", o.work, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", content)
	runGit(o.t, "-C", o.work, "push", "-q", "origin", "HEAD")
}

func gitRepo(t *testing.T, root string) origin {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}

	bare := filepath.Join(root, "origin.git")
	o := origin{t: t, url: "file://" + bare, work: filepath.Join(root, "work")}
	runGit(t, "init", "-q", "--bare", bare)
	runGit(t, "clone", "-q", bare, o.work)
	o.commit("v1")

	return o
}

func TestCloneKeepsTheOriginUrl(t *testing.T) {
	quiet(t)
	root := t.TempDir()
	o := gitRepo(t, root)
	first := filepath.Join(root, "first")
	runOK(t, "clone", o.url, first)

	out, err := exec.Command("git", "-C", first, "remote", "get-url", "origin").Output()

	require.NoError(t, err)
	assert.Equal(t, o.url, strings.TrimSpace(string(out)))
}

func TestCloneGoesThroughTheMirror(t *testing.T) {
	quiet(t)
	root := t.TempDir()
	o := gitRepo(t, root)
	runOK(t, "clone", o.url, filepath.Join(root, "first"))
	o.commit("v2")
	second := filepath.Join(root, "second")

	t.Run("mirror is reused", func(t *testing.T) {
		Run([]string{"clone", o.url, second})
		assert.Equal(t, "v1", read(t, filepath.Join(second, "readme")))
	})
	t.Run("force-update refreshes", func(t *testing.T) {
		runOK(t, "clone", "--force-update", o.url, second)
		assert.Equal(t, "v2", read(t, filepath.Join(second, "readme")))
	})
}

type fakeRunner struct{ calls [][]string }

func (f *fakeRunner) Run(name string, args ...string) int {
	f.calls = append(f.calls, append([]string{name}, args...))
	if len(args) > 0 && args[0] == "cp" {
		copyInto(args[3], args[4:])
	}
	return 0
}

func (f *fakeRunner) last() string {
	return strings.Join(f.calls[len(f.calls)-1], " ")
}

func copyInto(dst string, srcs []string) {
	for _, src := range srcs {
		b, _ := os.ReadFile(src)
		os.WriteFile(filepath.Join(dst, filepath.Base(src)), b, 0o644)
	}
}

func withArchives(t *testing.T) string {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("runs without sudo as root")
	}

	old := aptArchives
	aptArchives = t.TempDir()
	t.Cleanup(func() { aptArchives = old })

	return aptArchives
}

func withOSRelease(t *testing.T, content string) {
	t.Helper()

	osr := filepath.Join(t.TempDir(), "os-release")
	require.NoError(t, os.WriteFile(osr, []byte(content), 0o644))

	old := cache.OSRelease
	cache.OSRelease = osr
	t.Cleanup(func() { cache.OSRelease = old })
}

func aptEnv(t *testing.T, c *cache.Cache, cfg *config.Config) (*env, *fakeRunner) {
	t.Helper()

	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	require.NoError(t, err)
	t.Cleanup(func() { devnull.Close() })

	run := &fakeRunner{}
	return &env{cfg: cfg, cache: c, log: netlog.New(devnull, nil), run: run, out: devnull, err: devnull}, run
}

type aptFixture struct {
	e        *env
	run      *fakeRunner
	archives string
	aptDir   string
}

func newAptFixture(t *testing.T) aptFixture {
	t.Helper()

	archives := withArchives(t)
	withOSRelease(t, "ID=debian\nVERSION_ID=\"13\"\n")
	c, err := cache.Open(t.TempDir(), false)
	require.NoError(t, err)

	aptDir := c.AptDir(cache.Release())
	require.NoError(t, os.MkdirAll(aptDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(aptDir, "cached.deb"), []byte("c"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(archives, "fresh.deb"), []byte("f"), 0o644))

	e, run := aptEnv(t, c, config.New())
	return aptFixture{e: e, run: run, archives: archives, aptDir: aptDir}
}

func TestAptDirNamesTheRelease(t *testing.T) {
	f := newAptFixture(t)

	assert.Equal(t, "debian-13-"+debArchForTest(), filepath.Base(f.aptDir))
}

func aptRan(t *testing.T) aptFixture {
	t.Helper()

	f := newAptFixture(t)
	require.Equal(t, 0, cmdApt(f.e, []string{"zsh"}))

	return f
}

func TestAptPreloadsAndCollectsDebs(t *testing.T) {
	f := aptRan(t)

	t.Run("cached deb preloaded", func(t *testing.T) {
		assert.FileExists(t, filepath.Join(f.archives, "cached.deb"))
	})
	t.Run("downloaded deb collected", func(t *testing.T) {
		assert.FileExists(t, filepath.Join(f.aptDir, "fresh.deb"))
	})
	t.Run("apt call", func(t *testing.T) {
		assert.Regexp(t, `^sudo apt-get install -y .* zsh$`, f.run.last())
	})
}

func TestAptForceUpdateSkipsPreload(t *testing.T) {
	f := newAptFixture(t)
	f.e.cfg.ForceUpdate = true

	cmdApt(f.e, []string{"zsh"})

	assert.Len(t, f.run.calls, 1)
}

func debArchForTest() string {
	r := cache.Release()
	return r[strings.LastIndex(r, "-")+1:]
}

func TestReadOnlyCacheServesHits(t *testing.T) {
	quiet(t)
	srv := counting(t, numbered)
	dir := t.TempDir()
	Run([]string{"fetch", srv.srv.URL + "/cached", filepath.Join(dir, "a")})
	t.Setenv("NET_CACHE_RO", "1")

	runOK(t, "fetch", srv.srv.URL+"/cached", filepath.Join(dir, "b"))

	t.Run("content", func(t *testing.T) {
		assert.Equal(t, "v1", read(t, filepath.Join(dir, "b")))
	})
	t.Run("hits", func(t *testing.T) {
		assert.Equal(t, int64(1), srv.hits.Load())
	})
}

func TestReadOnlyCacheDoesNotStoreMisses(t *testing.T) {
	quiet(t)
	srv := counting(t, numbered)
	dest := filepath.Join(t.TempDir(), "c")
	t.Setenv("NET_CACHE_RO", "1")

	runOK(t, "fetch", srv.srv.URL+"/fresh", dest)

	t.Run("content", func(t *testing.T) {
		assert.Equal(t, "v1", read(t, dest))
	})
	t.Run("not stored", func(t *testing.T) {
		c, _ := cache.Open(os.Getenv("NET_CACHE_DIR"), true)
		file, _ := c.FilePath(srv.srv.URL + "/fresh")
		assert.NoFileExists(t, file)
	})
}

func readOnlyDir(t *testing.T) string {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	root := t.TempDir()
	require.NoError(t, os.Chmod(root, 0o555))
	t.Cleanup(func() { os.Chmod(root, 0o755) })

	return root
}

func TestReadOnlyCacheDir(t *testing.T) {
	quiet(t)
	srv := counting(t, constant("x"))
	root := readOnlyDir(t)
	dest := filepath.Join(t.TempDir(), "out")

	cases := []struct {
		name string
		args []string
		want int
	}{
		{name: "rw on a read-only dir", args: []string{"--cache-dir", root}, want: 1},
		{name: "ro on a read-only dir", args: []string{"--cache-dir", root, "--cache-ro"}, want: 0},
		{name: "ro with force-update", args: []string{"--cache-dir", root, "--cache-ro", "--force-update"}, want: 2},
		{name: "ro on a missing dir", args: []string{"--cache-dir", filepath.Join(root, "missing"), "--cache-ro"}, want: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string{"fetch"}, tc.args...), srv.srv.URL, dest)
			assert.Equal(t, tc.want, Run(args))
		})
	}
}

func TestReadOnlyCloneMissGoesToTheNetwork(t *testing.T) {
	quiet(t)
	root := t.TempDir()
	o := gitRepo(t, root)
	t.Setenv("NET_CACHE_RO", "1")
	dest := filepath.Join(root, "cloned")

	runOK(t, "clone", o.url, dest)

	t.Run("content", func(t *testing.T) {
		assert.Equal(t, "v1", read(t, filepath.Join(dest, "readme")))
	})
	t.Run("no mirror", func(t *testing.T) {
		entries, _ := os.ReadDir(os.Getenv("NET_CACHE_DIR"))
		assert.Empty(t, entries)
	})
}

func readOnlyApt(t *testing.T) (*env, string) {
	t.Helper()

	archives := withArchives(t)
	require.NoError(t, os.WriteFile(filepath.Join(archives, "fresh.deb"), []byte("f"), 0o644))

	root := t.TempDir()
	c, err := cache.Open(root, true)
	require.NoError(t, err)

	e, _ := aptEnv(t, c, config.New())
	return e, root
}

func TestReadOnlyAptDoesNotCollect(t *testing.T) {
	e, root := readOnlyApt(t)

	require.Equal(t, 0, cmdApt(e, []string{"zsh"}))

	entries, _ := os.ReadDir(root)
	assert.Empty(t, entries)
}

func TestForceSinceRefreshesOldEntries(t *testing.T) {
	quiet(t)
	srv := counting(t, numbered)
	dest := filepath.Join(t.TempDir(), "out")
	url := srv.srv.URL + "/f"
	Run([]string{"fetch", url, dest})

	runOK(t, "fetch", "--force-update", "--force-since", fmt.Sprint(time.Now().Unix()+3600), url, dest)

	assert.Equal(t, "v2", read(t, dest))
}

func TestForceSinceKeepsFreshEntries(t *testing.T) {
	quiet(t)
	srv := counting(t, numbered)
	dest := filepath.Join(t.TempDir(), "out")
	url := srv.srv.URL + "/f"
	Run([]string{"fetch", url, dest})

	Run([]string{"fetch", "--force-update", "--force-since", fmt.Sprint(time.Now().Unix() - 3600), url, dest})

	t.Run("content", func(t *testing.T) {
		assert.Equal(t, "v1", read(t, dest))
	})
	t.Run("hits", func(t *testing.T) {
		assert.Equal(t, int64(1), srv.hits.Load())
	})
}

func TestForceSinceKeepsAFreshMirror(t *testing.T) {
	quiet(t)
	root := t.TempDir()
	o := gitRepo(t, root)
	Run([]string{"clone", o.url, filepath.Join(root, "a")})
	o.commit("v2")
	dest := filepath.Join(root, "b")

	runOK(t, "clone", "--force-update", "--force-since", fmt.Sprint(time.Now().Unix()-3600), o.url, dest)

	assert.Equal(t, "v1", read(t, filepath.Join(dest, "readme")))
}

func TestAptDownloadOnly(t *testing.T) {
	withArchives(t)
	c, err := cache.Open(t.TempDir(), false)
	require.NoError(t, err)
	cfg := config.New()
	cfg.DownloadOnly = true
	e, run := aptEnv(t, c, cfg)

	require.Equal(t, 0, cmdApt(e, []string{"zsh"}))

	assert.True(t, strings.HasPrefix(run.last(), "sudo apt-get install -y --download-only "))
}

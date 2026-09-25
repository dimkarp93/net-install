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

	"github.com/dimkarp93/net-install/internal/cache"
	"github.com/dimkarp93/net-install/internal/config"
	"github.com/dimkarp93/net-install/internal/netlog"
)

func counting(t *testing.T, body func(n int64) string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body(hits.Add(1)))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestFetchIsServedFromTheCacheOnTheSecondRun(t *testing.T) {
	quiet(t)
	srv, hits := counting(t, func(n int64) string { return fmt.Sprintf("v%d", n) })
	dir := t.TempDir()

	for i := 0; i < 2; i++ {
		dest := filepath.Join(dir, fmt.Sprintf("out%d", i))
		if rc := Run([]string{"fetch", srv.URL + "/tool.sh", dest}); rc != 0 {
			t.Fatalf("rc=%d", rc)
		}
		if got := read(t, dest); got != "v1" {
			t.Fatalf("run %d got %q", i, got)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("server was hit %d times", hits.Load())
	}
}

func TestForceUpdateDownloadsAgain(t *testing.T) {
	quiet(t)
	srv, hits := counting(t, func(n int64) string { return fmt.Sprintf("v%d", n) })
	dest := filepath.Join(t.TempDir(), "out")

	Run([]string{"fetch", srv.URL + "/f", dest})
	if rc := Run([]string{"fetch", "--force-update", srv.URL + "/f", dest}); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if got := read(t, dest); got != "v2" || hits.Load() != 2 {
		t.Fatalf("got %q after %d hits", got, hits.Load())
	}

	t.Setenv("NET_FORCE_UPDATE", "1")
	Run([]string{"fetch", srv.URL + "/f", dest})
	if got := read(t, dest); got != "v3" {
		t.Fatalf("NET_FORCE_UPDATE ignored, got %q", got)
	}
}

func TestCorruptedCacheEntryIsDownloadedAgain(t *testing.T) {
	quiet(t)
	srv, hits := counting(t, func(n int64) string { return "payload" })
	dest := filepath.Join(t.TempDir(), "out")

	Run([]string{"fetch", srv.URL + "/f", dest})
	c, _ := cache.Open(os.Getenv("NET_CACHE_DIR"), false)
	file, _ := c.FilePath(srv.URL + "/f")
	if err := os.WriteFile(file, []byte("broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	Run([]string{"fetch", srv.URL + "/f", dest})
	if got := read(t, dest); got != "payload" || hits.Load() != 2 {
		t.Fatalf("got %q after %d hits", got, hits.Load())
	}
}

func TestForceUpdateFallsBackToTheCachedCopy(t *testing.T) {
	quiet(t)
	t.Setenv("NET_RETRIES", "1")
	srv, _ := counting(t, func(n int64) string { return "old" })
	dest := filepath.Join(t.TempDir(), "out")
	url := srv.URL + "/f"

	Run([]string{"fetch", url, dest})
	srv.Close()

	if rc := Run([]string{"fetch", "--force-update", url, dest}); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if got := read(t, dest); got != "old" {
		t.Fatalf("got %q", got)
	}
}

func TestNoCacheLeavesTheCacheEmpty(t *testing.T) {
	quiet(t)
	srv, hits := counting(t, func(n int64) string { return "x" })
	dest := filepath.Join(t.TempDir(), "out")

	for i := 0; i < 2; i++ {
		if rc := Run([]string{"fetch", "--no-cache", srv.URL + "/f", dest}); rc != 0 {
			t.Fatalf("rc=%d", rc)
		}
	}
	entries, _ := os.ReadDir(os.Getenv("NET_CACHE_DIR"))
	if hits.Load() != 2 || len(entries) != 0 {
		t.Fatalf("hits=%d cache entries=%d", hits.Load(), len(entries))
	}
}

func TestUnavailableCacheDirIsAnError(t *testing.T) {
	quiet(t)
	srv, hits := counting(t, func(n int64) string { return "x" })
	dest := filepath.Join(t.TempDir(), "out")
	missing := filepath.Join(t.TempDir(), "no", "such")

	if rc := Run([]string{"fetch", "--cache-dir", missing, srv.URL, dest}); rc != 1 {
		t.Fatalf("rc=%d, want 1", rc)
	}
	if hits.Load() != 0 {
		t.Fatal("downloaded despite an unavailable cache")
	}
}

func TestScriptAndDownloadShareTheCache(t *testing.T) {
	quiet(t)
	srv, hits := counting(t, func(n int64) string { return "#!/bin/sh\nexit 3\n" })
	url := srv.URL + "/install.sh"

	if rc := Run([]string{"download", url, filepath.Join(t.TempDir(), "i.sh")}); rc != 0 {
		t.Fatalf("download rc=%d", rc)
	}
	if rc := Run([]string{"script", url}); rc != 3 {
		t.Fatalf("script rc=%d", rc)
	}
	if hits.Load() != 1 {
		t.Fatalf("hits=%d", hits.Load())
	}
}

func gitRepo(t *testing.T, root string) (string, func(string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	bare := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "-q", "--bare", bare)
	git("clone", "-q", bare, work)
	commit := func(content string) {
		if err := os.WriteFile(filepath.Join(work, "readme"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		git("-C", work, "add", "readme")
		git("-C", work, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", content)
		git("-C", work, "push", "-q", "origin", "HEAD")
	}
	commit("v1")
	return "file://" + bare, commit
}

func TestCloneGoesThroughTheMirror(t *testing.T) {
	quiet(t)
	root := t.TempDir()
	url, commit := gitRepo(t, root)

	first := filepath.Join(root, "first")
	if rc := Run([]string{"clone", url, first}); rc != 0 {
		t.Fatalf("clone rc=%d", rc)
	}
	out, err := exec.Command("git", "-C", first, "remote", "get-url", "origin").Output()
	if err != nil || strings.TrimSpace(string(out)) != url {
		t.Fatalf("origin=%q err=%v", out, err)
	}

	commit("v2")
	second := filepath.Join(root, "second")
	Run([]string{"clone", url, second})
	if got := read(t, filepath.Join(second, "readme")); got != "v1" {
		t.Fatalf("mirror was not reused, got %q", got)
	}

	if rc := Run([]string{"clone", "--force-update", url, second}); rc != 0 {
		t.Fatalf("force-update rc=%d", rc)
	}
	if got := read(t, filepath.Join(second, "readme")); got != "v2" {
		t.Fatalf("force-update did not refresh, got %q", got)
	}
}

type fakeRunner struct{ calls [][]string }

func (f *fakeRunner) Run(name string, args ...string) int {
	f.calls = append(f.calls, append([]string{name}, args...))
	if len(args) > 0 && args[0] == "cp" {
		dst := args[3]
		for _, src := range args[4:] {
			b, _ := os.ReadFile(src)
			os.WriteFile(filepath.Join(dst, filepath.Base(src)), b, 0o644)
		}
	}
	return 0
}

func TestAptPreloadsAndCollectsDebs(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("runs without sudo as root")
	}
	archives := t.TempDir()
	old := aptArchives
	aptArchives = archives
	t.Cleanup(func() { aptArchives = old })

	osr := filepath.Join(t.TempDir(), "os-release")
	os.WriteFile(osr, []byte("ID=debian\nVERSION_ID=\"13\"\n"), 0o644)
	oldOSR := cache.OSRelease
	cache.OSRelease = osr
	t.Cleanup(func() { cache.OSRelease = oldOSR })

	c, err := cache.Open(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	aptDir := c.AptDir(cache.Release())
	if filepath.Base(aptDir) != "debian-13-"+debArchForTest() {
		t.Fatalf("apt dir %s", aptDir)
	}
	os.MkdirAll(aptDir, 0o755)
	os.WriteFile(filepath.Join(aptDir, "cached.deb"), []byte("c"), 0o644)
	os.WriteFile(filepath.Join(archives, "fresh.deb"), []byte("f"), 0o644)

	run := &fakeRunner{}
	devnull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	defer devnull.Close()
	e := &env{cfg: config.New(), cache: c, log: netlog.New(devnull, nil), run: run, out: devnull, err: devnull}

	if rc := cmdApt(e, []string{"zsh"}); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if _, err := os.Stat(filepath.Join(archives, "cached.deb")); err != nil {
		t.Fatal("cached deb was not preloaded")
	}
	if _, err := os.Stat(filepath.Join(aptDir, "fresh.deb")); err != nil {
		t.Fatal("downloaded deb was not collected")
	}
	last := strings.Join(run.calls[len(run.calls)-1], " ")
	if !strings.HasPrefix(last, "sudo apt-get install -y") || !strings.HasSuffix(last, " zsh") {
		t.Fatalf("apt call: %s", last)
	}

	run.calls = nil
	e.cfg.ForceUpdate = true
	os.Remove(filepath.Join(archives, "cached.deb"))
	cmdApt(e, []string{"zsh"})
	if len(run.calls) != 1 {
		t.Fatalf("force-update still preloaded: %v", run.calls)
	}
}

func debArchForTest() string {
	r := cache.Release()
	return r[strings.LastIndex(r, "-")+1:]
}

func TestReadOnlyCacheServesHitsAndDoesNotStoreMisses(t *testing.T) {
	quiet(t)
	srv, hits := counting(t, func(n int64) string { return fmt.Sprintf("v%d", n) })
	dir := t.TempDir()
	root := os.Getenv("NET_CACHE_DIR")

	Run([]string{"fetch", srv.URL + "/cached", filepath.Join(dir, "a")})

	t.Setenv("NET_CACHE_RO", "1")
	if rc := Run([]string{"fetch", srv.URL + "/cached", filepath.Join(dir, "b")}); rc != 0 {
		t.Fatalf("hit rc=%d", rc)
	}
	if got := read(t, filepath.Join(dir, "b")); got != "v1" || hits.Load() != 1 {
		t.Fatalf("ro hit got %q after %d hits", got, hits.Load())
	}

	if rc := Run([]string{"fetch", srv.URL + "/fresh", filepath.Join(dir, "c")}); rc != 0 {
		t.Fatalf("miss rc=%d", rc)
	}
	if got := read(t, filepath.Join(dir, "c")); got != "v2" {
		t.Fatalf("ro miss got %q", got)
	}
	c, _ := cache.Open(root, true)
	file, _ := c.FilePath(srv.URL + "/fresh")
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("a read-only cache stored a miss")
	}
}

func TestReadOnlyCacheWorksOnAReadOnlyDirAndRejectsForceUpdate(t *testing.T) {
	quiet(t)
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	srv, _ := counting(t, func(n int64) string { return "x" })
	root := t.TempDir()
	os.Chmod(root, 0o555)
	t.Cleanup(func() { os.Chmod(root, 0o755) })
	dest := filepath.Join(t.TempDir(), "out")

	if rc := Run([]string{"fetch", "--cache-dir", root, srv.URL, dest}); rc != 1 {
		t.Fatalf("rw on a read-only dir rc=%d, want 1", rc)
	}
	if rc := Run([]string{"fetch", "--cache-dir", root, "--cache-ro", srv.URL, dest}); rc != 0 {
		t.Fatalf("ro rc=%d", rc)
	}
	if rc := Run([]string{"fetch", "--cache-dir", root, "--cache-ro", "--force-update", srv.URL, dest}); rc != 2 {
		t.Fatalf("ro+force-update rc=%d, want 2", rc)
	}
	if rc := Run([]string{"fetch", "--cache-dir", filepath.Join(root, "missing"), "--cache-ro", srv.URL, dest}); rc != 1 {
		t.Fatalf("ro on a missing dir rc=%d, want 1", rc)
	}
}

func TestReadOnlyCloneMissGoesToTheNetwork(t *testing.T) {
	quiet(t)
	root := t.TempDir()
	url, _ := gitRepo(t, root)
	t.Setenv("NET_CACHE_RO", "1")

	dest := filepath.Join(root, "cloned")
	if rc := Run([]string{"clone", url, dest}); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	if got := read(t, filepath.Join(dest, "readme")); got != "v1" {
		t.Fatalf("got %q", got)
	}
	entries, _ := os.ReadDir(os.Getenv("NET_CACHE_DIR"))
	if len(entries) != 0 {
		t.Fatal("a read-only clone created a mirror")
	}
}

func TestReadOnlyAptDoesNotCollect(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("runs without sudo as root")
	}
	archives := t.TempDir()
	old := aptArchives
	aptArchives = archives
	t.Cleanup(func() { aptArchives = old })
	os.WriteFile(filepath.Join(archives, "fresh.deb"), []byte("f"), 0o644)

	root := t.TempDir()
	c, err := cache.Open(root, true)
	if err != nil {
		t.Fatal(err)
	}
	devnull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	defer devnull.Close()
	e := &env{cfg: config.New(), cache: c, log: netlog.New(devnull, nil), run: &fakeRunner{}, out: devnull, err: devnull}

	if rc := cmdApt(e, []string{"zsh"}); rc != 0 {
		t.Fatalf("rc=%d", rc)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("a read-only apt run collected debs")
	}
}

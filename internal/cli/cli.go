package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dimkarp93/net-install/internal/cache"
	"github.com/dimkarp93/net-install/internal/config"
	"github.com/dimkarp93/net-install/internal/httpget"
	"github.com/dimkarp93/net-install/internal/instfile"
	"github.com/dimkarp93/net-install/internal/netlog"
	"github.com/dimkarp93/net-install/internal/retry"
	"github.com/dimkarp93/net-install/internal/runner"
)

type env struct {
	cfg   *config.Config
	cache *cache.Cache
	log   *netlog.Logger
	run   runner.Runner
	out   io.Writer
	err   io.Writer
}

type localError struct{ err error }

func (e *localError) Error() string   { return e.err.Error() }
func (e *localError) Unwrap() error   { return e.err }
func (e *localError) ExitCode() int   { return 1 }
func (e *localError) Retryable() bool { return false }

type gitError struct{ rc int }

func (e *gitError) Error() string   { return fmt.Sprintf("git clone exited with %d", e.rc) }
func (e *gitError) ExitCode() int   { return e.rc }
func (e *gitError) Retryable() bool { return true }

var netSpec = spec{
	value: map[string]bool{
		config.Retries:        true,
		config.Delay:          true,
		config.ConnectTimeout: true,
		config.SpeedLimit:     true,
		config.SpeedTime:      true,
	},
	flag: map[string]bool{config.RetryAll: true},
}

var cacheSpec = spec{
	value: map[string]bool{config.CacheDir: true, config.ForceSince: true},
	flag:  map[string]bool{config.NoCache: true, config.ForceUpdate: true, config.CacheRO: true},
}

var aptArchives = "/var/cache/apt/archives"

func specFor(command string) (spec, bool) {
	s := spec{value: map[string]bool{}, flag: map[string]bool{}}
	for k, v := range netSpec.value {
		s.value[k] = v
	}
	for k, v := range netSpec.flag {
		s.flag[k] = v
	}
	for k, v := range cacheSpec.value {
		s.value[k] = v
	}
	for k, v := range cacheSpec.flag {
		s.flag[k] = v
	}

	switch command {
	case "fetch":
	case "apt":
		s.flag[config.DownloadOnly] = true
	case "download":
		s.value[config.Mode] = true
	case "script":
		s.value[config.Shell] = true
	case "clone":
		s.value[config.Depth] = true
		s.flag[config.Force] = true
	case "env":
		s.value[config.Mode] = true
		s.value[config.Shell] = true
		s.value[config.Depth] = true
		s.flag[config.Force] = true
	case "log":
		return spec{value: map[string]bool{}, flag: map[string]bool{}}, true
	default:
		return s, false
	}
	return s, true
}

func Run(args []string) int {
	e := &env{
		cfg: config.New(),
		log: netlog.New(os.Stderr, nil),
		run: runner.Exec{},
		out: os.Stdout,
		err: os.Stderr,
	}

	if len(args) == 0 {
		printUsage(e.err)
		return 2
	}

	if args[0] == "-h" || args[0] == "--help" {
		printUsage(e.out)
		return 0
	}

	command := args[0]
	s, ok := specFor(command)
	if !ok {
		fmt.Fprintf(e.err, "net-install: unknown command %q\n", command)
		printUsage(e.err)
		return 2
	}

	if err := e.cfg.LoadEnv(os.LookupEnv); err != nil {
		return usageError(e, err)
	}

	p, err := parse(args[1:], s)
	if err != nil {
		return usageError(e, err)
	}
	for _, f := range p.flags {
		if err := e.cfg.Set(f.name, f.value, config.SourceFlag); err != nil {
			return usageError(e, err)
		}
	}

	switch command {
	case "fetch", "download", "script", "clone", "apt":
		if !e.cfg.NoCache {
			if e.cfg.CacheRO && e.cfg.ForceUpdate {
				return usageError(e, fmt.Errorf("--force-update cannot refresh a --cache-ro cache"))
			}
			c, err := cache.Open(e.cfg.CacheDir, e.cfg.CacheRO)
			if err != nil {
				return fail(e, &localError{err})
			}
			e.cache = c
		}
	}

	switch command {
	case "fetch":
		return cmdFetch(e, p.args)
	case "download":
		return cmdDownload(e, p.args)
	case "script":
		return cmdScript(e, p.args)
	case "clone":
		return cmdClone(e, p.args)
	case "apt":
		return cmdApt(e, p.args)
	case "log":
		return cmdLog(e, p.args)
	case "env":
		return cmdEnv(e, p.args)
	}
	return 2
}

func usageError(e *env, err error) int {
	fmt.Fprintf(e.err, "net-install: %v\n", err)
	return 2
}

func fail(e *env, err error) int {
	fmt.Fprintf(e.err, "net-install: %v\n", err)
	return retry.ExitCode(err)
}

func newClient(c *config.Config) *httpget.Client {
	return httpget.New(httpget.Options{
		ConnectTimeout: time.Duration(c.ConnectTimeout) * time.Second,
		SpeedLimit:     c.SpeedLimit,
		SpeedTime:      c.SpeedTime,
	})
}

func retryOpts(c *config.Config) retry.Options {
	return retry.Options{Attempts: c.Retries, Delay: c.Delay, All: c.RetryAll}
}

func fetchInto(e *env, url string, f *os.File) error {
	client := newClient(e.cfg)
	var total int64

	err := retry.Do(e.log, retryOpts(e.cfg), "fetch "+url, func() error {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return &localError{err}
		}
		if err := f.Truncate(0); err != nil {
			return &localError{err}
		}
		n, err := client.Download(context.Background(), url, f)
		total = n
		return err
	})
	if err != nil {
		return err
	}

	e.log.Logf("event=size what=fetch url=%s bytes=%d", url, total)
	return nil
}

func obtain(e *env, url string) (string, func(), error) {
	if e.cache != nil {
		file, err := e.cache.FilePath(url)
		if err != nil {
			return "", nil, &localError{err}
		}
		have := cache.Valid(file)
		if have && (!e.cfg.ForceUpdate || fresh(e, file)) {
			e.log.Logf("event=cache-hit what=%s path=%s", url, file)
			return file, func() {}, nil
		}
		if !e.cache.ReadOnly() {
			file, err := storeFile(e, url, file, have)
			return file, func() {}, err
		}
		e.log.Logf("event=cache-miss what=%s path=%s", url, file)
	}

	f, err := os.CreateTemp("", "net-install-*")
	if err != nil {
		return "", nil, &localError{err}
	}
	name := f.Name()
	cleanup := func() { os.Remove(name) }

	if err := fetchInto(e, url, f); err != nil {
		f.Close()
		cleanup()
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, &localError{err}
	}
	return name, cleanup, nil
}

func storeFile(e *env, url, file string, have bool) (string, error) {
	tmp, err := cache.Temp(file)
	if err != nil {
		return "", &localError{err}
	}
	if err := fetchInto(e, url, tmp); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		if have {
			e.log.Logf("event=cache-stale what=%s path=%s", url, file)
			return file, nil
		}
		return "", err
	}
	if err := cache.Store(tmp, file); err != nil {
		os.Remove(tmp.Name())
		return "", &localError{err}
	}
	e.log.Logf("event=cache-store what=%s path=%s", url, file)
	return file, nil
}

func copyFrom(src string, w io.Writer) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

func cmdFetch(e *env, args []string) int {
	if len(args) != 2 {
		return usageError(e, fmt.Errorf("fetch requires URL and DEST"))
	}
	url, dest := args[0], args[1]

	src, cleanup, err := obtain(e, url)
	if err != nil {
		return fail(e, err)
	}
	defer cleanup()

	if dest == "-" {
		if err := copyFrom(src, e.out); err != nil {
			return fail(e, &localError{err})
		}
		return 0
	}

	f, err := os.Create(dest)
	if err != nil {
		return fail(e, &localError{err})
	}
	defer f.Close()

	if err := copyFrom(src, f); err != nil {
		return fail(e, &localError{err})
	}
	return 0
}

func cmdDownload(e *env, args []string) int {
	if len(args) != 2 {
		return usageError(e, fmt.Errorf("download requires URL and DEST"))
	}
	url, dest := args[0], args[1]

	mode, err := config.ParseMode(e.cfg.Mode)
	if err != nil {
		return usageError(e, err)
	}

	target, err := instfile.Open(dest, os.FileMode(mode))
	if err != nil {
		return fail(e, &localError{err})
	}
	defer target.Close()

	src, cleanup, err := obtain(e, url)
	if err != nil {
		return fail(e, err)
	}
	defer cleanup()

	if err := copyFrom(src, target.File()); err != nil {
		return fail(e, &localError{err})
	}

	rc := 0
	cerr := target.Commit(e.run)
	if cerr != nil {
		rc = 1
	}
	e.log.Logf("event=install what=%s mode=%s rc=%d", dest, e.cfg.Mode, rc)
	if cerr != nil {
		return fail(e, &localError{cerr})
	}
	return 0
}

func cmdScript(e *env, args []string) int {
	if len(args) < 1 {
		return usageError(e, fmt.Errorf("script requires a URL"))
	}
	url, rest := args[0], args[1:]

	src, cleanup, err := obtain(e, url)
	if err != nil {
		return fail(e, err)
	}
	defer cleanup()

	e.log.Logf("event=exec what=%s interpreter=%s", url, e.cfg.Shell)
	rc := e.run.Run(e.cfg.Shell, append([]string{src}, rest...)...)
	e.log.Logf("event=done what=%s rc=%d", url, rc)
	return rc
}

func gitNet(e *env, args ...string) []string {
	return append([]string{
		"-c", "http.lowSpeedLimit=" + strconv.Itoa(e.cfg.SpeedLimit),
		"-c", "http.lowSpeedTime=" + strconv.Itoa(e.cfg.SpeedTime),
	}, args...)
}

func cmdClone(e *env, args []string) int {
	if len(args) != 2 {
		return usageError(e, fmt.Errorf("clone requires URL and DEST"))
	}
	url, dest := args[0], args[1]

	if _, err := os.Stat(dest); err == nil && !e.cfg.Force && !e.cfg.ForceUpdate {
		e.log.Logf("event=skip what=clone dest=%s reason=exists", dest)
		return 0
	}

	if err := removable(dest); err != nil {
		return usageError(e, err)
	}

	if e.cache == nil {
		if err := cloneInto(e, url, dest, true); err != nil {
			return fail(e, err)
		}
		return 0
	}

	mirror, err := mirrorFor(e, url)
	if err != nil {
		return fail(e, err)
	}
	if mirror == "" {
		if err := cloneInto(e, url, dest, true); err != nil {
			return fail(e, err)
		}
		return 0
	}
	if err := cloneInto(e, mirror, dest, false); err != nil {
		return fail(e, err)
	}
	if rc := e.run.Run("git", "-C", dest, "remote", "set-url", "origin", url); rc != 0 {
		return fail(e, &gitError{rc})
	}
	return 0
}

func cloneInto(e *env, src, dest string, shallow bool) error {
	return retry.Do(e.log, retryOpts(e.cfg), "clone "+src, func() error {
		if err := os.RemoveAll(dest); err != nil {
			return &localError{err}
		}
		args := []string{"clone"}
		if shallow {
			args = append(args, "--depth", strconv.Itoa(e.cfg.Depth))
		}
		rc := e.run.Run("git", gitNet(e, append(args, src, dest)...)...)
		if rc != 0 {
			return &gitError{rc}
		}
		return nil
	})
}

func mirrorFor(e *env, url string) (string, error) {
	mirror, err := e.cache.GitPath(url)
	if err != nil {
		return "", &localError{err}
	}

	if st, err := os.Stat(filepath.Join(mirror, ".git")); err == nil && st.IsDir() {
		if !e.cfg.ForceUpdate || fresh(e, mirror) {
			e.log.Logf("event=cache-hit what=%s path=%s", url, mirror)
			return mirror, nil
		}
		if err := updateMirror(e, url, mirror); err != nil {
			e.log.Logf("event=cache-stale what=%s path=%s", url, mirror)
			return mirror, nil
		}
		touch(mirror)
		e.log.Logf("event=cache-store what=%s path=%s", url, mirror)
		return mirror, nil
	}

	if e.cache.ReadOnly() {
		e.log.Logf("event=cache-miss what=%s path=%s", url, mirror)
		return "", nil
	}

	if err := os.MkdirAll(filepath.Dir(mirror), 0o755); err != nil {
		return "", &localError{err}
	}
	tmp, err := os.MkdirTemp(filepath.Dir(mirror), ".net-install-*")
	if err != nil {
		return "", &localError{err}
	}
	defer os.RemoveAll(tmp)

	if err := cloneInto(e, url, tmp, true); err != nil {
		return "", err
	}
	if err := os.RemoveAll(mirror); err != nil {
		return "", &localError{err}
	}
	if err := os.Rename(tmp, mirror); err != nil {
		return "", &localError{err}
	}
	touch(mirror)
	e.log.Logf("event=cache-store what=%s path=%s", url, mirror)
	return mirror, nil
}

func updateMirror(e *env, url, mirror string) error {
	err := retry.Do(e.log, retryOpts(e.cfg), "fetch "+url, func() error {
		rc := e.run.Run("git", gitNet(e, "-C", mirror, "fetch", "--depth", strconv.Itoa(e.cfg.Depth), "origin")...)
		if rc != 0 {
			return &gitError{rc}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if rc := e.run.Run("git", "-C", mirror, "reset", "-q", "--hard", "FETCH_HEAD"); rc != 0 {
		return &gitError{rc}
	}
	return nil
}

func cmdApt(e *env, pkgs []string) int {
	if len(pkgs) == 0 {
		return usageError(e, fmt.Errorf("apt requires at least one package"))
	}

	if e.cache != nil && !e.cfg.ForceUpdate {
		if err := preloadDebs(e); err != nil {
			return fail(e, &localError{err})
		}
	}

	args := []string{"apt-get", "install", "-y"}
	if e.cfg.DownloadOnly {
		args = append(args, "--download-only")
	}
	args = append(append(args,
		"-o", "APT::Keep-Downloaded-Packages=true",
		"-o", "Acquire::Retries="+strconv.Itoa(e.cfg.Retries),
	), pkgs...)
	e.log.Logf("event=exec what=apt packages=%s", strings.Join(pkgs, ","))
	rc := runRoot(e, args...)
	e.log.Logf("event=done what=apt rc=%d", rc)
	if rc != 0 {
		return rc
	}

	if e.cache != nil && !e.cache.ReadOnly() {
		if err := collectDebs(e); err != nil {
			return fail(e, &localError{err})
		}
	}
	return 0
}

func fresh(e *env, path string) bool {
	if e.cfg.ForceSince <= 0 {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && st.ModTime().Unix() >= int64(e.cfg.ForceSince)
}

func touch(path string) {
	now := time.Now()
	os.Chtimes(path, now, now)
}

func runRoot(e *env, args ...string) int {
	if os.Geteuid() == 0 {
		return e.run.Run(args[0], args[1:]...)
	}
	return e.run.Run("sudo", args...)
}

func debs(dir string) (map[string]bool, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, en := range entries {
		if en.Type().IsRegular() && strings.HasSuffix(en.Name(), ".deb") {
			out[en.Name()] = true
		}
	}
	return out, nil
}

func preloadDebs(e *env) error {
	cached, err := debs(e.cache.AptDir(cache.Release()))
	if err != nil {
		return err
	}
	present, err := debs(aptArchives)
	if err != nil {
		return err
	}

	var missing []string
	for name := range cached {
		if !present[name] {
			missing = append(missing, filepath.Join(e.cache.AptDir(cache.Release()), name))
		}
	}
	if len(missing) == 0 {
		return nil
	}

	if rc := runRoot(e, append([]string{"cp", "-p", "-t", aptArchives}, missing...)...); rc != 0 {
		return fmt.Errorf("copying debs into %s exited with %d", aptArchives, rc)
	}
	e.log.Logf("event=cache-hit what=apt path=%s count=%d", e.cache.AptDir(cache.Release()), len(missing))
	return nil
}

func collectDebs(e *env) error {
	present, err := debs(aptArchives)
	if err != nil {
		return err
	}
	cached, err := debs(e.cache.AptDir(cache.Release()))
	if err != nil {
		return err
	}

	n := 0
	for name := range present {
		if cached[name] {
			continue
		}
		if err := cache.CopyFile(filepath.Join(aptArchives, name), filepath.Join(e.cache.AptDir(cache.Release()), name)); err != nil {
			return err
		}
		n++
	}
	if n > 0 {
		e.log.Logf("event=cache-store what=apt path=%s count=%d", e.cache.AptDir(cache.Release()), n)
	}
	return nil
}

func cmdLog(e *env, args []string) int {
	if len(args) == 0 {
		return usageError(e, fmt.Errorf("log requires at least one KEY=VALUE"))
	}
	e.log.Log(strings.Join(args, " "))
	return 0
}

func cmdEnv(e *env, args []string) int {
	if len(args) != 0 {
		return usageError(e, fmt.Errorf("env takes no arguments"))
	}
	for _, line := range e.cfg.Report() {
		fmt.Fprintln(e.out, line)
	}
	return 0
}

func removable(dest string) error {
	if strings.TrimSpace(dest) == "" {
		return fmt.Errorf("DEST is empty")
	}
	abs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	abs = filepath.Clean(abs)
	if abs == "/" {
		return fmt.Errorf("refusing to remove %s", abs)
	}
	if home, err := os.UserHomeDir(); err == nil && abs == filepath.Clean(home) {
		return fmt.Errorf("refusing to remove %s", abs)
	}
	return nil
}

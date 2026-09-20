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

	"github.com/dimkarp93/net-install/internal/config"
	"github.com/dimkarp93/net-install/internal/httpget"
	"github.com/dimkarp93/net-install/internal/instfile"
	"github.com/dimkarp93/net-install/internal/netlog"
	"github.com/dimkarp93/net-install/internal/retry"
	"github.com/dimkarp93/net-install/internal/runner"
)

type env struct {
	cfg *config.Config
	log *netlog.Logger
	run runner.Runner
	out io.Writer
	err io.Writer
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

func specFor(command string) (spec, bool) {
	s := spec{value: map[string]bool{}, flag: map[string]bool{}}
	for k, v := range netSpec.value {
		s.value[k] = v
	}
	for k, v := range netSpec.flag {
		s.flag[k] = v
	}

	switch command {
	case "fetch":
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
	case "fetch":
		return cmdFetch(e, p.args)
	case "download":
		return cmdDownload(e, p.args)
	case "script":
		return cmdScript(e, p.args)
	case "clone":
		return cmdClone(e, p.args)
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

func cmdFetch(e *env, args []string) int {
	if len(args) != 2 {
		return usageError(e, fmt.Errorf("fetch requires URL and DEST"))
	}
	url, dest := args[0], args[1]

	if dest == "-" {
		f, err := os.CreateTemp("", "net-install-*")
		if err != nil {
			return fail(e, &localError{err})
		}
		defer os.Remove(f.Name())
		defer f.Close()

		if err := fetchInto(e, url, f); err != nil {
			return fail(e, err)
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return fail(e, &localError{err})
		}
		if _, err := io.Copy(e.out, f); err != nil {
			return fail(e, &localError{err})
		}
		return 0
	}

	f, err := os.Create(dest)
	if err != nil {
		return fail(e, &localError{err})
	}
	defer f.Close()

	if err := fetchInto(e, url, f); err != nil {
		return fail(e, err)
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

	if err := fetchInto(e, url, target.File()); err != nil {
		return fail(e, err)
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

	f, err := os.CreateTemp("", "net-install-*")
	if err != nil {
		return fail(e, &localError{err})
	}
	defer os.Remove(f.Name())

	if err := fetchInto(e, url, f); err != nil {
		f.Close()
		return fail(e, err)
	}
	if err := f.Close(); err != nil {
		return fail(e, &localError{err})
	}

	e.log.Logf("event=exec what=%s interpreter=%s", url, e.cfg.Shell)
	rc := e.run.Run(e.cfg.Shell, append([]string{f.Name()}, rest...)...)
	e.log.Logf("event=done what=%s rc=%d", url, rc)
	return rc
}

func cmdClone(e *env, args []string) int {
	if len(args) != 2 {
		return usageError(e, fmt.Errorf("clone requires URL and DEST"))
	}
	url, dest := args[0], args[1]

	if _, err := os.Stat(dest); err == nil && !e.cfg.Force {
		e.log.Logf("event=skip what=clone dest=%s reason=exists", dest)
		return 0
	}

	if err := removable(dest); err != nil {
		return usageError(e, err)
	}

	err := retry.Do(e.log, retryOpts(e.cfg), "clone "+url, func() error {
		if err := os.RemoveAll(dest); err != nil {
			return &localError{err}
		}
		rc := e.run.Run("git",
			"-c", "http.lowSpeedLimit="+strconv.Itoa(e.cfg.SpeedLimit),
			"-c", "http.lowSpeedTime="+strconv.Itoa(e.cfg.SpeedTime),
			"clone", "--depth", strconv.Itoa(e.cfg.Depth), url, dest)
		if rc != 0 {
			return &gitError{rc}
		}
		return nil
	})
	if err != nil {
		return fail(e, err)
	}
	return 0
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

package config

import (
	"fmt"
	"io"
	"strconv"

	"github.com/dimkarp93/install-libs/envflag"
)

type Source = envflag.Source

const (
	SourceDefault = envflag.SourceDefault
	SourceEnv     = envflag.SourceEnv
	SourceFlag    = envflag.SourceFlag
)

const (
	Retries        = "retries"
	Delay          = "delay"
	ConnectTimeout = "connect-timeout"
	SpeedLimit     = "speed-limit"
	SpeedTime      = "speed-time"
	Shell          = "shell"
	Mode           = "mode"
	RetryAll       = "retry-all-errors"
	Depth          = "depth"
	Force          = "force"
	CacheDir       = "cache-dir"
	NoCache        = "no-cache"
	ForceUpdate    = "force-update"
	CacheRO        = "cache-ro"
	ForceSince     = "force-since"
	DownloadOnly   = "download-only"
)

const DefaultCacheDir = "/mnt/hdd/auto-distrib"

// Depth, Force and DownloadOnly are flag-only: they are not backed by an
// environment variable, so they are kept out of opts and tracked separately.
type Config struct {
	Retries        int
	Delay          int
	ConnectTimeout int
	SpeedLimit     int
	SpeedTime      int
	Shell          string
	Mode           string
	RetryAll       bool
	Depth          int
	Force          bool
	CacheDir       string
	NoCache        bool
	ForceUpdate    bool
	CacheRO        bool
	ForceSince     int
	DownloadOnly   bool

	opts   *envflag.Set
	source map[string]Source
}

func New() *Config {
	c := &Config{Depth: 1, source: map[string]Source{}}
	c.opts = envflag.New("NET_",
		c.intOption(Retries, "5", &c.Retries, 1),
		c.intOption(Delay, "5", &c.Delay, 0),
		c.intOption(ConnectTimeout, "20", &c.ConnectTimeout, 0),
		c.intOption(SpeedLimit, "1024", &c.SpeedLimit, 0),
		c.intOption(SpeedTime, "30", &c.SpeedTime, 0),
		envflag.Option{Name: Shell, Default: "sh", Parse: func(v string) error {
			c.Shell = v
			return nil
		}},
		envflag.Option{Name: Mode, Default: "0644", Parse: func(v string) error {
			if _, err := ParseMode(v); err != nil {
				return err
			}
			c.Mode = v
			return nil
		}},
		c.boolOption(RetryAll, "0", &c.RetryAll),
		envflag.Option{Name: CacheDir, Default: DefaultCacheDir, Parse: func(v string) error {
			if v == "" {
				return fmt.Errorf("%s: expected a path", CacheDir)
			}
			c.CacheDir = v
			return nil
		}},
		c.boolOption(NoCache, "0", &c.NoCache),
		c.boolOption(ForceUpdate, "0", &c.ForceUpdate),
		c.boolOption(CacheRO, "0", &c.CacheRO),
		c.intOption(ForceSince, "0", &c.ForceSince, 0),
	)
	c.Retries = 5
	c.Delay = 5
	c.ConnectTimeout = 20
	c.SpeedLimit = 1024
	c.SpeedTime = 30
	c.Shell = "sh"
	c.Mode = "0644"
	c.CacheDir = DefaultCacheDir
	return c
}

func (c *Config) intOption(name, def string, dst *int, min int) envflag.Option {
	return envflag.Option{Name: name, Default: def, Parse: func(v string) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("%s: expected an integer, got %q", name, v)
		}
		if n < min {
			return fmt.Errorf("%s: expected at least %d, got %d", name, min, n)
		}
		*dst = n
		return nil
	}}
}

func (c *Config) boolOption(name, def string, dst *bool) envflag.Option {
	return envflag.Option{Name: name, Default: def, Parse: func(v string) error {
		b, err := envflag.ParseBool(v)
		if err != nil {
			return err
		}
		*dst = b
		return nil
	}}
}

func (c *Config) LoadEnv(lookup func(string) (string, bool)) error {
	return c.opts.LoadEnv(lookup)
}

func (c *Config) Set(name, value string, src Source) error {
	switch name {
	case Depth:
		return c.setInt(&c.Depth, name, value, src, 0)
	case Force:
		return c.setBool(&c.Force, name, value, src)
	case DownloadOnly:
		return c.setBool(&c.DownloadOnly, name, value, src)
	}
	return c.opts.Set(name, value, src)
}

func (c *Config) setInt(dst *int, name, value string, src Source, min int) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("%s: expected an integer, got %q", name, value)
	}
	if n < min {
		return fmt.Errorf("%s: expected at least %d, got %d", name, min, n)
	}
	*dst = n
	c.source[name] = src
	return nil
}

func (c *Config) setBool(dst *bool, name, value string, src Source) error {
	b, err := envflag.ParseBool(value)
	if err != nil {
		return err
	}
	*dst = b
	c.source[name] = src
	return nil
}

func (c *Config) Source(name string) Source {
	switch name {
	case Depth, Force, DownloadOnly:
		if s, ok := c.source[name]; ok {
			return s
		}
		return SourceDefault
	}
	return c.opts.Source(name)
}

func (c *Config) Report() []string {
	return c.opts.Report()
}

func (c *Config) Fprint(w io.Writer) {
	c.opts.Fprint(w)
}

func (c *Config) HandleEnvs(w io.Writer, args []string) bool {
	return c.opts.HandleEnvs(w, args)
}

func ParseMode(s string) (uint32, error) {
	n, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("mode: expected an octal value, got %q", s)
	}
	return uint32(n), nil
}

package config

import (
	"fmt"
	"strconv"
)

type Source string

const (
	SourceDefault Source = "default"
	SourceEnv     Source = "env"
	SourceFlag    Source = "flag"
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
)

const DefaultCacheDir = "/mnt/hdd/auto-distrib"

var Env = map[string]string{
	Retries:        "NET_RETRIES",
	Delay:          "NET_DELAY",
	ConnectTimeout: "NET_CONNECT_TIMEOUT",
	SpeedLimit:     "NET_SPEED_LIMIT",
	SpeedTime:      "NET_SPEED_TIME",
	Shell:          "NET_SHELL",
	Mode:           "NET_MODE",
	RetryAll:       "NET_RETRY_ALL",
	CacheDir:       "NET_CACHE_DIR",
	NoCache:        "NET_NO_CACHE",
	ForceUpdate:    "NET_FORCE_UPDATE",
}

var order = []string{Retries, Delay, ConnectTimeout, SpeedLimit, SpeedTime, Shell, Mode, RetryAll, CacheDir, NoCache, ForceUpdate}

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

	source map[string]Source
}

func New() *Config {
	return &Config{
		Retries:        5,
		Delay:          5,
		ConnectTimeout: 20,
		SpeedLimit:     1024,
		SpeedTime:      30,
		Shell:          "sh",
		Mode:           "0644",
		Depth:          1,
		CacheDir:       DefaultCacheDir,
		source:         map[string]Source{},
	}
}

func (c *Config) LoadEnv(lookup func(string) (string, bool)) error {
	for _, name := range order {
		v, ok := lookup(Env[name])
		if !ok || v == "" {
			continue
		}
		if err := c.Set(name, v, SourceEnv); err != nil {
			return fmt.Errorf("%s: %w", Env[name], err)
		}
	}
	return nil
}

func (c *Config) Set(name, value string, src Source) error {
	switch name {
	case Retries:
		return c.setInt(&c.Retries, name, value, src, 1)
	case Delay:
		return c.setInt(&c.Delay, name, value, src, 0)
	case ConnectTimeout:
		return c.setInt(&c.ConnectTimeout, name, value, src, 0)
	case SpeedLimit:
		return c.setInt(&c.SpeedLimit, name, value, src, 0)
	case SpeedTime:
		return c.setInt(&c.SpeedTime, name, value, src, 0)
	case Depth:
		return c.setInt(&c.Depth, name, value, src, 0)
	case Shell:
		c.Shell = value
		c.source[name] = src
		return nil
	case Mode:
		if _, err := ParseMode(value); err != nil {
			return err
		}
		c.Mode = value
		c.source[name] = src
		return nil
	case RetryAll:
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		c.RetryAll = b
		c.source[name] = src
		return nil
	case Force:
		return c.setBool(&c.Force, name, value, src)
	case NoCache:
		return c.setBool(&c.NoCache, name, value, src)
	case ForceUpdate:
		return c.setBool(&c.ForceUpdate, name, value, src)
	case CacheDir:
		if value == "" {
			return fmt.Errorf("%s: expected a path", name)
		}
		c.CacheDir = value
		c.source[name] = src
		return nil
	}
	return fmt.Errorf("unknown option %q", name)
}

func (c *Config) setBool(dst *bool, name, value string, src Source) error {
	b, err := parseBool(value)
	if err != nil {
		return err
	}
	*dst = b
	c.source[name] = src
	return nil
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

func (c *Config) Source(name string) Source {
	if s, ok := c.source[name]; ok {
		return s
	}
	return SourceDefault
}

func (c *Config) Report() []string {
	out := make([]string, 0, len(order))
	for _, name := range order {
		out = append(out, fmt.Sprintf("%s=%s (%s)", Env[name], c.value(name), c.Source(name)))
	}
	return out
}

func (c *Config) value(name string) string {
	switch name {
	case Retries:
		return strconv.Itoa(c.Retries)
	case Delay:
		return strconv.Itoa(c.Delay)
	case ConnectTimeout:
		return strconv.Itoa(c.ConnectTimeout)
	case SpeedLimit:
		return strconv.Itoa(c.SpeedLimit)
	case SpeedTime:
		return strconv.Itoa(c.SpeedTime)
	case Shell:
		return c.Shell
	case Mode:
		return c.Mode
	case RetryAll:
		return boolString(c.RetryAll)
	case CacheDir:
		return c.CacheDir
	case NoCache:
		return boolString(c.NoCache)
	case ForceUpdate:
		return boolString(c.ForceUpdate)
	}
	return ""
}

func boolString(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func ParseMode(s string) (uint32, error) {
	n, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("mode: expected an octal value, got %q", s)
	}
	return uint32(n), nil
}

func parseBool(s string) (bool, error) {
	switch s {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("expected a boolean, got %q", s)
}

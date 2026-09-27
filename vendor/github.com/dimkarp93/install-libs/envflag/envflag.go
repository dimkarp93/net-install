package envflag

import (
	"fmt"
	"io"
	"strings"
)

type Source string

const (
	SourceDefault Source = "default"
	SourceEnv     Source = "env"
	SourceFlag    Source = "flag"
)

const EnvsFlag = "--envs"

type Option struct {
	Name    string
	Env     string
	Default string
	Parse   func(string) error
}

type Set struct {
	opts   []Option
	index  map[string]int
	values map[string]string
	source map[string]Source
}

func New(prefix string, opts ...Option) *Set {
	s := &Set{
		index:  make(map[string]int, len(opts)),
		values: make(map[string]string, len(opts)),
		source: make(map[string]Source, len(opts)),
	}
	for _, o := range opts {
		if o.Env == "" {
			o.Env = prefix + strings.ToUpper(strings.ReplaceAll(o.Name, "-", "_"))
		}
		if _, dup := s.index[o.Name]; dup {
			panic("envflag: duplicate option " + o.Name)
		}
		s.index[o.Name] = len(s.opts)
		s.opts = append(s.opts, o)
		s.values[o.Name] = o.Default
	}
	return s
}

func (s *Set) LoadEnv(lookup func(string) (string, bool)) error {
	for _, o := range s.opts {
		v, ok := lookup(o.Env)
		if !ok || v == "" {
			continue
		}
		if err := s.Set(o.Name, v, SourceEnv); err != nil {
			return fmt.Errorf("%s: %w", o.Env, err)
		}
	}
	return nil
}

func (s *Set) Set(name, value string, src Source) error {
	i, ok := s.index[name]
	if !ok {
		return fmt.Errorf("unknown option %q", name)
	}
	if p := s.opts[i].Parse; p != nil {
		if err := p(value); err != nil {
			return err
		}
	}
	s.values[name] = value
	s.source[name] = src
	return nil
}

func (s *Set) Has(name string) bool {
	_, ok := s.index[name]
	return ok
}

func (s *Set) Env(name string) string {
	if i, ok := s.index[name]; ok {
		return s.opts[i].Env
	}
	return ""
}

func (s *Set) Value(name string) string {
	return s.values[name]
}

func (s *Set) Source(name string) Source {
	if src, ok := s.source[name]; ok {
		return src
	}
	return SourceDefault
}

func (s *Set) Report() []string {
	out := make([]string, 0, len(s.opts))
	for _, o := range s.opts {
		out = append(out, fmt.Sprintf("%s=%s (%s)", o.Env, s.values[o.Name], s.Source(o.Name)))
	}
	return out
}

func (s *Set) Fprint(w io.Writer) {
	for _, line := range s.Report() {
		fmt.Fprintln(w, line)
	}
}

func (s *Set) HandleEnvs(w io.Writer, args []string) bool {
	if len(args) == 0 || args[0] != EnvsFlag {
		return false
	}
	s.Fprint(w)
	return true
}

func ParseBool(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("invalid boolean %q", v)
}

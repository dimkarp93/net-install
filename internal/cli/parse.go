package cli

import (
	"fmt"
	"strings"
)

type parsed struct {
	command string
	flags   []flag
	args    []string
}

type flag struct {
	name  string
	value string
}

type spec struct {
	value map[string]bool
	flag  map[string]bool
}

func (s spec) known(name string) bool {
	return s.value[name] || s.flag[name]
}

func parse(args []string, s spec) (*parsed, error) {
	p := &parsed{}

	for i := 0; i < len(args); i++ {
		a := args[i]

		if a == "--" {
			p.args = append(p.args, args[i+1:]...)
			return p, nil
		}

		if !strings.HasPrefix(a, "-") || a == "-" {
			p.args = append(p.args, args[i:]...)
			return p, nil
		}

		name, value, hasValue := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		if !strings.HasPrefix(a, "--") {
			return nil, fmt.Errorf("unknown flag: %s", a)
		}
		if !s.known(name) {
			return nil, fmt.Errorf("unknown flag: --%s", name)
		}

		if s.flag[name] {
			if !hasValue {
				value = "1"
			}
			p.flags = append(p.flags, flag{name, value})
			continue
		}

		if !hasValue {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--%s requires a value", name)
			}
			i++
			value = args[i]
		}
		p.flags = append(p.flags, flag{name, value})
	}

	return p, nil
}

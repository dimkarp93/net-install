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

		f, next, err := parseFlag(args, i, s)
		if err != nil {
			return nil, err
		}
		p.flags = append(p.flags, f)
		i = next
	}

	return p, nil
}

func parseFlag(args []string, i int, s spec) (flag, int, error) {
	a := args[i]
	name, value, hasValue := strings.Cut(strings.TrimPrefix(a, "--"), "=")
	if !strings.HasPrefix(a, "--") {
		return flag{}, i, fmt.Errorf("unknown flag: %s", a)
	}
	if !s.known(name) {
		return flag{}, i, fmt.Errorf("unknown flag: --%s", name)
	}

	switch {
	case hasValue:
		return flag{name, value}, i, nil
	case s.flag[name]:
		return flag{name, "1"}, i, nil
	case i+1 >= len(args):
		return flag{}, i, fmt.Errorf("--%s requires a value", name)
	default:
		return flag{name, args[i+1]}, i + 1, nil
	}
}

package shellcomplete

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func (s Spec) Handle(stdout, stderr io.Writer, args []string) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	switch args[0] {
	case CompleteCommand:
		for _, item := range s.Complete(args[1:]) {
			fmt.Fprintln(stdout, item)
		}
		return 0, true
	case CompletionCommand:
		if len(args) != 2 {
			fmt.Fprintf(stderr, "%s: %s takes exactly one argument: %s\n", s.Bin, CompletionCommand, strings.Join(Shells, " or "))
			return 2, true
		}
		script, err := s.Script(args[1])
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", s.Bin, err)
			return 2, true
		}
		fmt.Fprint(stdout, script)
		return 0, true
	case InstallCommand, UninstallCommand:
		return s.setup(stdout, stderr, args[0], args[1:]), true
	}
	return 0, false
}

func (s Spec) setup(stdout, stderr io.Writer, cmd string, args []string) int {
	dryRun := false
	var free []string
	for _, a := range args {
		switch a {
		case "--dry-run", "-dry-run":
			dryRun = true
		case "-h", "--help", "-help":
			fmt.Fprintf(stdout, "usage: %s %s [--dry-run] [%s]\n", s.Bin, cmd, strings.Join(Targets, "|"))
			return 0
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(stderr, "%s: %s: unknown flag %s\n", s.Bin, cmd, a)
				return 2
			}
			free = append(free, a)
		}
	}
	if len(free) > 1 {
		fmt.Fprintf(stderr, "%s: %s: extra arguments: %s\n", s.Bin, cmd, strings.Join(free[1:], " "))
		return 2
	}
	target := ShellFromEnv(os.Getenv("SHELL"))
	if len(free) == 1 {
		target = free[0]
	}
	run := s.Install
	if cmd == UninstallCommand {
		run = s.Uninstall
	}
	if err := run(target, dryRun, stdout); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", s.Bin, err)
		return 1
	}
	return 0
}

package runner

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

type Runner interface {
	Run(name string, args ...string) int
}

type Exec struct{}

func (Exec) Run(name string, args ...string) int {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return code(cmd.Run())
}

func code(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return ee.ExitCode()
	}
	return 127
}

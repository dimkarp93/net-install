package instfile

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/dimkarp93/net-install/internal/runner"
)

const wok = 0x2

type Target struct {
	dest  string
	mode  os.FileMode
	sudo  bool
	file  *os.File
	moved bool
}

func Open(dest string, mode os.FileMode) (*Target, error) {
	if st, err := os.Stat(dest); err == nil && st.IsDir() {
		return nil, fmt.Errorf("%s is a directory", dest)
	}

	dir := filepath.Dir(dest)
	probe := dir
	if _, err := os.Stat(dir); err != nil {
		probe = filepath.Dir(dir)
	}

	t := &Target{dest: dest, mode: mode, sudo: !writable(probe)}

	var err error
	if t.sudo {
		t.file, err = os.CreateTemp("", "net-install-*")
	} else {
		if err = os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		t.file, err = os.CreateTemp(dir, ".net-install-*")
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (t *Target) File() *os.File { return t.file }

func (t *Target) Truncate() error {
	if _, err := t.file.Seek(0, 0); err != nil {
		return err
	}
	return t.file.Truncate(0)
}

func (t *Target) Commit(run runner.Runner) error {
	if err := t.file.Chmod(t.mode); err != nil {
		return err
	}
	if err := t.file.Sync(); err != nil {
		return err
	}
	name := t.file.Name()
	if err := t.file.Close(); err != nil {
		return err
	}

	if !t.sudo {
		if err := os.Rename(name, t.dest); err != nil {
			return err
		}
		t.moved = true
		return nil
	}

	if rc := run.Run("sudo", "install", "-D", "-m", fmt.Sprintf("%04o", t.mode), name, t.dest); rc != 0 {
		return fmt.Errorf("sudo install exited with %d", rc)
	}
	return nil
}

func (t *Target) Close() {
	name := t.file.Name()
	t.file.Close()
	if !t.moved {
		os.Remove(name)
	}
}

func writable(path string) bool {
	return syscall.Access(path, wok) == nil
}

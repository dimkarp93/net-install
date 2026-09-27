package shellcomplete

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dimkarp93/install-libs/xdgpath"
)

type paths struct {
	bashFile string
	zshDir   string
	zshFile  string
	bashRC   string
	zshRC    string
}

func (s Spec) paths() (paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return paths{}, err
	}
	data := xdgpath.DataHome()
	zshDir := filepath.Join(data, "zsh", "site-functions")
	return paths{
		bashFile: filepath.Join(data, "bash-completion", "completions", s.Bin),
		zshDir:   zshDir,
		zshFile:  filepath.Join(zshDir, "_"+s.Bin),
		bashRC:   filepath.Join(home, ".bashrc"),
		zshRC:    filepath.Join(home, ".zshrc"),
	}, nil
}

func (s Spec) marker() string {
	return fmt.Sprintf("# %s completion (%s %s)", s.Bin, s.Bin, InstallCommand)
}

func (s Spec) rcLine(shell string, p paths) string {
	if shell == "zsh" {
		return fmt.Sprintf("fpath=(%s $fpath); autoload -Uz compinit && compinit -u   %s", p.zshDir, s.marker())
	}
	return fmt.Sprintf("[ -r \"%s\" ] && . \"%s\"   %s", p.bashFile, p.bashFile, s.marker())
}

func shellsOf(target string) ([]string, error) {
	if !slices.Contains(Targets, target) {
		return nil, fmt.Errorf("unknown shell %q, available: %s", target, strings.Join(Targets, ", "))
	}
	if target == "all" {
		return Shells, nil
	}
	return []string{target}, nil
}

func ShellFromEnv(value string) string {
	switch filepath.Base(strings.TrimSpace(value)) {
	case "bash":
		return "bash"
	case "zsh":
		return "zsh"
	}
	return "all"
}

func (s Spec) Install(target string, dryRun bool, w io.Writer) error {
	shells, err := shellsOf(target)
	if err != nil {
		return err
	}
	p, err := s.paths()
	if err != nil {
		return err
	}
	for _, sh := range shells {
		script, _ := s.Script(sh)
		file, rc := p.bashFile, p.bashRC
		if sh == "zsh" {
			file, rc = p.zshFile, p.zshRC
		}
		if dryRun {
			fmt.Fprintf(w, "Would write: %s\n", file)
		} else {
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(file, []byte(script), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(w, "Installed: %s\n", file)
		}
		if err := s.updateRC(rc, s.rcLine(sh, p), dryRun, w); err != nil {
			return err
		}
	}
	if !dryRun {
		fmt.Fprintln(w, "Open a new shell or run: exec $SHELL -l")
	}
	return nil
}

func (s Spec) Uninstall(target string, dryRun bool, w io.Writer) error {
	shells, err := shellsOf(target)
	if err != nil {
		return err
	}
	p, err := s.paths()
	if err != nil {
		return err
	}
	for _, sh := range shells {
		file, rc := p.bashFile, p.bashRC
		if sh == "zsh" {
			file, rc = p.zshFile, p.zshRC
		}
		if dryRun {
			fmt.Fprintf(w, "Would remove: %s\n", file)
		} else {
			if err := os.Remove(file); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			fmt.Fprintf(w, "Removed: %s\n", file)
		}
		if err := s.cleanRC(rc, dryRun, w); err != nil {
			return err
		}
	}
	return nil
}

func (s Spec) updateRC(rc, line string, dryRun bool, w io.Writer) error {
	text, mode, err := readRC(rc)
	if err != nil {
		return err
	}
	next, action := applyRCLine(text, line, s.marker())
	switch action {
	case rcSame:
		fmt.Fprintf(w, "Already present in %s\n", rc)
		return nil
	case rcReplaced:
		fmt.Fprintf(w, "Replaced the previous line in %s\n", rc)
	}
	if dryRun {
		fmt.Fprintf(w, "Would append to %s: %s\n", rc, line)
		return nil
	}
	if err := os.WriteFile(rc, []byte(next), mode); err != nil {
		return err
	}
	fmt.Fprintf(w, "Appended to %s: %s\n", rc, line)
	return nil
}

func (s Spec) cleanRC(rc string, dryRun bool, w io.Writer) error {
	text, mode, err := readRC(rc)
	if err != nil {
		return err
	}
	next, removed := stripMarked(text, s.marker())
	if !removed {
		fmt.Fprintf(w, "No marked line in %s, if you edited it by hand remove it yourself\n", rc)
		return nil
	}
	if dryRun {
		fmt.Fprintf(w, "Would remove the marked line from %s\n", rc)
		return nil
	}
	if err := os.WriteFile(rc, []byte(next), mode); err != nil {
		return err
	}
	fmt.Fprintf(w, "Removed the marked line from %s\n", rc)
	return nil
}

func readRC(rc string) (string, os.FileMode, error) {
	mode := os.FileMode(0o644)
	info, err := os.Stat(rc)
	if err == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", mode, err
	}
	data, err := os.ReadFile(rc)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", mode, nil
		}
		return "", mode, err
	}
	return string(data), mode, nil
}

const (
	rcSame = iota
	rcAdded
	rcReplaced
)

func applyRCLine(text, line, marker string) (string, int) {
	for _, item := range strings.Split(text, "\n") {
		if item == line {
			return text, rcSame
		}
	}
	next, removed := stripMarked(text, marker)
	if next != "" && !strings.HasSuffix(next, "\n") {
		next += "\n"
	}
	next += "\n" + line + "\n"
	if removed {
		return next, rcReplaced
	}
	return next, rcAdded
}

func stripMarked(text, marker string) (string, bool) {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	removed := false
	blanks := 0
	for _, item := range lines {
		if strings.TrimSpace(item) == "" {
			blanks++
			continue
		}
		if strings.Contains(item, marker) {
			removed = true
			if blanks > 0 {
				blanks--
			}
			out = appendBlanks(out, blanks)
			blanks = 0
			continue
		}
		out = appendBlanks(out, blanks)
		blanks = 0
		out = append(out, item)
	}
	out = appendBlanks(out, blanks)
	return strings.Join(out, "\n"), removed
}

func appendBlanks(out []string, n int) []string {
	for i := 0; i < n; i++ {
		out = append(out, "")
	}
	return out
}

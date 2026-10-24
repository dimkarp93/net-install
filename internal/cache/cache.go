package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

const wok = 0x2

type Cache struct {
	root string
	ro   bool
}

func Open(root string, ro bool) (*Cache, error) {
	if reason := checkRoot(root, ro); reason != "" {
		return nil, fmt.Errorf("cache dir %s is unavailable (%s): pass --cache-dir DIR or --no-cache", root, reason)
	}

	return &Cache{root: root, ro: ro}, nil
}

func checkRoot(root string, ro bool) string {
	st, err := os.Stat(root)
	switch {
	case err == nil && !st.IsDir():
		return "not a directory"
	case os.IsNotExist(err) && ro:
		return "does not exist"
	case os.IsNotExist(err):
		if reason := createRoot(root); reason != "" {
			return reason
		}
	case err != nil:
		return err.Error()
	}

	if !ro && syscall.Access(root, wok) != nil {
		return "not writable"
	}
	return ""
}

func createRoot(root string) string {
	if _, err := os.Stat(filepath.Dir(root)); err != nil {
		return "parent does not exist"
	}

	if err := os.Mkdir(root, 0o755); err != nil && !os.IsExist(err) {
		return err.Error()
	}
	return ""
}

func (c *Cache) Root() string { return c.root }

func (c *Cache) ReadOnly() bool { return c.ro }

func (c *Cache) AptDir(release string) string { return filepath.Join(c.root, "apt", release) }

func (c *Cache) FilePath(raw string) (string, error) {
	host, p, query, err := split(raw)
	if err != nil {
		return "", err
	}
	if p == "" || strings.HasSuffix(p, "/") {
		p += "index"
	}
	if query != "" {
		sum := sha256.Sum256([]byte(query))
		p += "@" + hex.EncodeToString(sum[:])[:12]
	}
	return filepath.Join(c.root, "files", host, filepath.FromSlash(p)), nil
}

func (c *Cache) GitPath(raw string) (string, error) {
	host, p, _, err := split(raw)
	if err != nil {
		return "", err
	}
	p = strings.TrimSuffix(strings.TrimSuffix(p, "/"), ".git")
	if p == "" {
		return "", fmt.Errorf("%s: no repository path", raw)
	}
	return filepath.Join(c.root, "git", host, filepath.FromSlash(p)), nil
}

func split(raw string) (host, p, query string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", "", err
	}

	return cacheHost(u.Host), cachePath(u.Path), u.RawQuery, nil
}

func cacheHost(host string) string {
	if host == "" {
		return "local"
	}

	return strings.ReplaceAll(host, ":", "_")
}

func cachePath(raw string) string {
	p := strings.TrimPrefix(path.Clean("/"+raw), "/")
	switch {
	case p == ".":
		return ""
	case strings.HasSuffix(raw, "/") && p != "":
		return p + "/"
	default:
		return p
	}
}

func Valid(file string) bool {
	want, err := os.ReadFile(file + ".sha256")
	if err != nil {
		return false
	}
	got, err := Sum(file)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(want)) == got
}

func Sum(file string) (string, error) {
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func Temp(file string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return nil, err
	}
	return os.CreateTemp(filepath.Dir(file), ".net-install-*")
}

func Store(tmp *os.File, file string) error {
	name := tmp.Name()
	if err := finishTemp(tmp); err != nil {
		return err
	}

	sum, err := Sum(name)
	if err != nil {
		return err
	}

	if err := os.Rename(name, file); err != nil {
		return err
	}
	return os.WriteFile(file+".sha256", []byte(sum+"\n"), 0o644)
}

func finishTemp(tmp *os.File) error {
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Chmod(tmp.Name(), 0o644)
}

func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := Temp(dst)
	if err != nil {
		return err
	}

	if err := fillTemp(out, in); err != nil {
		os.Remove(out.Name())
		return err
	}
	return os.Rename(out.Name(), dst)
}

func fillTemp(out *os.File, in io.Reader) error {
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}

	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(out.Name(), 0o644)
}

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
}

func Open(root string) (*Cache, error) {
	unavailable := func(reason string) error {
		return fmt.Errorf("cache dir %s is unavailable (%s): pass --cache-dir DIR or --no-cache", root, reason)
	}

	st, err := os.Stat(root)
	switch {
	case err == nil && !st.IsDir():
		return nil, unavailable("not a directory")
	case os.IsNotExist(err):
		if _, perr := os.Stat(filepath.Dir(root)); perr != nil {
			return nil, unavailable("parent does not exist")
		}
		if err := os.Mkdir(root, 0o755); err != nil && !os.IsExist(err) {
			return nil, unavailable(err.Error())
		}
	case err != nil:
		return nil, unavailable(err.Error())
	}

	if syscall.Access(root, wok) != nil {
		return nil, unavailable("not writable")
	}
	return &Cache{root: root}, nil
}

func (c *Cache) Root() string { return c.root }

func (c *Cache) AptDir() string { return filepath.Join(c.root, "apt") }

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
	host = u.Host
	if host == "" {
		host = "local"
	}
	host = strings.ReplaceAll(host, ":", "_")
	p = path.Clean("/" + u.Path)
	p = strings.TrimPrefix(p, "/")
	if p == "." {
		p = ""
	}
	if strings.HasSuffix(u.Path, "/") && p != "" {
		p += "/"
	}
	return host, p, u.RawQuery, nil
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
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
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
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(out.Name())
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(out.Name())
		return err
	}
	if err := os.Chmod(out.Name(), 0o644); err != nil {
		os.Remove(out.Name())
		return err
	}
	return os.Rename(out.Name(), dst)
}

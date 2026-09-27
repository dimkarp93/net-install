package xdgpath

import (
	"os"
	"path/filepath"
	"strings"
)

func home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

func base(env string, fallback ...string) string {
	if v := strings.TrimSpace(os.Getenv(env)); v != "" {
		return v
	}
	return filepath.Join(append([]string{home()}, fallback...)...)
}

func ConfigHome() string {
	return base("XDG_CONFIG_HOME", ".config")
}

func DataHome() string {
	return base("XDG_DATA_HOME", ".local", "share")
}

func ConfigDir(app string) string {
	return filepath.Join(ConfigHome(), app)
}

func ExpandHome(p string) string {
	if p == "~" {
		if h := home(); h != "" {
			return h
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		h := home()
		if h == "" {
			return p
		}
		return filepath.Join(h, p[2:])
	}
	return p
}

func Resolve(override, def string) string {
	if strings.TrimSpace(override) != "" {
		return ExpandHome(override)
	}
	return def
}

func WithLegacy(primary string, legacy ...string) string {
	if exists(primary) {
		return primary
	}
	for _, p := range legacy {
		if p != "" && p != primary && exists(p) {
			return p
		}
	}
	return primary
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

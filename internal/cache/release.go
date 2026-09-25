package cache

import (
	"bufio"
	"os"
	"runtime"
	"strings"
)

var OSRelease = "/etc/os-release"

func Release() string {
	id, version := "unknown", ""
	if f, err := os.Open(OSRelease); err == nil {
		defer f.Close()
		fields := map[string]string{}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			k, v, ok := strings.Cut(sc.Text(), "=")
			if ok {
				fields[k] = strings.Trim(v, `"'`)
			}
		}
		if fields["ID"] != "" {
			id = fields["ID"]
		}
		version = fields["VERSION_ID"]
		if version == "" {
			version = fields["VERSION_CODENAME"]
		}
	}

	parts := []string{id}
	if version != "" {
		parts = append(parts, version)
	}
	parts = append(parts, debArch())
	return strings.NewReplacer("/", "_", " ", "_").Replace(strings.Join(parts, "-"))
}

func debArch() string {
	switch runtime.GOARCH {
	case "386":
		return "i386"
	case "arm":
		return "armhf"
	}
	return runtime.GOARCH
}

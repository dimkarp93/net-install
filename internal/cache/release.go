package cache

import (
	"bufio"
	"os"
	"runtime"
	"strings"
)

var OSRelease = "/etc/os-release"

func Release() string {
	fields := osReleaseFields(OSRelease)

	parts := []string{releaseID(fields)}
	if version := releaseVersion(fields); version != "" {
		parts = append(parts, version)
	}
	parts = append(parts, debArch())

	return strings.NewReplacer("/", "_", " ", "_").Replace(strings.Join(parts, "-"))
}

func osReleaseFields(path string) map[string]string {
	fields := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return fields
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "="); ok {
			fields[k] = strings.Trim(v, `"'`)
		}
	}
	return fields
}

func releaseID(fields map[string]string) string {
	if id := fields["ID"]; id != "" {
		return id
	}

	return "unknown"
}

func releaseVersion(fields map[string]string) string {
	if version := fields["VERSION_ID"]; version != "" {
		return version
	}

	return fields["VERSION_CODENAME"]
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

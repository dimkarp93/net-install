package main

import (
	"fmt"
	"os"

	"github.com/dimkarp93/install-libs/buildinfo"
)

var (
	version  string
	origin   string
	upstream string
	commit   string
	channel  string
)

func build() buildinfo.Info {
	return buildinfo.Info{
		Version:  version,
		Origin:   origin,
		Upstream: upstream,
		Commit:   commit,
		Channel:  channel,
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, `Usage: net-install [FLAGS]

  --version, -v   print the version
  --origin        print the repository the binary was built from
  --buildinfo     print the full build info
  -h, --help      show this help
`)
}

func run() {
	for _, a := range os.Args[1:] {
		if a == "-h" || a == "--help" {
			usage(os.Stdout)
			return
		}
	}
	usage(os.Stderr)
	os.Exit(2)
}

func main() {
	if build().Handle(os.Args[1:]) {
		return
	}
	run()
}

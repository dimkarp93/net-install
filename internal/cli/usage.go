package cli

import (
	"fmt"
	"io"
)

const usageText = `Usage: net-install <command> [flags] [arguments]

Downloads with retries and a structured log on stderr.

Commands:
  fetch [flags] URL DEST        download URL into DEST ("-" writes to stdout)
  download [flags] URL DEST     download into a temporary file, then install it into DEST
  script [flags] URL [ARG...]   download a script and run it; ARG go to the script
  clone [flags] URL DEST        git clone --depth 1 with retries; skips an existing DEST
  apt [flags] PKG...            sudo apt-get install -y PKG, reusing cached .deb files;
                                --download-only fetches them without installing
  log KEY=VALUE...              print one log line, no network
  env                           print the effective NET_* values and where they come from

Network flags (fetch, download, script, clone):
  --retries N            number of attempts            (NET_RETRIES, default 5)
  --delay SEC            pause between attempts        (NET_DELAY, default 5)
  --connect-timeout SEC  connection timeout            (NET_CONNECT_TIMEOUT, default 20)
  --speed-limit BYTES    speed threshold, bytes/sec    (NET_SPEED_LIMIT, default 1024)
  --speed-time SEC       time below the threshold      (NET_SPEED_TIME, default 30)
  --retry-all-errors     retry 4xx as well             (NET_RETRY_ALL)

Cache flags (fetch, download, script, clone, apt):
  --cache-dir DIR        artifact cache                (NET_CACHE_DIR, default /mnt/hdd/auto-distrib)
  --no-cache             bypass the cache              (NET_NO_CACHE)
  --force-update         download again, refresh cache (NET_FORCE_UPDATE)
  --cache-ro             read-only: use hits, fetch     (NET_CACHE_RO)
                         misses without storing them
  --force-since UNIX     with --force-update, keep      (NET_FORCE_SINCE)
                         entries refreshed after UNIX
  A missing cache dir, or a read-only one without --cache-ro, is an error.

download:
  --mode MODE            permissions on DEST           (NET_MODE, default 0644)

script:
  --shell PATH           interpreter                   (NET_SHELL, default sh)

clone:
  --depth N              clone depth (default 1)
  --force                clone even if DEST exists
  With --force-update an existing DEST is cloned again as well.

  --version, -v          print the version
  --origin               print the repository this binary was built from
  --buildinfo            print the full build info
  -h, --help             show this help

Exit codes: 0 ok, 1 local error, 2 usage error, 6 DNS, 7 connect, 18 truncated,
22 HTTP >= 400, 28 timeout or stalled transfer, 35 TLS, 47 too many redirects.
"script" and "clone" propagate the exit code of the child process instead.
`

func printUsage(w io.Writer) {
	fmt.Fprint(w, usageText)
}

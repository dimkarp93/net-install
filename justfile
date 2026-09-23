bin := "net-install"
version_file := "versions.txt"
export GOWORK := "off"
export GOFLAGS := "-mod=vendor"

_default:
    @just --list

build:
    #!/usr/bin/env sh
    set -eu
    v=$(tr -d '[:space:]' < {{version_file}})
    u=$(git remote get-url origin 2>/dev/null || true)
    case "$u" in
        "")    o=local ;;
        *://*) h=${u#*://}; h=${h#*@}; o="https://${h%.git}" ;;
        *:*)   h=${u#*@};   o="https://$(printf '%s' "${h%.git}" | tr ':' '/')" ;;
        *)     o=local ;;
    esac
    if [ -f upstream.txt ]; then up=$(tr -d '[:space:]' < upstream.txt); else up="$o"; fi
    c=$(git rev-parse --short HEAD 2>/dev/null || true)
    CGO_ENABLED=0 go build -trimpath \
        -ldflags="-s -w -X main.version=$v -X main.origin=$o -X main.upstream=$up -X main.commit=$c -X main.channel=local" \
        -o {{bin}} ./cmd/net-install
    echo "Built: ./{{bin}} (v$v)"

test mask="":
    go test {{ if mask != "" { "-run " + mask } else { "" } }} ./...

vet:
    go vet ./...

fmt:
    go fmt ./...

check: vet test

check-conventions:
    check_install.sh --build .

clean:
    rm -f {{bin}}
    rm -rf dist

install: build
    install -d "$HOME/.local/bin"
    install -m 0755 {{bin}} "$HOME/.local/bin/{{bin}}"

uninstall:
    rm -f "$HOME/.local/bin/{{bin}}"

bump-patch:
    #!/usr/bin/env sh
    set -eu
    v=$(tr -d '[:space:]' < versions.txt)
    IFS=. read -r MAJ MIN PAT <<EOF
    $v
    EOF
    printf '%s.%s.%s\n' "$MAJ" "$MIN" "$((PAT + 1))" > versions.txt
    cat versions.txt

bump-minor:
    #!/usr/bin/env sh
    set -eu
    v=$(tr -d '[:space:]' < versions.txt)
    IFS=. read -r MAJ MIN PAT <<EOF
    $v
    EOF
    printf '%s.%s.0\n' "$MAJ" "$((MIN + 1))" > versions.txt
    cat versions.txt

bump-major:
    #!/usr/bin/env sh
    set -eu
    v=$(tr -d '[:space:]' < versions.txt)
    IFS=. read -r MAJ MIN PAT <<EOF
    $v
    EOF
    printf '%s.0.0\n' "$((MAJ + 1))" > versions.txt
    cat versions.txt

release level="patch":
    #!/usr/bin/env sh
    set -eu
    case "{{level}}" in
        patch|minor|major) ;;
        *) echo "level must be patch, minor or major" >&2; exit 1 ;;
    esac
    git diff --quiet && git diff --cached --quiet || { echo "working tree is dirty" >&2; exit 1; }
    just bump-{{level}} >/dev/null
    v=$(tr -d '[:space:]' < versions.txt)
    git add versions.txt
    git commit -q -m "release v$v"
    git push origin HEAD
    echo "Pushed v$v - the release workflow will create the tag"

vendor:
    GOWORK=off go mod tidy
    GOWORK=off go mod vendor

vendor-check:
    GOWORK=off go mod vendor
    test -z "$(git status --porcelain -- go.mod go.sum vendor/ | tee /dev/stderr)"

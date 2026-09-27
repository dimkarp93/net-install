package shellcomplete

import (
	"fmt"
	"strings"
)

const bashTemplate = `{{fn}}() {
    local cur line out
    local IFS=$'\n'
    cur=${COMP_WORDS[COMP_CWORD]}
    [ "$cur" = "=" ] && cur=
    COMPREPLY=()
    out=$({{bin}} ` + CompleteCommand + ` "${COMP_WORDS[@]:1:COMP_CWORD}" 2>/dev/null) || return
    for line in $out; do
        case $line in
            ` + DirNone + `)
                return
                ;;
            ` + DirDirs + `)
                COMPREPLY=($(compgen -d -- "$cur"))
                compopt -o filenames 2>/dev/null
                return
                ;;
            ` + DirDefault + `)
                compopt -o default 2>/dev/null
                return
                ;;
            *)
                COMPREPLY+=("$line")
                ;;
        esac
    done
}
complete -F {{fn}} {{bin}}
`

const zshTemplate = `#compdef {{bin}}

_{{bin}}() {
    local -a args lines
    args=("${(@)words[2,CURRENT]}")
    lines=("${(@f)$({{bin}} ` + CompleteCommand + ` "${args[@]}" 2>/dev/null)}")
    case ${lines[1]} in
        ` + DirNone + `|'')
            return 1
            ;;
        ` + DirDirs + `)
            _files -/
            return
            ;;
        ` + DirDefault + `)
            _files
            return
            ;;
    esac
    compadd -- "${lines[@]}"
}

if [ "$funcstack[1]" = "_{{bin}}" ]; then
    _{{bin}} "$@"
else
    compdef _{{bin}} {{bin}}
fi
`

func bashFunc(bin string) string {
	var b strings.Builder
	b.WriteByte('_')
	for _, r := range bin {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func (s Spec) Script(shell string) (string, error) {
	var tpl string
	switch shell {
	case "bash":
		tpl = bashTemplate
	case "zsh":
		tpl = zshTemplate
	default:
		return "", fmt.Errorf("unknown shell %q, available: %s", shell, strings.Join(Shells, ", "))
	}
	return strings.NewReplacer("{{fn}}", bashFunc(s.Bin), "{{bin}}", s.Bin).Replace(tpl), nil
}

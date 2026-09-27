package cli

import (
	"sort"

	"github.com/dimkarp93/install-libs/shellcomplete"
	"github.com/dimkarp93/net-install/internal/config"
)

var completionCommandNames = []string{"fetch", "download", "script", "clone", "apt", "log"}

var completionSpec = shellcomplete.Spec{
	Bin: "net-install",
	Flags: []shellcomplete.Flag{
		{Name: "--help", Bool: true},
		{Name: "--envs", Bool: true},
		{Name: "--version", Bool: true},
		{Name: "--origin", Bool: true},
		{Name: "--buildinfo", Bool: true},
	},
	Commands: completionCommands(),
}

func completionCommands() []shellcomplete.Command {
	cmds := make([]shellcomplete.Command, 0, len(completionCommandNames))
	for _, name := range completionCommandNames {
		s, _ := specFor(name)
		cmds = append(cmds, shellcomplete.Command{Name: name, Flags: specFlags(s), Args: shellcomplete.Anything})
	}
	return cmds
}

func specFlags(s spec) []shellcomplete.Flag {
	var out []shellcomplete.Flag
	for name := range s.flag {
		out = append(out, shellcomplete.Flag{Name: "--" + name, Bool: true})
	}
	for name := range s.value {
		f := shellcomplete.Flag{Name: "--" + name}
		if name == config.CacheDir {
			f.Dirs = true
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

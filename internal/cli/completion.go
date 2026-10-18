package cli

import (
	"slices"
	"sort"

	"github.com/dimkarp93/install-libs/shellcomplete"
	"github.com/dimkarp93/net-install/internal/config"
	"github.com/dimkarp93/utils/pkg/collections"
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
	return collections.Map(completionCommandNames, completionCommand)
}

func completionCommand(name string) shellcomplete.Command {
	s, _ := specFor(name)
	return shellcomplete.Command{
		Name:  name,
		Flags: specFlags(s),
		Args:  shellcomplete.Anything,
	}
}

func specFlags(s spec) []shellcomplete.Flag {
	out := slices.Concat(
		collections.Map(collections.SortedKeys(s.flag), boolFlag),
		collections.Map(collections.SortedKeys(s.value), valueFlag),
	)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func boolFlag(name string) shellcomplete.Flag {
	return shellcomplete.Flag{Name: "--" + name, Bool: true}
}

func valueFlag(name string) shellcomplete.Flag {
	return shellcomplete.Flag{Name: "--" + name, Dirs: name == config.CacheDir}
}

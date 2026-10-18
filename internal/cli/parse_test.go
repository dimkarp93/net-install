package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dimkarp93/install-libs/buildinfo"
)

func parsedFor(t *testing.T, command string, args ...string) *parsed {
	t.Helper()

	s, ok := specFor(command)
	require.True(t, ok)
	p, err := parse(args, s)
	require.NoError(t, err)

	return p
}

type parseCase struct {
	name    string
	command string
	args    []string
	want    parsed
}

func parseCases() []parseCase {
	return []parseCase{
		{
			name: "script arguments reach the script", command: "script",
			args: []string{"--shell", "bash", "https://x/i.sh", "--to", "/opt", "--unattended"},
			want: parsed{flags: []flag{{"shell", "bash"}}, args: []string{"https://x/i.sh", "--to", "/opt", "--unattended"}},
		},
		{
			name: "flags after the url are not consumed", command: "script",
			args: []string{"https://x/i.sh", "--shell", "bash"},
			want: parsed{args: []string{"https://x/i.sh", "--shell", "bash"}},
		},
		{
			name: "equals form", command: "download",
			args: []string{"--mode=0755", "u", "d"},
			want: parsed{flags: []flag{{"mode", "0755"}}, args: []string{"u", "d"}},
		},
		{
			name: "boolean flag takes no value", command: "clone",
			args: []string{"--force", "u", "d"},
			want: parsed{flags: []flag{{"force", "1"}}, args: []string{"u", "d"}},
		},
		{
			name: "double dash ends flags", command: "fetch",
			args: []string{"--retries", "2", "--", "-weird-url", "dest"},
			want: parsed{flags: []flag{{"retries", "2"}}, args: []string{"-weird-url", "dest"}},
		},
		{
			name: "stdout destination is positional", command: "fetch",
			args: []string{"https://x", "-"},
			want: parsed{args: []string{"https://x", "-"}},
		},
	}
}

func TestParse(t *testing.T) {
	for _, tc := range parseCases() {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, &tc.want, parsedFor(t, tc.command, tc.args...))
		})
	}
}

func TestParseRejects(t *testing.T) {
	cases := map[string][]string{
		"unknown flag":             {"--nope", "u", "d"},
		"single dash flag":         {"-r", "u", "d"},
		"value flag without value": {"--retries"},
		"mode outside download":    {"--mode", "0755", "u", "d"},
	}
	s, _ := specFor("fetch")
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parse(args, s)
			assert.Error(t, err)
		})
	}
}

func TestSpecForUnknownCommand(t *testing.T) {
	_, ok := specFor("nope")

	assert.False(t, ok)
}

func TestBuildinfoDoesNotSwallowSubcommands(t *testing.T) {
	info := buildinfo.Info{Version: "0.1.0"}
	for _, command := range []string{"fetch", "download", "script", "clone", "log", "--envs"} {
		t.Run(command, func(t *testing.T) {
			assert.False(t, info.FHandle(os.Stdout, []string{command, "https://x", "--version"}))
		})
	}
}

func TestBuildinfoHandlesVersion(t *testing.T) {
	assert.True(t, buildinfo.Info{Version: "0.1.0"}.FHandle(os.Stdout, []string{"--version"}))
}

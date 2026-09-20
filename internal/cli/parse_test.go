package cli

import (
	"os"
	"testing"

	"github.com/dimkarp93/install-libs/buildinfo"
)

func TestScriptArgumentsReachTheScript(t *testing.T) {
	s, _ := specFor("script")
	p, err := parse([]string{"--shell", "bash", "https://x/i.sh", "--to", "/opt", "--unattended"}, s)
	if err != nil {
		t.Fatal(err)
	}

	if len(p.flags) != 1 || p.flags[0].name != "shell" || p.flags[0].value != "bash" {
		t.Fatalf("flags=%v", p.flags)
	}
	want := []string{"https://x/i.sh", "--to", "/opt", "--unattended"}
	if len(p.args) != len(want) {
		t.Fatalf("args=%v, want %v", p.args, want)
	}
	for i := range want {
		if p.args[i] != want[i] {
			t.Fatalf("args=%v, want %v", p.args, want)
		}
	}
}

func TestFlagsAfterTheUrlAreNotConsumed(t *testing.T) {
	s, _ := specFor("script")
	p, err := parse([]string{"https://x/i.sh", "--shell", "bash"}, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.flags) != 0 {
		t.Fatalf("flags=%v, expected none", p.flags)
	}
	if len(p.args) != 3 {
		t.Fatalf("args=%v", p.args)
	}
}

func TestEqualsForm(t *testing.T) {
	s, _ := specFor("download")
	p, err := parse([]string{"--mode=0755", "u", "d"}, s)
	if err != nil {
		t.Fatal(err)
	}
	if p.flags[0].name != "mode" || p.flags[0].value != "0755" {
		t.Fatalf("flags=%v", p.flags)
	}
}

func TestBooleanFlagTakesNoValue(t *testing.T) {
	s, _ := specFor("clone")
	p, err := parse([]string{"--force", "u", "d"}, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.flags) != 1 || p.flags[0].value != "1" {
		t.Fatalf("flags=%v", p.flags)
	}
	if len(p.args) != 2 {
		t.Fatalf("args=%v", p.args)
	}
}

func TestDoubleDashEndsFlags(t *testing.T) {
	s, _ := specFor("fetch")
	p, err := parse([]string{"--retries", "2", "--", "-weird-url", "dest"}, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.args) != 2 || p.args[0] != "-weird-url" {
		t.Fatalf("args=%v", p.args)
	}
}

func TestStdoutDestinationIsPositional(t *testing.T) {
	s, _ := specFor("fetch")
	p, err := parse([]string{"https://x", "-"}, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.args) != 2 || p.args[1] != "-" {
		t.Fatalf("args=%v", p.args)
	}
}

func TestUnknownFlagIsRejected(t *testing.T) {
	s, _ := specFor("fetch")
	if _, err := parse([]string{"--nope", "u", "d"}, s); err == nil {
		t.Fatal("expected an error")
	}
	if _, err := parse([]string{"--retries"}, s); err == nil {
		t.Fatal("a value flag without a value was accepted")
	}
}

func TestModeIsNotAcceptedOutsideDownload(t *testing.T) {
	s, _ := specFor("fetch")
	if _, err := parse([]string{"--mode", "0755", "u", "d"}, s); err == nil {
		t.Fatal("fetch accepted --mode")
	}
}

func TestBuildinfoDoesNotSwallowSubcommands(t *testing.T) {
	info := buildinfo.Info{Version: "0.1.0"}
	for _, command := range []string{"fetch", "download", "script", "clone", "log", "env"} {
		if info.FHandle(os.Stdout, []string{command, "https://x", "--version"}) {
			t.Fatalf("buildinfo swallowed %q", command)
		}
	}
	if !info.FHandle(os.Stdout, []string{"--version"}) {
		t.Fatal("buildinfo did not handle --version")
	}
}

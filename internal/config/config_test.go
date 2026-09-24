package config

import "testing"

func lookup(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestDefaultsMatchTheShellLibrary(t *testing.T) {
	c := New()
	if c.Retries != 5 || c.Delay != 5 || c.ConnectTimeout != 20 {
		t.Fatalf("retries/delay/connect: %d/%d/%d", c.Retries, c.Delay, c.ConnectTimeout)
	}
	if c.SpeedLimit != 1024 || c.SpeedTime != 30 {
		t.Fatalf("speed: %d/%d", c.SpeedLimit, c.SpeedTime)
	}
	if c.Shell != "sh" || c.Mode != "0644" {
		t.Fatalf("shell/mode: %q/%q", c.Shell, c.Mode)
	}
}

func TestFlagBeatsEnvBeatsDefault(t *testing.T) {
	c := New()
	if err := c.LoadEnv(lookup(map[string]string{"NET_RETRIES": "9", "NET_SHELL": "bash"})); err != nil {
		t.Fatal(err)
	}
	if err := c.Set(Delay, "2", SourceFlag); err != nil {
		t.Fatal(err)
	}

	if c.Retries != 9 || c.Source(Retries) != SourceEnv {
		t.Fatalf("retries=%d source=%s", c.Retries, c.Source(Retries))
	}
	if c.Shell != "bash" || c.Source(Shell) != SourceEnv {
		t.Fatalf("shell=%q source=%s", c.Shell, c.Source(Shell))
	}
	if c.Delay != 2 || c.Source(Delay) != SourceFlag {
		t.Fatalf("delay=%d source=%s", c.Delay, c.Source(Delay))
	}
	if c.SpeedTime != 30 || c.Source(SpeedTime) != SourceDefault {
		t.Fatalf("speed-time=%d source=%s", c.SpeedTime, c.Source(SpeedTime))
	}
}

func TestBadValuesAreRejected(t *testing.T) {
	cases := []struct{ name, value string }{
		{Retries, "many"},
		{Retries, "0"},
		{Delay, "-1"},
		{Mode, "0999"},
		{RetryAll, "maybe"},
	}
	for _, tc := range cases {
		if err := New().Set(tc.name, tc.value, SourceFlag); err == nil {
			t.Fatalf("%s=%q was accepted", tc.name, tc.value)
		}
	}
}

func TestEnvErrorNamesTheVariable(t *testing.T) {
	err := New().LoadEnv(lookup(map[string]string{"NET_SPEED_LIMIT": "fast"}))
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); got[:len("NET_SPEED_LIMIT")] != "NET_SPEED_LIMIT" {
		t.Fatalf("error does not name the variable: %s", got)
	}
}

func TestParseMode(t *testing.T) {
	m, err := ParseMode("0755")
	if err != nil || m != 0o755 {
		t.Fatalf("got %o, %v", m, err)
	}
}

func TestCacheOptions(t *testing.T) {
	c := New()
	if c.CacheDir != "/mnt/hdd/auto-distrib" || c.NoCache || c.ForceUpdate {
		t.Fatalf("defaults: %q %v %v", c.CacheDir, c.NoCache, c.ForceUpdate)
	}
	err := c.LoadEnv(lookup(map[string]string{"NET_CACHE_DIR": "/x", "NET_FORCE_UPDATE": "1", "NET_NO_CACHE": "yes"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.CacheDir != "/x" || !c.ForceUpdate || !c.NoCache {
		t.Fatalf("env: %q %v %v", c.CacheDir, c.ForceUpdate, c.NoCache)
	}
	if New().Set(CacheDir, "", SourceFlag) == nil {
		t.Fatal("an empty cache dir was accepted")
	}
}

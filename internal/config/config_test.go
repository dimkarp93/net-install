package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lookup(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

type valueSource struct {
	Value  any
	Source Source
}

func TestDefaultsMatchTheShellLibrary(t *testing.T) {
	c := New()

	got := []any{c.Retries, c.Delay, c.ConnectTimeout, c.SpeedLimit, c.SpeedTime, c.Shell, c.Mode, c.Depth}

	assert.Equal(t, []any{5, 5, 20, 1024, 30, "sh", "0644", 1}, got)
}

func TestFlagBeatsEnvBeatsDefault(t *testing.T) {
	c := New()
	require.NoError(t, c.LoadEnv(lookup(map[string]string{"NET_RETRIES": "9", "NET_SHELL": "bash"})))
	require.NoError(t, c.Set(Delay, "2", SourceFlag))

	cases := []struct {
		name string
		got  valueSource
		want valueSource
	}{
		{name: Retries, got: valueSource{c.Retries, c.Source(Retries)}, want: valueSource{9, SourceEnv}},
		{name: Shell, got: valueSource{c.Shell, c.Source(Shell)}, want: valueSource{"bash", SourceEnv}},
		{name: Delay, got: valueSource{c.Delay, c.Source(Delay)}, want: valueSource{2, SourceFlag}},
		{name: SpeedTime, got: valueSource{c.SpeedTime, c.Source(SpeedTime)}, want: valueSource{30, SourceDefault}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.got)
		})
	}
}

func TestFlagOnlyOptionsTrackTheirSource(t *testing.T) {
	c := New()
	require.NoError(t, c.Set(Depth, "3", SourceFlag))

	t.Run("value", func(t *testing.T) {
		assert.Equal(t, 3, c.Depth)
	})
	t.Run("set source", func(t *testing.T) {
		assert.Equal(t, SourceFlag, c.Source(Depth))
	})
	t.Run("unset source", func(t *testing.T) {
		assert.Equal(t, SourceDefault, c.Source(Force))
	})
}

func TestBadValuesAreRejected(t *testing.T) {
	cases := []struct{ name, value string }{
		{Retries, "many"},
		{Retries, "0"},
		{Delay, "-1"},
		{Mode, "0999"},
		{RetryAll, "maybe"},
		{Depth, "x"},
		{Force, "maybe"},
		{CacheDir, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name+"="+tc.value, func(t *testing.T) {
			assert.Error(t, New().Set(tc.name, tc.value, SourceFlag))
		})
	}
}

func TestEnvErrorNamesTheVariable(t *testing.T) {
	err := New().LoadEnv(lookup(map[string]string{"NET_SPEED_LIMIT": "fast"}))

	require.Error(t, err)
	assert.Regexp(t, `^NET_SPEED_LIMIT`, err.Error())
}

func TestParseMode(t *testing.T) {
	m, err := ParseMode("0755")

	require.NoError(t, err)
	assert.Equal(t, uint32(0o755), m)
}

func TestCacheDefaults(t *testing.T) {
	c := New()

	assert.Equal(t, []any{"/mnt/hdd/auto-distrib", false, false}, []any{c.CacheDir, c.NoCache, c.ForceUpdate})
}

func TestCacheOptionsFromEnv(t *testing.T) {
	c := New()

	err := c.LoadEnv(lookup(map[string]string{"NET_CACHE_DIR": "/x", "NET_FORCE_UPDATE": "1", "NET_NO_CACHE": "yes"}))

	require.NoError(t, err)
	assert.Equal(t, []any{"/x", true, true}, []any{c.CacheDir, c.ForceUpdate, c.NoCache})
}

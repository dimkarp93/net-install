package instfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRunner struct {
	calls [][]string
	rc    int
}

func (f *fakeRunner) Run(name string, args ...string) int {
	f.calls = append(f.calls, append([]string{name}, args...))
	return f.rc
}

func committed(t *testing.T, dest string, mode os.FileMode, run *fakeRunner) error {
	t.Helper()

	target, err := Open(dest, mode)
	require.NoError(t, err)
	t.Cleanup(target.Close)

	_, err = target.File().WriteString("payload")
	require.NoError(t, err)

	return target.Commit(run)
}

func committedOK(t *testing.T, dest string, mode os.FileMode) *fakeRunner {
	t.Helper()

	run := &fakeRunner{}
	require.NoError(t, committed(t, dest, mode, run))

	return run
}

func lockedDir(t *testing.T) string {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}

	locked := filepath.Join(t.TempDir(), "locked")
	require.NoError(t, os.Mkdir(locked, 0o555))

	return locked
}

func TestWritableTargetGoesThroughGo(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "sub", "file")
	run := committedOK(t, dest, 0o755)

	t.Run("no sudo", func(t *testing.T) {
		assert.Empty(t, run.calls)
	})
	t.Run("content", func(t *testing.T) {
		body, err := os.ReadFile(dest)
		require.NoError(t, err)
		assert.Equal(t, "payload", string(body))
	})
	t.Run("mode", func(t *testing.T) {
		st, err := os.Stat(dest)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), st.Mode().Perm())
	})
}

func TestTempFileIsRemovedWhenNotCommitted(t *testing.T) {
	dir := t.TempDir()
	target, err := Open(filepath.Join(dir, "file"), 0o644)
	require.NoError(t, err)

	target.Close()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestUnwritableTargetEscalates(t *testing.T) {
	dest := filepath.Join(lockedDir(t), "file")
	got := singleCall(t, committedOK(t, dest, 0o600))

	t.Run("sudo install", func(t *testing.T) {
		assert.Equal(t, []string{"sudo", "install", "-D", "-m", "0600"}, got[:5])
	})
	t.Run("destination", func(t *testing.T) {
		assert.Equal(t, dest, got[len(got)-1])
	})
}

func singleCall(t *testing.T, run *fakeRunner) []string {
	t.Helper()

	require.Len(t, run.calls, 1)

	return run.calls[0]
}

func TestSudoFailurePropagates(t *testing.T) {
	err := committed(t, filepath.Join(lockedDir(t), "file"), 0o644, &fakeRunner{rc: 1})

	assert.Error(t, err)
}

func TestDirectoryDestinationIsRejected(t *testing.T) {
	_, err := Open(t.TempDir(), 0o644)

	assert.Error(t, err)
}

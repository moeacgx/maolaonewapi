package common

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRuntimeVersionPriority(t *testing.T) {
	tests := []struct {
		name          string
		envVersion    string
		linkedVersion string
		fileContent   string
		want          string
	}{
		{name: "environment wins", envVersion: " v9.9.9 ", linkedVersion: "v2.3.4", fileContent: "v1.2.3\n", want: "v9.9.9"},
		{name: "linked release wins", linkedVersion: " v2.3.4 ", fileContent: "v1.2.3\n", want: "v2.3.4"},
		{name: "caller injected deployment version wins", linkedVersion: "deployment-test", fileContent: "v1.2.3\n", want: "deployment-test"},
		{name: "version file wins over git", fileContent: " v1.2.3\n", want: "v1.2.3"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			versionFile := writeVersionFile(t, test.fileContent)
			gitCalled := false

			got := resolveRuntimeVersion(test.envVersion, test.linkedVersion, versionFile, func(context.Context) (string, error) {
				gitCalled = true
				return "v1.0.0", nil
			})

			assert.Equal(t, test.want, got)
			assert.False(t, gitCalled, "git fallback must not run when an explicit version source resolves")
		})
	}
}

func TestResolveRuntimeVersionAcceptsSafeGitTags(t *testing.T) {
	tests := []string{
		"v1.0.0-rc.10",
		"release/2026.08",
		"deployment-test",
	}

	for _, gitVersion := range tests {
		t.Run(gitVersion, func(t *testing.T) {
			got := resolveRuntimeVersion("", "v0.0.0", missingVersionFile(t), func(context.Context) (string, error) {
				return " \t" + gitVersion + "\n", nil
			})

			assert.Equal(t, gitVersion, got)
		})
	}
}

func TestResolveRuntimeVersionRejectsUnsafeGitOutput(t *testing.T) {
	tests := []struct {
		name       string
		gitVersion string
	}{
		{name: "dirty suffix", gitVersion: "v1.2.3-dirty"},
		{name: "dirty marker within output", gitVersion: "release-dirty-build"},
		{name: "abbreviated raw sha", gitVersion: "abc1234"},
		{name: "git prefixed raw sha", gitVersion: "gabcdef0"},
		{name: "tag plus raw sha", gitVersion: "v1.2.3-4-gabcdef0"},
		{name: "full raw sha", gitVersion: "0123456789abcdef0123456789abcdef01234567"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolveRuntimeVersion("", "v0.0.0", missingVersionFile(t), func(context.Context) (string, error) {
				return test.gitVersion, nil
			})

			assert.Equal(t, "v0.0.0", got)
		})
	}
}

func TestResolveRuntimeVersionFallsBackOnGitTimeoutOrError(t *testing.T) {
	tests := []struct {
		name       string
		gitResolve func(*testing.T, context.Context) (string, error)
	}{
		{
			name: "timeout",
			gitResolve: func(t *testing.T, ctx context.Context) (string, error) {
				deadline, ok := ctx.Deadline()
				require.True(t, ok, "git fallback must have a deadline")
				assert.WithinDuration(t, time.Now().Add(gitVersionTimeout), deadline, 100*time.Millisecond)
				return "", context.DeadlineExceeded
			},
		},
		{
			name: "command error",
			gitResolve: func(_ *testing.T, _ context.Context) (string, error) {
				return "", errors.New("git unavailable")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolveRuntimeVersion("", "v0.0.0", missingVersionFile(t), func(ctx context.Context) (string, error) {
				return test.gitResolve(t, ctx)
			})

			assert.Equal(t, "v0.0.0", got)
		})
	}
}

func writeVersionFile(t *testing.T, content string) string {
	t.Helper()
	versionFile := filepath.Join(t.TempDir(), "VERSION")
	require.NoError(t, os.WriteFile(versionFile, []byte(content), 0o600))
	return versionFile
}

func missingVersionFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "missing.VERSION")
}

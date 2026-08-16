package common

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRuntimeVersion(t *testing.T) {
	tempDir := t.TempDir()
	tests := []struct {
		name          string
		envVersion    string
		linkedVersion string
		fileContent   string
		gitVersion    string
		want          string
	}{
		{name: "environment wins", envVersion: " v9.9.9 ", linkedVersion: "v2.3.4", fileContent: "v1.2.3\n", gitVersion: "v1.0.0", want: "v9.9.9"},
		{name: "linked release wins", linkedVersion: "v2.3.4", fileContent: "v1.2.3\n", gitVersion: "v1.0.0", want: "v2.3.4"},
		{name: "version file wins over git", fileContent: "v1.2.3\n", gitVersion: "v1.0.0", want: "v1.2.3"},
		{name: "git fallback", gitVersion: "v1.0.0-rc.10", want: "v1.0.0-rc.10"},
		{name: "linked development fallback", linkedVersion: "v0.0.0", want: "v0.0.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			versionFile := filepath.Join(tempDir, test.name+".VERSION")
			if err := os.WriteFile(versionFile, []byte(test.fileContent), 0o600); err != nil {
				t.Fatalf("write version file: %v", err)
			}
			got := resolveRuntimeVersion(test.envVersion, test.linkedVersion, versionFile, func() (string, error) {
				if test.gitVersion == "" {
					return "", os.ErrNotExist
				}
				return test.gitVersion, nil
			})
			if got != test.want {
				t.Fatalf("resolveRuntimeVersion() = %q, want %q", got, test.want)
			}
		})
	}
}

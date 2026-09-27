// SPDX-License-Identifier: Apache-2.0

package k8s

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// stubKubectl puts a fake kubectl first on PATH. script is the body of a POSIX
// shell script that sees kubectl's arguments as "$*".
func stubKubectl(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestGetClusterInfo(t *testing.T) {
	tests := []struct {
		name          string
		script        string
		wantConnected bool
		wantError     string
		wantNodes     int
	}{
		{
			name: "a reachable cluster is connected",
			script: `case "$*" in
  "config current-context") echo k3d-snowops ;;
  *readyz*) echo ok ;;
  *"--minify"*) echo https://127.0.0.1:6443 ;;
  "version -o json") echo '{"serverVersion":{"gitVersion":"v1.33.6+k3s1"}}' ;;
  "get nodes --no-headers") printf 'a Ready\nb Ready\n' ;;
esac`,
			wantConnected: true,
			wantNodes:     2,
		},
		{
			name: "a configured but unreachable cluster is not connected",
			script: `case "$*" in
  "config current-context") echo k3d-snowops ;;
  *readyz*) echo "Unable to connect to the server: net/http: TLS handshake timeout" >&2; exit 1 ;;
esac`,
			wantError: "net/http: TLS handshake timeout",
		},
		{
			name:   "no current context is not connected",
			script: `exit 1`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubKubectl(t, tt.script)
			info, err := GetClusterInfo(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if info.Connected != tt.wantConnected {
				t.Errorf("Connected = %v, want %v", info.Connected, tt.wantConnected)
			}
			if info.Error != tt.wantError {
				t.Errorf("Error = %q, want %q", info.Error, tt.wantError)
			}
			if info.NodeCount != tt.wantNodes {
				t.Errorf("NodeCount = %d, want %d", info.NodeCount, tt.wantNodes)
			}
		})
	}
}

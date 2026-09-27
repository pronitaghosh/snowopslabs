// SPDX-License-Identifier: Apache-2.0

package toolchain

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMinimumsMatchVersionsEnv keeps doctor and setup-tools agreeing on what
// "new enough" means. They once disagreed (kubectl 1.28 vs 1.36.1), so doctor
// passed a machine that setup-tools then asked for sudo to "fix".
func TestMinimumsMatchVersionsEnv(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "..", "config", "versions.env"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	env := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if ok && !strings.HasPrefix(k, "#") {
			env[k] = v
		}
	}

	keys := map[string]string{
		"kubectl": "KUBECTL_MIN_VERSION",
		"helm":    "HELM_MIN_VERSION",
		"k3d":     "K3D_MIN_VERSION",
		"kind":    "KIND_MIN_VERSION",
	}
	for _, req := range Requirements() {
		key, ok := keys[req.Binary]
		if !ok {
			continue
		}
		delete(keys, req.Binary)
		if env[key] != req.MinVersion {
			t.Errorf("%s: preflight minimum %q, versions.env %s=%q", req.Binary, req.MinVersion, key, env[key])
		}
	}
	for bin := range keys {
		t.Errorf("%s has a minimum in versions.env but no preflight requirement", bin)
	}
}

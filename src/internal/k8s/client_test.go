// SPDX-License-Identifier: Apache-2.0

package k8s

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// parseVersionJSON exercises the same JSON unmarshaling logic as GetClusterInfo.
func parseVersionJSON(raw string) string {
	var vOut struct {
		ServerVersion struct {
			GitVersion string `json:"gitVersion"`
		} `json:"serverVersion"`
	}
	if json.Unmarshal([]byte(raw), &vOut) == nil && vOut.ServerVersion.GitVersion != "" {
		return vOut.ServerVersion.GitVersion
	}
	return "unknown"
}

func TestParseVersionJSON_HappyPath(t *testing.T) {
	raw := `{"clientVersion":{"gitVersion":"v1.36.1"},"serverVersion":{"gitVersion":"v1.31.0+k3s1"}}`
	got := parseVersionJSON(raw)
	if got != "v1.31.0+k3s1" {
		t.Errorf("got %q, want %q", got, "v1.31.0+k3s1")
	}
}

func TestParseVersionJSON_Offline(t *testing.T) {
	// When cluster is unreachable, kubectl version -o json omits serverVersion.
	raw := `{"clientVersion":{"gitVersion":"v1.36.1"}}`
	got := parseVersionJSON(raw)
	if got != "unknown" {
		t.Errorf("got %q, want \"unknown\"", got)
	}
}

func TestParseVersionJSON_Empty(t *testing.T) {
	got := parseVersionJSON("")
	if got != "unknown" {
		t.Errorf("got %q, want \"unknown\"", got)
	}
}

// parsePodListJSON exercises the same JSON unmarshaling logic used by GetNamespacePods.
func parsePodListJSON(raw, namespace string) ([]PodInfo, error) {
	var list podListJSON
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	var pods []PodInfo
	for _, item := range list.Items {
		readyCount := 0
		totalCount := len(item.Status.ContainerStatuses)
		var restarts int32
		for _, cs := range item.Status.ContainerStatuses {
			if cs.Ready {
				readyCount++
			}
			restarts += cs.RestartCount
		}
		pods = append(pods, PodInfo{
			Name:      item.Metadata.Name,
			Namespace: namespace,
			Status:    item.Status.Phase,
			Ready:     fmt.Sprintf("%d/%d", readyCount, totalCount),
			Restarts:  strconv.Itoa(int(restarts)),
		})
	}
	return pods, nil
}

const runningPodJSON = `{
  "items": [{
    "metadata": {"name": "go-api-abc"},
    "status": {
      "phase": "Running",
      "containerStatuses": [
        {"ready": true, "restartCount": 0}
      ]
    }
  }]
}`

const pendingPodJSON = `{
  "items": [{
    "metadata": {"name": "go-api-xyz"},
    "status": {
      "phase": "Pending",
      "containerStatuses": []
    }
  }]
}`

const restartsPodJSON = `{
  "items": [{
    "metadata": {"name": "crasher-pod"},
    "status": {
      "phase": "Running",
      "containerStatuses": [
        {"ready": true,  "restartCount": 3},
        {"ready": false, "restartCount": 2}
      ]
    }
  }]
}`

func TestParsePodList_RunningPod(t *testing.T) {
	pods, err := parsePodListJSON(runningPodJSON, "go-api")
	if err != nil {
		t.Fatalf("parsePodListJSON: %v", err)
	}
	if len(pods) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(pods))
	}
	p := pods[0]
	if p.Name != "go-api-abc" {
		t.Errorf("Name: got %q, want %q", p.Name, "go-api-abc")
	}
	if p.Status != "Running" {
		t.Errorf("Status: got %q, want %q", p.Status, "Running")
	}
	if p.Ready != "1/1" {
		t.Errorf("Ready: got %q, want %q", p.Ready, "1/1")
	}
	if p.Restarts != "0" {
		t.Errorf("Restarts: got %q, want %q", p.Restarts, "0")
	}
}

func TestParsePodList_PendingPod(t *testing.T) {
	pods, err := parsePodListJSON(pendingPodJSON, "go-api")
	if err != nil {
		t.Fatalf("parsePodListJSON: %v", err)
	}
	if len(pods) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(pods))
	}
	p := pods[0]
	if p.Status != "Pending" {
		t.Errorf("Status: got %q, want %q", p.Status, "Pending")
	}
	if p.Ready != "0/0" {
		t.Errorf("Ready: got %q, want %q", p.Ready, "0/0")
	}
}

func TestParsePodList_MultiContainerRestarts(t *testing.T) {
	pods, err := parsePodListJSON(restartsPodJSON, "default")
	if err != nil {
		t.Fatalf("parsePodListJSON: %v", err)
	}
	if len(pods) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(pods))
	}
	p := pods[0]
	if p.Ready != "1/2" {
		t.Errorf("Ready: got %q, want %q", p.Ready, "1/2")
	}
	if p.Restarts != "5" {
		t.Errorf("Restarts: got %q, want %q", p.Restarts, "5")
	}
}

// kedaHPAJSON is a KEDA-created v2 HPA driven by an External (Prometheus) metric
// — the shape the autoscaling-under-load scenario produces under load.
const kedaHPAJSON = `{
  "items": [
    {
      "metadata": {"name": "keda-hpa-go-api"},
      "spec": {
        "scaleTargetRef": {"kind": "Deployment", "name": "go-api"},
        "minReplicas": 1,
        "maxReplicas": 6,
        "metrics": [
          {"type": "External", "external": {
            "metric": {"name": "s0-prometheus-go_api_requests_per_second"},
            "target": {"type": "AverageValue", "averageValue": "25"}
          }}
        ]
      },
      "status": {
        "currentReplicas": 3,
        "desiredReplicas": 3,
        "currentMetrics": [
          {"type": "External", "external": {
            "metric": {"name": "s0-prometheus-go_api_requests_per_second"},
            "current": {"averageValue": "27467m"}
          }}
        ]
      }
    }
  ]
}`

// cpuHPAJSON is a classic Resource (CPU utilization) HPA targeting a different
// deployment, used to prove selection-by-name and the "%" rendering.
const cpuHPAJSON = `{
  "items": [
    {
      "metadata": {"name": "web-hpa"},
      "spec": {
        "scaleTargetRef": {"kind": "Deployment", "name": "web"},
        "minReplicas": 2,
        "maxReplicas": 10,
        "metrics": [
          {"type": "Resource", "resource": {"name": "cpu", "target": {"type": "Utilization", "averageUtilization": 80}}}
        ]
      },
      "status": {
        "currentReplicas": 4,
        "desiredReplicas": 5,
        "currentMetrics": [
          {"type": "Resource", "resource": {"name": "cpu", "current": {"averageUtilization": 92}}}
        ]
      }
    }
  ]
}`

func TestParseHPAList_KEDAExternalMetric(t *testing.T) {
	hpa, err := parseHPAList(kedaHPAJSON, "go-api")
	if err != nil {
		t.Fatalf("parseHPAList: %v", err)
	}
	if !hpa.Present {
		t.Fatal("expected HPA to be present for go-api")
	}
	if hpa.MinReplicas != 1 || hpa.MaxReplicas != 6 {
		t.Errorf("bounds: got %d/%d, want 1/6", hpa.MinReplicas, hpa.MaxReplicas)
	}
	if hpa.CurrentReplicas != 3 || hpa.DesiredReplicas != 3 {
		t.Errorf("replicas: got current=%d desired=%d, want 3/3", hpa.CurrentReplicas, hpa.DesiredReplicas)
	}
	if hpa.MetricName != "go_api_requests_per_second" {
		t.Errorf("name: got %q, want %q (KEDA prefix must be stripped)", hpa.MetricName, "go_api_requests_per_second")
	}
	if hpa.MetricTarget != "25" {
		t.Errorf("target: got %q, want %q", hpa.MetricTarget, "25")
	}
	// 27467m (milli) must be humanized to a plain decimal.
	if hpa.MetricCurrent != "27.467" {
		t.Errorf("current: got %q, want %q", hpa.MetricCurrent, "27.467")
	}
}

func TestCleanMetricName(t *testing.T) {
	cases := map[string]string{
		"s0-prometheus-go_api_requests_per_second": "go_api_requests_per_second",
		"s0-go_api_requests_per_second":            "go_api_requests_per_second",
		"s1-kafka-lag":                             "lag",
		"go_api_requests_per_second":               "go_api_requests_per_second",
		"cpu":                                      "cpu",
	}
	for in, want := range cases {
		if got := cleanMetricName(in); got != want {
			t.Errorf("cleanMetricName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanizeQuantity(t *testing.T) {
	cases := map[string]string{
		"300m":   "0.3",
		"27467m": "27.467",
		"1000m":  "1",
		"25":     "25",
		"80%":    "80%", // not a quantity — left as-is
		"":       "",
	}
	for in, want := range cases {
		if got := humanizeQuantity(in); got != want {
			t.Errorf("humanizeQuantity(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseHPAList_CPUResourceMetricRendersPercent(t *testing.T) {
	hpa, err := parseHPAList(cpuHPAJSON, "web")
	if err != nil {
		t.Fatalf("parseHPAList: %v", err)
	}
	if hpa.MetricName != "cpu" {
		t.Errorf("name: got %q, want %q", hpa.MetricName, "cpu")
	}
	if hpa.MetricTarget != "80%" {
		t.Errorf("target: got %q, want %q", hpa.MetricTarget, "80%")
	}
	if hpa.MetricCurrent != "92%" {
		t.Errorf("current: got %q, want %q", hpa.MetricCurrent, "92%")
	}
	if hpa.DesiredReplicas != 5 {
		t.Errorf("desired: got %d, want 5", hpa.DesiredReplicas)
	}
}

func TestParseHPAList_NoMatchingDeployment(t *testing.T) {
	hpa, err := parseHPAList(kedaHPAJSON, "some-other-app")
	if err != nil {
		t.Fatalf("parseHPAList: %v", err)
	}
	if hpa.Present {
		t.Error("expected Present=false when no HPA targets the deployment")
	}
}

func TestParseHPAList_EmptyList(t *testing.T) {
	hpa, err := parseHPAList(`{"items": []}`, "go-api")
	if err != nil {
		t.Fatalf("parseHPAList: %v", err)
	}
	if hpa.Present {
		t.Error("expected Present=false for an empty HPA list")
	}
}

func TestUniqueLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "hosts in order", in: "a.k3d.local\nb.k3d.local", want: []string{"a.k3d.local", "b.k3d.local"}},
		{name: "duplicates and blanks dropped", in: "a\n\n a \nb\na\n", want: []string{"a", "b"}},
		{name: "no ingress at all", in: "", want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := uniqueLines(tt.in); fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("uniqueLines(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

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
			info, err := GetClusterInfo(t.Context())
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

func TestHelmReleaseDeployed(t *testing.T) {
	// Only a secret labelled status=deployed counts; a failed install leaves
	// status=failed, which must not make init skip the install.
	stubKubectl(t, `case "$*" in
  *"name=grafana,status=deployed"*) echo "sh.helm.release.v1.grafana.v1 helm.sh/release.v1 1 5m" ;;
esac`)
	if !HelmReleaseDeployed(t.Context(), "monitoring", "grafana") {
		t.Error("a deployed release should count")
	}
	if HelmReleaseDeployed(t.Context(), "monitoring", "prometheus") {
		t.Error("a release with no deployed revision must not count")
	}
}

func TestUnreachableReason(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name: "kubectl stderr, generic prefix dropped",
			err: &kubectlError{
				err:    errors.New("exit status 1"),
				stderr: "E0927 noise\nUnable to connect to the server: net/http: TLS handshake timeout",
			},
			expected: "net/http: TLS handshake timeout",
		},
		{name: "plain error", err: errors.New("kubectl not found in PATH"), expected: "kubectl not found in PATH"},
		{
			name:     "long message is cut on a rune boundary",
			err:      errors.New(strings.Repeat("é", maxReasonRunes+10)),
			expected: strings.Repeat("é", maxReasonRunes) + "…",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := unreachableReason(tt.err); got != tt.expected {
				t.Errorf("unreachableReason = %q, want %q", got, tt.expected)
			}
		})
	}
}

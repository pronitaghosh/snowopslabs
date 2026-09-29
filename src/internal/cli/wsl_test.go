// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestDetectWSL(t *testing.T) {
	noEnv := func(string) string { return "" }
	noFile := func(string) ([]byte, error) { return nil, errors.New("nope") }

	tests := []struct {
		name    string
		goos    string
		getenv  func(string) string
		read    func(string) ([]byte, error)
		wantWSL bool
	}{
		{
			name:    "non-linux is never WSL",
			goos:    "darwin",
			getenv:  func(string) string { return "Ubuntu" },
			read:    func(string) ([]byte, error) { return []byte("microsoft"), nil },
			wantWSL: false,
		},
		{
			name:    "WSL_DISTRO_NAME set",
			goos:    "linux",
			getenv:  func(k string) string { return map[string]string{"WSL_DISTRO_NAME": "Ubuntu"}[k] },
			read:    noFile,
			wantWSL: true,
		},
		{
			name:    "osrelease mentions microsoft",
			goos:    "linux",
			getenv:  noEnv,
			read:    func(p string) ([]byte, error) { return []byte("5.15.90.1-microsoft-standard-WSL2"), nil },
			wantWSL: true,
		},
		{
			name:    "proc version mentions WSL",
			goos:    "linux",
			getenv:  noEnv,
			read:    func(p string) ([]byte, error) { return []byte("Linux version ... WSL ..."), nil },
			wantWSL: true,
		},
		{
			name:    "plain linux is not WSL",
			goos:    "linux",
			getenv:  noEnv,
			read:    func(p string) ([]byte, error) { return []byte("6.8.0-generic"), nil },
			wantWSL: false,
		},
		{
			name:    "unreadable proc files, no env",
			goos:    "linux",
			getenv:  noEnv,
			read:    noFile,
			wantWSL: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectWSL(tt.goos, tt.getenv, tt.read); got != tt.wantWSL {
				t.Fatalf("detectWSL()=%v want %v", got, tt.wantWSL)
			}
		})
	}
}

func TestBrowserCommands(t *testing.T) {
	const url = "http://localhost:3939"

	// WSL tries Windows openers before the Linux fallback.
	wsl := browserCommands("linux", true, url)
	if len(wsl) == 0 || wsl[0][0] != "wslview" {
		t.Fatalf("WSL should try wslview first, got %v", wsl)
	}
	if last := wsl[len(wsl)-1][0]; last != "xdg-open" {
		t.Fatalf("WSL should keep xdg-open as last resort, got %v", wsl)
	}

	cases := map[string]string{"linux": "xdg-open", "darwin": "open", "windows": "rundll32"}
	for goos, want := range cases {
		got := browserCommands(goos, false, url)
		if len(got) != 1 || got[0][0] != want {
			t.Fatalf("browserCommands(%q)=%v want single %q", goos, got, want)
		}
	}

	if got := browserCommands("plan9", false, url); got != nil {
		t.Fatalf("unknown GOOS should return nil, got %v", got)
	}
}

func TestWSLDoctorNotes(t *testing.T) {
	tests := []struct {
		name            string
		suffix          string
		resolvesLocally bool
		want            string
		notWant         string
	}{
		{name: "localhost needs no hosts file", suffix: "snowops.localhost", resolvesLocally: true, want: "No hosts-file edits", notWant: `drivers\etc\hosts`},
		{name: "another suffix needs the windows hosts file", suffix: "k3d.local", want: `drivers\etc\hosts`, notWant: "No hosts-file edits"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notes := strings.Join(wslDoctorNotes("/home/me/snowopslabs", tt.suffix, tt.resolvesLocally), "\n")
			if !strings.Contains(notes, tt.want) || strings.Contains(notes, tt.notWant) {
				t.Errorf("notes should mention %q and not %q:\n%s", tt.want, tt.notWant, notes)
			}
			if !strings.Contains(notes, "grafana."+tt.suffix) {
				t.Errorf("notes should show the lab's own hostname:\n%s", notes)
			}
		})
	}
}

// Doctor prints each note as one bullet, so a wrapped note must continue
// inside the same item rather than become a bullet of its own.
func TestWSLDoctorNotesOneBulletPerNote(t *testing.T) {
	for _, local := range []bool{true, false} {
		for _, n := range wslDoctorNotes("/mnt/c/Users/me/snowopslabs", "snowops.localhost", local) {
			if strings.TrimLeft(n, " ") != n {
				t.Errorf("note starts with whitespace, so it would print as its own bullet: %q", n)
			}
		}
	}
}

func TestWSLDoctorNotesWarnOnWindowsDrive(t *testing.T) {
	has := func(notes []string) bool {
		for _, n := range notes {
			if strings.Contains(n, "Windows drive") {
				return true
			}
		}
		return false
	}
	if !has(wslDoctorNotes("/mnt/c/Users/me/snowopslabs", "snowops.localhost", true)) {
		t.Error("a lab under /mnt/c should be warned about")
	}
	if has(wslDoctorNotes("/home/me/snowopslabs", "snowops.localhost", true)) {
		t.Error("a lab on the Linux filesystem should not be warned about")
	}
}

package main

import (
	"errors"
	"strings"
	"testing"
)

// fakeEnv builds a sysEnv with the given variables and commands on PATH.
func fakeEnv(goos string, vars map[string]string, cmds ...string) sysEnv {
	return sysEnv{
		goos:   goos,
		getenv: func(k string) string { return vars[k] },
		lookPath: func(name string) (string, error) {
			for _, c := range cmds {
				if c == name {
					return "/usr/bin/" + name, nil
				}
			}
			return "", errors.New("not found")
		},
	}
}

func TestDetectBackend(t *testing.T) {
	wayland := map[string]string{"WAYLAND_DISPLAY": "wayland-0"}
	x11 := map[string]string{"DISPLAY": ":0"}
	both := map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"}

	tests := []struct {
		name string
		env  sysEnv
		want string
	}{
		{"macos", fakeEnv("darwin", nil, "pbpaste"), "pbpaste"},
		{"macos ignores display vars", fakeEnv("darwin", both, "pbpaste", "xclip"), "pbpaste"},
		{"wayland", fakeEnv("linux", wayland, "wl-paste", "xclip"), "wl-paste"},
		{"x11 prefers xclip", fakeEnv("linux", x11, "xsel", "xclip"), "xclip"},
		{"x11 xsel", fakeEnv("linux", x11, "xsel"), "xsel"},
		{"xwayland without wl-paste", fakeEnv("linux", both, "xsel"), "xsel"},
		{"wayland preferred over x11", fakeEnv("linux", both, "xclip", "wl-paste"), "wl-paste"},
		{"freebsd x11", fakeEnv("freebsd", x11, "xclip"), "xclip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := selectBackend("auto", tt.env)
			if err != nil {
				t.Fatal(err)
			}
			if b.Name() != tt.want {
				t.Errorf("backend = %s, want %s", b.Name(), tt.want)
			}
		})
	}
}

func TestDetectBackendErrors(t *testing.T) {
	tests := []struct {
		name string
		env  sysEnv
		want string
	}{
		{"no session", fakeEnv("linux", nil, "xclip", "wl-paste"), "neither WAYLAND_DISPLAY nor DISPLAY is set"},
		{"wayland without wl-paste", fakeEnv("linux", map[string]string{"WAYLAND_DISPLAY": "w"}, "xclip"), "wl-clipboard"},
		{"x11 without tools", fakeEnv("linux", map[string]string{"DISPLAY": ":0"}), "neither xclip nor xsel"},
		{"macos without pbpaste", fakeEnv("darwin", nil), "pbpaste not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := selectBackend("auto", tt.env)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestForcedBackend(t *testing.T) {
	env := fakeEnv("linux", nil, "xsel")
	b, err := selectBackend("xsel", env)
	if err != nil {
		t.Fatal(err)
	}
	if b.Name() != "xsel" {
		t.Errorf("backend = %s, want xsel", b.Name())
	}

	if _, err := selectBackend("xclip", env); err == nil || !strings.Contains(err.Error(), "not found in PATH") {
		t.Errorf("missing forced backend: err = %v", err)
	}
	if _, err := selectBackend("klipper", env); err == nil || !strings.Contains(err.Error(), "unknown backend") {
		t.Errorf("unknown backend: err = %v", err)
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("  Error: target STRING not available\nmore\n"); got != "Error: target STRING not available" {
		t.Errorf("firstLine = %q", got)
	}
	if got := firstLine("\n\n"); got != "" {
		t.Errorf("firstLine of blank = %q", got)
	}
}

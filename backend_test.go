package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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

// shBackend is a cmdBackend whose "paste tool" is a shell snippet, standing
// in for wl-paste or xclip with the given empty-clipboard hints.
func shBackend(script string, emptyHints ...string) *cmdBackend {
	return &cmdBackend{name: "fake-paste", argv: []string{"sh", "-c", script}, emptyHints: emptyHints}
}

func TestCmdBackendRead(t *testing.T) {
	b := shBackend(`printf 'two\nlines'`)
	got, err := b.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "two\nlines" {
		t.Errorf("Read = %q", got)
	}
}

func TestCmdBackendEmptyClipboard(t *testing.T) {
	wl := pasteCommands["wl-paste"].emptyHints
	xclip := pasteCommands["xclip"].emptyHints
	tests := []struct {
		name   string
		script string
		hints  []string
	}{
		{"wl-paste", `echo "Nothing is copied" >&2; exit 1`, wl},
		{"old wl-paste", `echo "No selection" >&2; exit 1`, wl},
		{"wl-paste image only", `echo "No suitable type of content copied" >&2; exit 1`, wl},
		{"xclip", `echo "Error: target STRING not available" >&2; exit 1`, xclip},
		{"xclip utf8", `echo "Error: target UTF8_STRING not available" >&2; exit 1`, xclip},
		{"pbpaste", `exit 0`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := shBackend(tt.script, tt.hints...).Read(context.Background())
			if err != nil {
				t.Fatalf("empty clipboard reported as error: %v", err)
			}
			if len(got) != 0 {
				t.Errorf("Read = %q, want nothing", got)
			}
		})
	}
}

func TestPasteCmdEmptyClipboard(t *testing.T) {
	// What the --paste-cmd examples in the readme print when nothing is
	// highlighted or buffered yet.
	for _, msg := range []string{
		"Error: target STRING not available", // xclip -selection primary -o
		"Nothing is copied",                  // wl-paste --primary
		"no buffers",                         // tmux show-buffer
	} {
		got, err := newShellBackend(`echo "` + msg + `" >&2; exit 1`).Read(context.Background())
		if err != nil {
			t.Errorf("%q reported as error: %v", msg, err)
		}
		if len(got) != 0 {
			t.Errorf("%q: Read = %q, want nothing", msg, got)
		}
	}

	_, err := newShellBackend(`echo "no server running on /tmp/tmux-1000/default" >&2; exit 1`).Read(context.Background())
	if err == nil {
		t.Error("a real failure of the paste command was taken for an empty clipboard")
	}
}

func TestCmdBackendErrors(t *testing.T) {
	hints := pasteCommands["wl-paste"].emptyHints

	_, err := shBackend(`echo "Failed to connect to a Wayland server" >&2; echo detail >&2; exit 1`, hints...).Read(context.Background())
	if err == nil || err.Error() != "fake-paste: Failed to connect to a Wayland server" {
		t.Errorf("err = %v, want the first stderr line", err)
	}

	_, err = shBackend(`exit 7`, hints...).Read(context.Background())
	if err == nil || !strings.Contains(err.Error(), "exit status 7") {
		t.Errorf("silent failure: err = %v, want the exit status", err)
	}
}

func TestCmdBackendKilledOnTimeout(t *testing.T) {
	b := &cmdBackend{name: "slow", argv: []string{"sleep", "30"}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := b.Read(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Read took %v after its deadline", elapsed)
	}
}

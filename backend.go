package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Backend reads the current clipboard contents.
type Backend interface {
	// Name identifies the backend in messages.
	Name() string
	// Read returns the clipboard as it is right now. An empty clipboard is
	// returned as no data, not as an error.
	Read(ctx context.Context) ([]byte, error)
}

// pasteCommand is how one built-in backend reads the clipboard.
type pasteCommand struct {
	argv []string
	// emptyHints are lower-case fragments of the message the tool prints
	// when it fails only because there is nothing (textual) to paste. Such
	// failures are an empty clipboard, not a broken backend.
	emptyHints []string
}

// pasteCommands maps each built-in backend to the command that prints the
// clipboard to stdout.
var pasteCommands = map[string]pasteCommand{
	"pbpaste": {argv: []string{"pbpaste"}},
	"wl-paste": {
		// --type text keeps images and other binary targets out of the stream.
		argv: []string{"wl-paste", "--no-newline", "--type", "text"},
		// "Nothing is copied", "No selection" (older releases) and "No
		// suitable type of content copied" (only an image, say).
		emptyHints: []string{"nothing is copied", "no selection", "no suitable type"},
	},
	"xclip": {
		argv: []string{"xclip", "-selection", "clipboard", "-o"},
		// "Error: target STRING not available" and the UTF8_STRING variant.
		emptyHints: []string{"target string not available", "target utf8_string not available"},
	},
	"xsel": {argv: []string{"xsel", "--clipboard", "--output"}},
}

// backendNames lists the accepted --backend values, for messages.
const backendNames = "auto, pbpaste, wl-paste, xclip or xsel"

// cmdBackend reads the clipboard by running an external paste command.
type cmdBackend struct {
	name       string
	argv       []string
	emptyHints []string
}

func newCmdBackend(name string) *cmdBackend {
	pc := pasteCommands[name]
	return &cmdBackend{name: name, argv: pc.argv, emptyHints: pc.emptyHints}
}

func (b *cmdBackend) Name() string { return b.name }

func (b *cmdBackend) Read(ctx context.Context) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, b.argv[0], b.argv[1:]...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		msg := firstLine(stderr.String())
		if b.meansEmpty(msg) {
			return nil, nil
		}
		if msg != "" {
			return nil, fmt.Errorf("%s: %s", b.name, msg)
		}
		return nil, fmt.Errorf("%s: %w", b.name, err)
	}
	return stdout.Bytes(), nil
}

// meansEmpty reports whether an error message from the paste command just
// says that the clipboard is empty.
func (b *cmdBackend) meansEmpty(msg string) bool {
	msg = strings.ToLower(msg)
	for _, hint := range b.emptyHints {
		if strings.Contains(msg, hint) {
			return true
		}
	}
	return false
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

// sysEnv is the part of the environment backend detection looks at,
// split out so detection can be tested without a desktop session.
type sysEnv struct {
	goos     string
	getenv   func(string) string
	lookPath func(string) (string, error)
}

func systemEnv() sysEnv {
	return sysEnv{goos: runtime.GOOS, getenv: os.Getenv, lookPath: exec.LookPath}
}

func (e sysEnv) has(cmd string) bool {
	_, err := e.lookPath(cmd)
	return err == nil
}

// selectBackend returns the backend called name, or detects one for "auto".
func selectBackend(name string, env sysEnv) (Backend, error) {
	if name == "" || name == "auto" {
		return detectBackend(env)
	}
	pc, ok := pasteCommands[name]
	if !ok {
		return nil, fmt.Errorf("unknown backend %q (want %s)", name, backendNames)
	}
	if !env.has(pc.argv[0]) {
		return nil, fmt.Errorf("backend %s: %s not found in PATH", name, pc.argv[0])
	}
	return newCmdBackend(name), nil
}

// detectBackend picks a backend for the current session: pbpaste on macOS,
// wl-paste under Wayland, then xclip or xsel under X11.
func detectBackend(env sysEnv) (Backend, error) {
	if env.goos == "darwin" {
		if env.has("pbpaste") {
			return newCmdBackend("pbpaste"), nil
		}
		return nil, errors.New("no clipboard backend available: pbpaste not found in PATH")
	}

	var why []string
	if env.getenv("WAYLAND_DISPLAY") != "" {
		if env.has("wl-paste") {
			return newCmdBackend("wl-paste"), nil
		}
		why = append(why, "WAYLAND_DISPLAY is set but wl-paste is not installed (package wl-clipboard)")
	}
	if env.getenv("DISPLAY") != "" {
		for _, name := range []string{"xclip", "xsel"} {
			if env.has(name) {
				return newCmdBackend(name), nil
			}
		}
		why = append(why, "DISPLAY is set but neither xclip nor xsel is installed")
	}
	if len(why) == 0 {
		why = append(why, "neither WAYLAND_DISPLAY nor DISPLAY is set")
	}
	return nil, fmt.Errorf("no clipboard backend available: %s; install one or pick one with --backend",
		strings.Join(why, "; "))
}

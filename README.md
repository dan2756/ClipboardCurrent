# clipwatch

Streams new clipboard contents to stdout.

`clipwatch` polls the clipboard and prints each new clip once, as it changes,
so whatever you copy can flow into an ordinary Unix pipeline:

```sh
clipwatch | grep -E '^https?://' >> links.txt
```

## Install

```sh
go install github.com/dan2756/clipwatch@latest
```

clipwatch shells out to the platform's paste tool, so one of these must be
installed:

| Platform      | Tool                                 |
| ------------- | ------------------------------------ |
| macOS         | `pbpaste` (built in)                 |
| Linux Wayland | `wl-paste` from wl-clipboard         |
| Linux X11     | `xclip` or `xsel`                    |

## Usage

```
clipwatch [flags]
```

| Flag                  | Description                                                   |
| --------------------- | ------------------------------------------------------------- |
| `-i`, `--interval`    | Poll interval, e.g. `250ms`, `2s`; a bare number is ms. Default `500ms`. |
| `--backend NAME`      | `auto` (default), `pbpaste`, `wl-paste`, `xclip` or `xsel`.   |
| `--paste-cmd CMD`     | Read the clipboard by running `sh -c CMD` instead of a built-in backend. |
| `--include-current`   | Also print what is on the clipboard at startup.               |
| `-0`, `--null`        | End each clip with a NUL byte instead of a newline.           |
| `--json`              | Print one JSON object per clip (NDJSON).                      |
| `--timestamp`         | Prefix each clip with an RFC 3339 timestamp and a tab.        |
| `--exec CMD`          | Run `sh -c CMD` once per clip, with the clip on stdin, instead of printing. |
| `--filter REGEX`      | Only emit clips matching `REGEX` ([Go syntax](https://pkg.go.dev/regexp/syntax)). |
| `--max-bytes SIZE`    | Skip clips larger than `SIZE` (`4096`, `64KB`, `1MB`; units are 1024-based). |
| `--trim`              | Strip leading and trailing whitespace.                        |
| `--ignore-whitespace-only` | Skip clips that contain nothing but whitespace.          |
| `--dedupe N`          | Skip clips identical to any of the last `N` printed. Default `1`; `0` disables. |
| `--version`           | Print the version.                                            |

What is already on the clipboard when clipwatch starts is not printed unless
`--include-current` is given. Stop it with Ctrl-C.

## Output formats

**Lines (default).** Each clip is followed by a newline, unless it already
ends with one. Good for single-line clips such as URLs; a multi-line clip is
indistinguishable from several clips.

**NUL-terminated (`-0`).** Each clip is written exactly as copied, followed
by a NUL byte. Safe for multi-line clips and pairs with `xargs -0`:

```sh
clipwatch -0 | xargs -0 -n1 notify-send Copied
```

**JSON (`--json`).** One object per line with the local time (RFC 3339 with
milliseconds), the clip size in bytes, and the text:

```json
{"time":"2026-08-03T12:32:52.123+02:00","bytes":19,"text":"https://example.com"}
```

`text` is a JSON string, so invalid UTF-8 is replaced with U+FFFD; `bytes`
is always the exact size of the emitted clip.

**Timestamps (`--timestamp`).** Adds `2026-08-03T12:32:52+02:00<TAB>` in
front of each clip in line or NUL mode. JSON already carries the time.

## Backends

With `--backend auto` (the default) clipwatch picks:

1. `pbpaste` on macOS;
2. `wl-paste --no-newline --type text` when `WAYLAND_DISPLAY` is set;
3. `xclip -selection clipboard -o`, then `xsel --clipboard --output`, when
   `DISPLAY` is set.

If nothing fits, it exits with an error that says what it looked for.
`--backend NAME` skips detection and uses that tool.

### Other clipboards: `--paste-cmd`

Anything that prints a clipboard to stdout can be watched with
`--paste-cmd CMD`, which runs `CMD` with `sh -c` on every poll. A non-zero
exit is reported as a read error.

```sh
# WSL: the Windows clipboard (Get-Clipboard ends lines with CRLF, hence --trim).
clipwatch --trim --paste-cmd 'powershell.exe -NoProfile -Command Get-Clipboard'

# The X11 primary selection, i.e. whatever text is highlighted.
clipwatch --paste-cmd 'xclip -selection primary -o'

# The tmux paste buffer, or Termux on Android.
clipwatch --paste-cmd 'tmux show-buffer'
clipwatch --paste-cmd termux-clipboard-get
```

Keep `--interval` in mind for slow commands: `powershell.exe` takes a few
hundred milliseconds to start, so `-i 1s` is kinder there.

### Empty clipboards and errors

An empty clipboard, or one holding only non-text data such as an image, is
not an error: wl-paste and xclip complain and exit non-zero in that case,
and clipwatch treats it as empty. Other read errors after startup are
reported once on stderr and polling continues; an error on the very first
read is fatal, since it usually means the tool cannot reach the display.

## Running a command per clip

`--exec CMD` runs `CMD` with `sh -c` for every clip instead of printing it.
The clip is on the command's stdin, exactly as it would have been printed
(after `--trim`), and these variables are set:

| Variable          | Value                         |
| ----------------- | ----------------------------- |
| `CLIPWATCH_BYTES` | Size of the clip in bytes.    |

The command's own stdout and stderr go to clipwatch's. Commands run one at a
time and polling pauses while one runs, so they never overlap; anything
copied in the meantime is seen as a single change once the command exits. A
command that fails is reported on stderr and the watch carries on.

```sh
# Desktop notification for every copy.
clipwatch --exec 'notify-send "Copied $CLIPWATCH_BYTES bytes"'

# Append each multi-line clip to a log, with a separator.
clipwatch --exec 'cat >> ~/clips.log; printf "\n---\n" >> ~/clips.log'
```

`--exec` cannot be combined with `--null`, `--json` or `--timestamp`, since
nothing is printed.

## Filters

Filters run in this order, and a clip that fails one is silently skipped:

1. `--trim` strips surrounding whitespace (everything below sees the trimmed text).
2. Empty clips, such as a cleared clipboard, are always skipped.
3. `--ignore-whitespace-only` skips clips that are only spaces, tabs and newlines.
4. `--max-bytes` skips clips that are too large.
5. `--filter` keeps only clips the regular expression matches. It is matched
   against the whole clip, so `^` and `$` anchor to its start and end; use
   `(?m)` for per-line anchors.
6. `--dedupe N` skips a clip identical to one of the last `N` clips that were
   actually printed. The default of 1 hides the common "copy, clear, copy the
   same thing again" pattern; a larger window keeps a history file free of
   repeats. Only hashes are kept, so a large `N` is cheap.

```sh
# Collect git commit hashes as they are copied.
clipwatch --trim --filter '^[0-9a-f]{7,40}$' >> shas.txt

# A clipboard history without repeats from the last 500 clips.
clipwatch --trim --dedupe 500 --timestamp >> ~/.clipboard-history
```

## Exit status

| Code | Meaning                                                              |
| ---- | -------------------------------------------------------------------- |
| 0    | Stopped by Ctrl-C or SIGTERM, or whatever was reading stdout exited. |
| 1    | No usable backend, the first clipboard read failed, or output could not be written. |
| 2    | Invalid command line.                                                |

When stdout is a pipe, clipwatch notices on its next poll that the reader has
gone away, so `clipwatch | head -3` returns right after the third clip
rather than waiting for a fourth copy to fail with a broken pipe.

## License

MIT

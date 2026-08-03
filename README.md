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
| `--include-current`   | Also print what is on the clipboard at startup.               |
| `-0`, `--null`        | End each clip with a NUL byte instead of a newline.           |
| `--json`              | Print one JSON object per clip (NDJSON).                      |
| `--timestamp`         | Prefix each clip with an RFC 3339 timestamp and a tab.        |
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
is always the size of the clip as copied.

**Timestamps (`--timestamp`).** Adds `2026-08-03T12:32:52+02:00<TAB>` in
front of each clip in line or NUL mode. JSON already carries the time.

## License

MIT

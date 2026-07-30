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
| `--version`           | Print the version.                                            |

What is already on the clipboard when clipwatch starts is not printed unless
`--include-current` is given. Each clip is followed by a newline. Stop it with
Ctrl-C.

## License

MIT

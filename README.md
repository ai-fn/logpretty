<p align="center">
  <img src="assets/logo.svg" width="64" height="64" alt="logpretty logo" />
</p>

<h1 align="center">logpretty</h1>

<p align="center">
  Pretty-print JSON, logfmt and plain-text logs from stdin, with level, field and
  expression filters. Built for <code>kubectl logs -f</code>, <code>docker</code> and <code>tail -f</code>.
</p>

<p align="center">
  <a href="https://github.com/ai-fn/logpretty/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/ai-fn/logpretty/actions/workflows/ci.yml/badge.svg" /></a>
  <img alt="Go version" src="https://img.shields.io/github/go-mod/go-version/ai-fn/logpretty?logo=go&logoColor=white&color=00ADD8" />
  <img alt="golangci-lint" src="https://img.shields.io/badge/golangci--lint-v2-00ADD8?logo=go&logoColor=white" />
  <img alt="License" src="https://img.shields.io/github/license/ai-fn/logpretty" />
</p>

---

`logpretty` reads a log stream from stdin and prints it in a readable form. It detects the
format of **every line** on its own (JSON, then logfmt, then plain text), so mixed streams
from `kubectl` and `docker compose` work without flags. Output is streamed: it is flushed
whenever the next read would block, so `kubectl logs -f` shows lines as they arrive.

```bash
kubectl logs -f pod | logpretty --level warn --where 'user_id=42' --fields msg,latency
```

> [!NOTE]
> **Status: early development.** Streaming stdin to stdout works, and for now the output equals
> the input. Format detection, pretty rendering, colors and the filters are not implemented yet,
> so the command above is the target usage, not something you can run today.

## Features

| Feature | Status |
|---|---|
| Stream stdin to stdout line by line, any line length (longer lines are truncated with an `…[+N]` marker and the stream keeps flowing) | Available |
| `-max-line N`: longest line to keep, in bytes (default 1 MiB) | Available |
| Exit codes: `0` success, `1` read/write error, `2` invalid arguments, `130`/`143` on SIGINT/SIGTERM | Available |
| Graceful stop: on SIGINT or SIGTERM the complete lines already read are written out before exit | Available |
| Quiet exit when the reader of stdout leaves early, as in `logpretty \| head` | Available |
| Per-line format detection: JSON, logfmt, plain text | Not yet |
| Readable output with aligned level badges, timestamps and `key=value` fields | Not yet |
| Colors: `--color=auto\|always\|never`, `NO_COLOR` support | Not yet |
| Safe output: control characters from logs never reach the terminal raw | Not yet |
| Filters: `--level`, `--where 'key OP value'`, `--fields a,b` | Not yet |

## Installation

```bash
go install github.com/ai-fn/logpretty/cmd/logpretty@latest
```

Or build from source:

```bash
git clone https://github.com/ai-fn/logpretty.git
cd logpretty
make build          # produces bin/logpretty
```

## Usage

For now `logpretty` passes lines through: it turns CRLF into LF and marks truncated lines.

```console
$ printf 'one\r\ntwo\n' | logpretty
one
two
$ printf '0123456789\n' | logpretty -max-line 4
0123…[+6]
```

## Development

Requires Go 1.26+, [golangci-lint](https://golangci-lint.run) v2 and `make`. CI runs the tests
on Linux, macOS and Windows, golangci-lint, `go mod tidy` and [govulncheck](https://go.dev/doc/security/vuln/)
on every pull request and every push to `main`.

| Command | Description |
|---|---|
| `make build` | Build `bin/logpretty` |
| `make test` | Run tests with the race detector and coverage |
| `make lint` | Run golangci-lint (includes gofumpt formatting) |
| `make check` | `test` + `lint`: run this before opening a pull request |
| `make vuln` | Check dependencies and the standard library for known vulnerabilities |
| `make fuzz FUZZ=FuzzName PKG=./path` | Fuzz one target |
| `make bench` | Run benchmarks |
| `make help` | List all targets |

## Project structure

```
cmd/logpretty/        # main() maps signals to a context and calls app.Run
internal/app/         # flags and the read-to-write pipeline
internal/linereader/  # reads lines of any length and truncates at a limit
internal/entry/       # parsed form of a log line: fields, level (not wired into the binary yet)
internal/parse/       # tells JSON, logfmt and plain lines apart, finds time, level and message (not wired in yet)
testdata/script/      # end-to-end scenarios run against the real binary (testscript)
assets/               # logo
.github/workflows/    # CI
Makefile              # build, test, lint, fuzz, bench, vuln
.golangci.yml         # golangci-lint v2 config
```

## Contributing

Bug reports and pull requests are welcome. Run `make check` before opening a pull request.

## License

MIT, see [`LICENSE`](LICENSE) for the full text.

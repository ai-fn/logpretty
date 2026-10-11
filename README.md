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
> **Status: early development.** Stdin reading and per-line format detection work. Pretty
> rendering, colors and the filters are not implemented yet, so the command above is the
> target usage, not something you can run today.

## Features

| Feature | Status |
|---|---|
| Read stdin line by line, any line length (lines over 1 MiB are truncated, the stream keeps flowing) | Available |
| Per-line format detection: JSON, logfmt, plain text | Available (prototype) |
| Exit codes: `0` success, `1` read/write error, `2` invalid arguments | Available |
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

For now `logpretty` prints one summary line per input line: the detected format (plus the `id` for JSON) and the line length.

```console
$ printf '{"id":1,"msg":"started"}\nlevel=warn msg=slow ms=12\nplain text\n' | logpretty
json id="1"                    len=24
logfmt                         len=25
text                           len=10
```

## Development

Requires Go 1.26+, [golangci-lint](https://golangci-lint.run) v2 and `make`. CI runs the tests
on Linux and macOS, golangci-lint, `go mod tidy` and [govulncheck](https://go.dev/doc/security/vuln/)
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
cmd/logpretty/        # CLI entrypoint: run() holds the logic, main() only calls it
assets/               # logo
.github/workflows/    # CI
Makefile              # build, test, lint, fuzz, bench, vuln
.golangci.yml         # golangci-lint v2 config
```

## Contributing

Bug reports and pull requests are welcome. Run `make check` before opening a pull request.

## License

MIT, see [`LICENSE`](LICENSE) for the full text.

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
  <a href="https://github.com/ai-fn/logpretty/releases/latest"><img alt="Latest release" src="https://img.shields.io/github/v/release/ai-fn/logpretty?color=00ADD8" /></a>
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

<p align="center">
  <img src="assets/demo.gif" width="800" alt="A terminal: five raw log lines in JSON, logfmt and plain text; the same lines printed by logpretty, with the time and the keys faint and each level in its own color; then only the warnings and errors of one user, with two fields" />
</p>

## Features

| Feature | Status |
|---|---|
| Stream stdin to stdout line by line, any line length (longer lines are cut and marked with what was dropped, as in `…[+2.1MiB]`, and the stream keeps flowing) | Available |
| `--max-line SIZE`: longest line to keep, in bytes or with a binary unit such as `64K` or `2M` (default `1M`) | Available |
| Exit codes: `0` success, `1` read/write error, `2` invalid arguments, `130`/`143` on SIGINT/SIGTERM | Available |
| Graceful stop: on SIGINT or SIGTERM the complete lines already read are written out before exit | Available |
| Quiet exit when the reader of stdout leaves early, as in `logpretty \| head` | Available |
| Per-line format detection: JSON, logfmt, plain text; JSON after the prefix of `kubectl logs --timestamps` or `docker compose logs` | Available |
| Readable output: `time LEVEL message  key=value …`, fields in their original order, nested objects as compact JSON | Available |
| Times in the local zone, or in UTC with `--utc` | Available |
| Safe output: control characters from logs never reach the terminal raw | Available |
| Colors: `--color=auto\|always\|never`; `auto` colors only a terminal and respects [`NO_COLOR`](https://no-color.org) and `TERM=dumb` | Available |
| `--level LEVEL`: show only the entries of that level and above; by default plain lines, such as a stack trace, go the way of the entry before them | Available |
| `--where EXPR`: show only the entries that meet an expression, such as `user_id=42`, `http.status>=500` or `msg~timeout`; given twice, it asks for both | Available |
| `--plain follow\|show\|hide`: what to do with the lines that are not entries: send them the way of the entry before them when a filter is on (the default), show them all, or hide them all, filter or not | Available |
| `--fields msg,latency`: show only the fields named, in the order of the list, nested ones as `http.status`; the message too, in its usual place, if `msg` is named; `--no-time` leaves the time out | Available |

## Installation

With [Homebrew](https://brew.sh) on macOS:

```bash
brew install --cask ai-fn/tap/logpretty
```

Or take the archive for your system from the
[latest release](https://github.com/ai-fn/logpretty/releases/latest), unpack it and put `logpretty`
on your `PATH`. The archives are named `logpretty_<version>_<os>_<arch>`, where `darwin` is macOS
and `amd64` is x86-64; there is one for Linux, macOS and Windows, each for x86-64 and ARM64. The
`checksums.txt` of the release holds the SHA-256 of every archive; with both files in one
directory:

```bash
shasum -a 256 --ignore-missing -c checksums.txt    # or: sha256sum --ignore-missing -c checksums.txt
```

It must print the name of the archive and `OK`: anything else, or nothing at all, means that the
archive is not the one that was released.

The macOS binaries are not notarized by Apple. The cask clears the quarantine mark itself. If macOS
refuses to open a binary from an archive that a browser downloaded, clear the mark on the unpacked
file: `xattr -d com.apple.quarantine logpretty`. `curl` does not set the mark.

With Go 1.26 or later, on any system:

```bash
go install github.com/ai-fn/logpretty/cmd/logpretty@latest
```

Or build from source:

```bash
git clone https://github.com/ai-fn/logpretty.git
cd logpretty
make build          # produces bin/logpretty
```

`logpretty --version` prints the version that is installed.

## Usage

Pipe any log stream into `logpretty`. Each line is rendered by its own format, and a line that is
not structured is printed as it is, apart from control characters:

```console
$ cat app.log
{"time":"2026-10-11T12:00:00.123Z","level":"info","msg":"server started","port":8080}
time=2026-10-11T12:00:01Z level=warn msg="slow request" latency=1.2s path=/api/v1/users user_id=42
web-1  | {"time":"2026-10-11T12:00:02Z","level":"error","msg":"query failed","user_id":42,"db":{"host":"pg-1","retries":3}}
panic: runtime error: index out of range
{"time":"2026-10-11T12:00:03Z","level":"error","msg":"query failed","user_id":7,"db":{"host":"pg-2","retries":1}}
$ logpretty --utc < app.log
12:00:00.123 INFO  server started  port=8080
12:00:01.000 WARN  slow request  latency=1.2s path=/api/v1/users user_id=42
web-1  | 12:00:02.000 ERROR query failed  user_id=42 db={"host":"pg-1","retries":3}
panic: runtime error: index out of range
12:00:03.000 ERROR query failed  user_id=7 db={"host":"pg-2","retries":1}
```

Control characters are written out, so a log line cannot change the terminal, and a line longer
than `--max-line` is cut with a note of how much was dropped:

```console
$ printf '{"msg":"\\u001b]0;pwned\\u0007"}\n' | logpretty
\x1b]0;pwned\x07
$ printf '0123456789\n' | logpretty --max-line 4
0123…[+6B]
```

On a terminal the output is colored: the time, the keys and a collector's prefix are faint, and
each level has its own style. Into a pipe or a file it is plain, unless `--color=always` asks for
colors, for example for `less -R`:

```bash
kubectl logs -f pod | logpretty --color=always | less -R
```

### Filtering

`--level` shows the entries of a level and above, and `--where` the ones that meet an expression.
An entry has to pass every filter that is given:

```console
$ logpretty --utc --level warn --where 'user_id=42' < app.log
12:00:01.000 WARN  slow request  latency=1.2s path=/api/v1/users user_id=42
web-1  | 12:00:02.000 ERROR query failed  user_id=42 db={"host":"pg-1","retries":3}
panic: runtime error: index out of range
$ logpretty --utc --where 'msg~^query' --where 'db.host!=pg-1' < app.log
12:00:03.000 ERROR query failed  user_id=7 db={"host":"pg-2","retries":1}
```

An expression is a key alone, or a key, an operator and a value; the key names a field of the
entry. The spaces around the operator may be left out:

| Expression | Shows the entries where |
|---|---|
| `key` | the field is there, whatever its value |
| `key=value` | the field equals the value |
| `key!=value` | it does not, or the field is missing |
| `key>value`, `key>=value`, `key<value`, `key<=value` | the field is a number above or below the value; `>=` and `<=` take an equal one too |
| `key~regexp` | the regular expression ([RE2 syntax](https://github.com/google/re2/wiki/Syntax)) matches a part of the field |
| `key!~regexp` | it does not, or the field is missing |

- **Numbers and text.** `=` and `!=` compare two numbers as numbers, so `user_id=42` finds `42`,
  `"42"`, `42.0` and the logfmt `user_id=42`. A string that reads as a number is one: `id=42` also
  finds `"042"` and `version=1.10` finds `"1.1"`. A value in double quotes is text, and finds only
  what is written the very same way: `--where 'version="1.10"'` leaves `"1.1"` out. Anything that
  is not a number is compared as text too, byte for byte, case and all. `>`, `>=`, `<` and `<=`
  compare numbers only, in quotes or not: `latency>100ms` is an error instead of being compared as
  text. The keys `level` and `time` are the exception to all of this: they are compared as a level
  and as a moment, in quotes or not.
- **Keys.** `db.host` is the field of that very name if the entry has one, and otherwise `host`
  inside the object `db`; arrays are not looked into. `msg`, `level` and `time` stand for the
  message, the level and the time of the entry, whatever key the log keeps them under:
  `level>=warn` compares by severity and `time>=2026-10-11T12:00:02Z` by the moment. A time without
  a zone is local, or UTC with `--utc`; the timestamp a collector puts in front of a line is not
  the time of the entry. A key in double quotes, as in `"level"=3`, is the top-level field of that
  very name: it stands for nothing else and does not look into objects.
- **Quotes.** A value with a space goes in double quotes, inside the quotes of the shell:
  `--where 'msg="slow request"'`. Inside the double quotes `\"` is a quote and `\\` a backslash;
  any other backslash stays, so `msg~"\d+ ms"` works. An empty value is written `""`. Without
  quotes a value cannot start with an operator, a bracket or `'`, so a regular expression that
  does goes in quotes: `msg~"(?i)timeout"`.
- **Missing fields.** An entry without the field meets `!=` and `!~` and nothing else, so
  `--where 'status!=200'` also shows the entries that have no status; add `--where status` to
  leave them out.
- **Several conditions.** Give the flag again: every condition has to hold. There is no `or`.

A mistake in an expression is shown with a caret under it, and no input is read. The caret is
placed by characters, so wide ones, such as CJK and emoji, shift it:

```console
$ logpretty --where 'user_id=>42' < app.log
logpretty: --where 'user_id=>42'
                            ^ unexpected '>'
Run 'logpretty --help' for usage.
```

A line that is not an entry, such as the panic above, goes the way of the entry before it: it is
shown after a shown entry and hidden after a hidden one, so a stack trace stays with its error.
`--plain show` shows such lines whatever the filters say, and `--plain hide` hides them all, even
when nothing is filtered:

```console
$ logpretty --utc --level error --plain hide < app.log
web-1  | 12:00:02.000 ERROR query failed  user_id=42 db={"host":"pg-1","retries":3}
12:00:03.000 ERROR query failed  user_id=7 db={"host":"pg-2","retries":1}
```

### Choosing fields

`--fields` names the fields to show, in the order to show them in; a nested one is written with
dots. The time and the level are shown anyway, and the message, in its usual place, if `msg` is in
the list. `time` and `level` may be named too: `--fields level` shows the time and the level of
every entry that has a level. `--no-time` leaves the time out:

```console
$ logpretty --utc --fields msg,db.host < app.log
12:00:00.123 INFO  server started
12:00:01.000 WARN  slow request
web-1  | 12:00:02.000 ERROR query failed  db.host=pg-1
panic: runtime error: index out of range
12:00:03.000 ERROR query failed  db.host=pg-2
$ logpretty --no-time --fields latency,path < app.log
WARN  latency=1.2s path=/api/v1/users
panic: runtime error: index out of range
```

An entry that has none of the fields is not printed, as in the second command. The lines that are
not entries are left to the filters and to `--plain`, so the stack trace of such an entry stays.

All of it together:

```console
$ logpretty --utc --level warn --where 'user_id=42' --fields msg,latency < app.log
12:00:01.000 WARN  slow request  latency=1.2s
web-1  | 12:00:02.000 ERROR query failed
panic: runtime error: index out of range
```

On Windows, `cmd.exe` does not take single quotes for quotes: write `--where "user_id=42"`, and
`\"` for a quote inside, as in `--where "msg=\"slow request\""`. Keep `&`, `|`, `<`, `>` and `^`
inside the double quotes. Between a pair of `\"` the shell takes them for its own, so put a `^`
before each one there: `--where "msg~\"^^slow request^|failed\""`.

PowerShell takes the single quotes, but it cannot read a file with `<`, so pipe the file in:
`Get-Content app.log | logpretty`. Windows PowerShell 5.1 does not pass the double quotes inside an
argument as they are written. Put `--%` before the arguments there, which makes it pass the rest of
the line as it is written, so a single quote is no quote there and a quote inside double quotes is
`\"`: `logpretty --% --where "msg=\"slow request\""`. PowerShell 7.3 and later passes the quotes
as they are. A pipe of Windows PowerShell 5.1 also turns the characters outside ASCII into `?`,
which `cmd /c "logpretty < app.log"` does not.

### Recipes

```bash
# follow one pod of a deployment: warnings and above, each with the stack trace that follows it
kubectl logs -f deploy/api | logpretty --level warn

# kubectl --timestamps and docker compose put a prefix before each line; logpretty reads past it
kubectl logs -f deploy/api --timestamps | logpretty --where 'http.status>=500'
docker compose logs -f | logpretty --where 'msg~timeout' --fields msg,latency

# a file that is still being written: errors and above, without the lines that are not entries
tail -f app.log | logpretty --level error --plain hide

# what one user did in one hour of an old log, paged with colors
logpretty --color=always --where 'user_id=42' --where 'time>=2026-10-11T12:00:00Z' \
  --where 'time<2026-10-11T13:00:00Z' < app.log | less -R
```

### Flags

| Flag | Description |
|---|---|
| `--level LEVEL` | Show the entries of this level or above: `trace`, `debug`, `info`, `warn`, `error` or `fatal`. An entry without a level is hidden, and by default so are the plain lines after a hidden entry (see `--plain`) |
| `--where EXPR` | Show the entries that meet the expression: `key`, or `key OP value` with `=`, `!=`, `>`, `>=`, `<`, `<=`, `~` (a regular expression) or `!~`. Given twice, it asks for both. By default the plain lines after a hidden entry are hidden too (see `--plain`). The expressions are described under [Filtering](#filtering) |
| `--plain MODE` | What to do with the lines that are not entries, such as a stack trace: `follow` (the default) shows every line when nothing is filtered, and under `--level` or `--where` a line only after an entry that is shown; `show` shows them all, and `hide` hides them all, filter or not |
| `--fields LIST` | Show only these fields, in this order: names separated by commas, as in `latency,http.status`. The time and the level are shown anyway; so is the message, in its usual place, if `msg` is in the list. An entry with none of the fields is not printed, while the plain lines after it still are |
| `--no-time` | Leave the time out |
| `--color WHEN` | When to color the output: `auto` (on a terminal, the default), `always` or `never`. `always` also beats `NO_COLOR` |
| `--utc` | Show times in UTC instead of the local time zone |
| `--max-line SIZE` | Longest line to keep: a number of bytes, or one with `K`, `M` or `G` for KiB, MiB or GiB (default `1M`) |
| `--version` | Print the version |
| `-h`, `--help` | Print the help with examples |

A flag may also be written with one dash, as in `-utc`.

Git Bash and other terminals on Windows that give a program a pipe instead of a console get no
colors in the `auto` mode; use `--color=always` there.

## Development

Requires Go 1.26+, [golangci-lint](https://golangci-lint.run) v2 and `make`. CI runs the tests
on Linux, macOS and Windows, golangci-lint, `go mod tidy`, [govulncheck](https://go.dev/doc/security/vuln/)
and a check of the GoReleaser config on every pull request and every push to `main`. A tag such as
`v0.1.0` starts the release: the archives are built by GoReleaser and published with their
checksums.

| Command | Description |
|---|---|
| `make build` | Build `bin/logpretty` |
| `make test` | Run tests with the race detector and coverage, then without it |
| `make lint` | Run golangci-lint (includes gofumpt formatting) |
| `make check` | `test` + `lint`: run this before opening a pull request |
| `make vuln` | Check dependencies and the standard library for known vulnerabilities |
| `make fuzz FUZZ=FuzzName PKG=./path` | Fuzz one target |
| `make bench` | Run benchmarks |
| `make demo` | Record `assets/demo.gif` from `assets/demo.tape`; needs [vhs](https://github.com/charmbracelet/vhs) |
| `make snapshot` | Build the release archives into `dist/` without publishing them; needs [GoReleaser](https://goreleaser.com) v2 |
| `make help` | List all targets |

## Project structure

```
cmd/logpretty/        # main() maps signals to a context and calls app.Run
internal/app/         # the pipeline: read, parse, render, write
internal/cli/         # reads the command line into a config; the text of --help
internal/linereader/  # reads lines of any length and truncates at a limit
internal/entry/       # parsed form of a log line: fields, level
internal/parse/       # tells JSON, logfmt and plain lines apart, finds time, level and message
internal/filter/      # decides which lines are shown: --level, the expressions of --where, the plain lines of --plain
internal/render/      # formats an entry as a line of text (--fields, --no-time) and keeps control characters out of the terminal
internal/term/        # decides whether to color the output: the --color mode, the terminal, NO_COLOR
internal/golden/      # test helper: compares output with golden files
testdata/script/      # end-to-end scenarios run against the real binary (testscript)
assets/               # logo, the recording of the output and what it is made from
.github/workflows/    # CI, and the release that a version tag starts
Makefile              # build, test, lint, fuzz, bench, vuln, snapshot, demo
.golangci.yml         # golangci-lint v2 config
.goreleaser.yaml      # GoReleaser config: what a release is built from
```

## Contributing

Bug reports and pull requests are welcome. Run `make check` before opening a pull request.

## License

MIT, see [`LICENSE`](LICENSE) for the full text.

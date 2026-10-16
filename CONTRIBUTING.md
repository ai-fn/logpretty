# Contributing to logpretty

Thank you for taking the time. Bug reports, ideas and pull requests are all welcome. By taking
part you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Reporting a bug

Use the [bug report form](https://github.com/ai-fn/logpretty/issues/new?template=bug.yml). The
fastest bugs to fix come with the whole command, a line or two of input and what you expected.
Take secrets out of the log lines first.

Do not report a security problem in a public issue. Write to the maintainer
([@ai-fn](https://github.com/ai-fn)) through their GitHub profile instead.

## Suggesting a change

For a new flag, a new input format or a change in the output, open an issue first and describe
the case it solves. This saves you from writing a patch that cannot be merged. A fix for a bug
or for the docs can go straight to a pull request.

## Setting up

You need Go (the version in [`go.mod`](go.mod)), [golangci-lint](https://golangci-lint.run) v2
and `make`.

```sh
git clone https://github.com/ai-fn/logpretty
cd logpretty
make build            # bin/logpretty
make check            # tests with the race detector, then the linter
```

`make help` lists the other targets: `fuzz`, `bench`, `vuln`, `snapshot`, `demo`.

## Making a change

1. Fork the repository and branch off `develop`, not `main`: `main` holds releases only.
   Name the branch `feature/<name>`.
2. Write the test first: a table-driven case for the behavior you change. Output is compared
   with golden files; rewrite them with `go test ./internal/<pkg> -update` (`render`, `parse`
   and `cli` have them) and read the diff.
   An end-to-end case goes into [`testdata/script`](testdata/script) as a `testscript` file.
3. Run `make check`. If you touched platform-specific code, also run `GOOS=windows go vet ./...`.
4. If you changed a parser, the filter or the renderer, fuzz it for a few minutes:
   `make fuzz FUZZ=FuzzParse PKG=./internal/parse FUZZTIME=5m`. List the targets with
   `grep -rn 'func Fuzz' internal`.
5. Open a pull request against `develop`. Say what changed and why, and link the issue.

CI runs the tests on Linux, macOS and Windows, the linter, `go mod tidy`, govulncheck and a
check of the GoReleaser config. All of them should be green before a pull request is merged.

## Code

- Format with `gofumpt`; `make lint` checks it.
- Everything that reaches the terminal goes through `appendSafe`, so a log line cannot inject
  escape sequences. Parts of a line (message, key, value) go through `AppendInline`
(both in `internal/render`).
- A line from the reader and an `Entry` from the parser are valid only until the next line is
  read or parsed. Anything kept longer must be copied (`bytes.Clone`).
- All the logic of the command lives in `app.Run` (`internal/app`); `main` only handles signals
  and calls it.
- Keep dependencies to the minimum. A new one needs a good reason in the pull request.
- Comments and docstrings are in English, one or two lines, and say why, not what.
- Do not slow the hot path down: if you touch the reader, the parser or the renderer, compare
  `make bench` before and after (`benchstat` helps) and include the numbers.

## Commits

Use [Conventional Commits](https://www.conventionalcommits.org) in English, such as
`feat(filter): add --where operator` or `fix(parse): read a Serilog event without @l`. Keep the
subject within 72 characters; add a body only when the reason is not obvious. The release notes
are built from these messages; `test`, `ci`, `chore`, `build` and `refactor` commits are left out.

## Releases

Maintainers only. A release is prepared on `release/vX.Y.Z` off `develop`, tried as
`vX.Y.Z-rc.N`, merged into `main` and tagged `vX.Y.Z`; the tag starts GoReleaser.
An urgent fix goes through `hotfix/vX.Y.Z` off `main`.

## License

By contributing you agree that your work is released under the [MIT License](LICENSE).

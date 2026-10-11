# logpretty

Pretty-print JSON, logfmt and plain-text logs from stdin, with level, field and expression filters.

```bash
kubectl logs -f pod | logpretty --level warn --where 'user_id=42' --fields msg,latency
```

> **Status:** early development, not usable yet.

## Install

```bash
go install github.com/ai-fn/logpretty/cmd/logpretty@latest
```

## Development

Requires Go 1.26+ and [golangci-lint](https://golangci-lint.run) v2 (it also enforces gofumpt formatting).

```bash
make build  # build bin/logpretty
make test   # tests with -race and coverage
make lint   # golangci-lint
make vuln   # govulncheck
make help   # all targets
```

## License

[MIT](LICENSE)

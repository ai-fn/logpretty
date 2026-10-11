// Command logpretty pretty-prints JSON, logfmt and plain-text logs read from stdin.
package main

import (
	"os"

	"github.com/ai-fn/logpretty/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

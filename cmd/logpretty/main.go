// Command logpretty pretty-prints JSON, logfmt and plain-text logs read from stdin.
package main

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/ai-fn/logpretty/internal/app"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	var signo atomic.Int32
	go func() {
		sig := <-sigs
		signal.Stop(sigs) // a second signal kills the process the default way if stdout is stuck
		if s, ok := sig.(syscall.Signal); ok {
			signo.Store(int32(s))
		}
		cancel()
	}()

	code := app.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	if n := signo.Load(); code == app.ExitInterrupted && n != 0 {
		code = 128 + int(n) // shell convention for death by signal
	}
	os.Exit(code)
}

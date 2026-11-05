// Command agk wires coding-agent workflows into agterm companions and overlays.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/shurastik/agterm-kit/internal/cli"
)

func main() {
	env, err := cli.NewEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "agk:", err)
		os.Exit(cli.ExitError)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := env.Main(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}

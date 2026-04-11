package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"codex-lover/internal/app"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := app.Run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "codex-lover: %v\n", err)
		os.Exit(1)
	}
}

package main

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/Periecle/teamcity-axi/internal/axi"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	var termination atomic.Int32
	go func() {
		select {
		case s := <-signals:
			if s == syscall.SIGTERM {
				termination.Store(143)
			} else {
				termination.Store(130)
			}
			cancel()
		case <-ctx.Done():
		}
	}()
	code := axi.Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	if t := termination.Load(); t != 0 {
		code = int(t)
	}
	os.Exit(code)
}

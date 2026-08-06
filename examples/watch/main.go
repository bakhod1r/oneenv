// Example: hot-reload configuration when the .env file changes.
//
// oneenv.NewLive holds the configuration and the lock it needs on the inside,
// so every goroutine can simply call Get and receive a snapshot.
package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/bakhod1r/oneenv"
)

type Config struct {
	Host    string        `env:"HOST" default:"localhost"`
	Port    int           `env:"PORT" default:"8080"`
	Timeout time.Duration `env:"TIMEOUT" default:"5s"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Loads once, then follows the files until ctx is cancelled. A broken
	// configuration fails here rather than in the background.
	live, err := oneenv.NewLive[Config](ctx, oneenv.WithEnvFiles())
	if err != nil {
		log.Fatal(err)
	}

	// Edit .env while this runs.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cfg := live.Get()
			fmt.Printf("serving on %s:%d\n", cfg.Host, cfg.Port)
			if err := live.Err(); err != nil {
				log.Printf("last reload failed, still on the previous values: %v", err)
			}
		}
	}
}

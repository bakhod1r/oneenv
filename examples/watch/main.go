// Example: hot-reload configuration when the .env file changes.
//
// watch.Watch writes to the same struct from its own goroutine, so guard it
// with a mutex (as here) or swap a pointer inside onReload.
package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bakhod1r/oneenv"
	"github.com/bakhod1r/oneenv/watch"
)

type Config struct {
	Host    string        `env:"HOST" default:"localhost"`
	Port    int           `env:"PORT" default:"8080"`
	Timeout time.Duration `env:"TIMEOUT" default:"5s"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// live is owned by Watch's goroutine; current is what the rest of the
	// program reads. onReload runs on the goroutine that just wrote live, so
	// copying there is the safe hand-off point.
	var (
		live    Config
		mu      sync.RWMutex
		current Config
	)

	onReload := func(err error) {
		if err != nil {
			log.Printf("reload failed, keeping previous config: %v", err)
			return
		}
		mu.Lock()
		current = live
		mu.Unlock()
		log.Printf("reloaded: %+v", live)
	}

	// Watch reports errors only for reloads, so seed current with an initial
	// load and fail fast if the configuration is broken at startup.
	if err := oneenv.Load(&current, oneenv.WithEnvFiles()); err != nil {
		log.Fatal(err)
	}

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mu.RLock()
				fmt.Printf("serving on %s:%d\n", current.Host, current.Port)
				mu.RUnlock()
			}
		}
	}()

	// Blocks until ctx is cancelled. Edit .env while it runs.
	if err := watch.Watch(ctx, &live, onReload, oneenv.WithEnvFiles()); err != nil {
		log.Fatal(err)
	}
}

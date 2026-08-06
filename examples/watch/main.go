// Example: hot-reload configuration when the .env file changes.
//
// oneenv.WithWatch re-decodes into the same struct from its own goroutine, so
// the value it writes is copied under a mutex inside onReload and the rest of
// the program reads the copy.
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
)

type Config struct {
	Host    string        `env:"HOST" default:"localhost"`
	Port    int           `env:"PORT" default:"8080"`
	Timeout time.Duration `env:"TIMEOUT" default:"5s"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// live is written by the watcher's goroutine; current is what the rest of
	// the program reads. onReload runs on the goroutine that just wrote live,
	// so copying there is the safe hand-off point.
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

	// Load returns as soon as the first decode is done; a broken configuration
	// fails fast here rather than in the background.
	err := oneenv.Load(&live,
		oneenv.WithEnvFiles(),
		oneenv.WithContext(ctx),
		oneenv.WithWatch(onReload),
	)
	if err != nil {
		log.Fatal(err)
	}
	current = live

	// Edit .env while this runs.
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
}

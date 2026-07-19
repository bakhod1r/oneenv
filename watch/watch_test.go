package watch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bakhod1r/oneenv"
)

func TestWatchReloads(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	if err := os.WriteFile(env, []byte("VALUE=one\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	type Config struct {
		Value string `env:"VALUE"`
	}
	var cfg Config
	// Reading cfg inside onReload is race-free: Watch writes cfg and then calls
	// onReload from the same goroutine, so the write happens-before the read.
	reloaded := make(chan string, 4)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel the watcher and wait for its goroutine to fully return before the
	// test exits. Leaving the goroutine running while the test binary writes
	// coverage counters on shutdown can corrupt them ("reading counter data
	// file: EOF") and fail the run under -cover.
	done := make(chan struct{})
	defer func() {
		cancel()
		<-done
	}()
	go func() {
		defer close(done)
		_ = Watch(ctx, &cfg, func(error) { reloaded <- cfg.Value },
			oneenv.WithFiles(env), oneenv.WithLookuper(oneenv.MapLookuper{}))
	}()

	// Give the watcher a moment to arm, then change the file.
	time.Sleep(200 * time.Millisecond)
	if err := os.WriteFile(env, []byte("VALUE=two\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-reloaded:
		if got != "two" {
			t.Errorf("after reload: got %q, want two", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reload not observed")
	}
}

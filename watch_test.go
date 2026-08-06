package oneenv_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bakhod1r/oneenv"
)

func TestWithWatchReloadsOnChange(t *testing.T) {
	oneenv.SetPollInterval(20 * time.Millisecond)

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("PORT=8080\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type conf struct {
		Port int `env:"PORT"`
	}
	var (
		mu     sync.Mutex
		cfg    conf
		reload = make(chan error, 4)
	)
	err := oneenv.Load(&cfg,
		oneenv.WithFiles(path),
		oneenv.WithLookuper(oneenv.MapLookuper{}),
		oneenv.WithContext(ctx),
		oneenv.WithWatch(func(err error) {
			mu.Lock()
			defer mu.Unlock()
			select {
			case reload <- err:
			default:
			}
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 8080 {
		t.Fatalf("initial Port = %d, want 8080", cfg.Port)
	}

	// Rewrite the file and wait for the reload to land.
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(path, []byte("PORT=9090\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(5 * time.Second)
	for {
		select {
		case err := <-reload:
			if err != nil {
				t.Fatalf("reload reported an error: %v", err)
			}
			mu.Lock()
			got := cfg.Port
			mu.Unlock()
			if got == 9090 {
				return // reloaded
			}
		case <-deadline:
			t.Fatal("no reload within 5s")
		}
	}
}

func TestWithWatchDoesNotStartOnFailedLoad(t *testing.T) {
	type conf struct {
		Port int `env:"PORT,required"`
	}
	var cfg conf
	called := false
	err := oneenv.Load(&cfg,
		oneenv.WithFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{}),
		oneenv.WithWatch(func(error) { called = true }),
	)
	if err == nil {
		t.Fatal("expected the required field to fail the load")
	}
	if called {
		t.Fatal("the reload callback ran even though the initial load failed")
	}
}

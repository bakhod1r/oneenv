package oneenv_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// syncWriter makes a writer safe to read from the test goroutine while the
// watcher writes to it.
type syncWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *syncWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func TestWithWatchWithoutCallback(t *testing.T) {
	oneenv.SetPollInterval(20 * time.Millisecond)

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("PORT=8080\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Without a callback the reload is reported through the logger, which is
	// also the only race-free way for this test to observe it: the target
	// itself is written by the watcher's goroutine.
	out := &syncWriter{}
	logger := slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	type conf struct {
		Port int `env:"PORT"`
	}
	var cfg conf
	err := oneenv.Load(&cfg,
		oneenv.WithFiles(path),
		oneenv.WithLookuper(oneenv.MapLookuper{}),
		oneenv.WithContext(ctx),
		oneenv.WithLogger(logger),
		oneenv.WithWatch(),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(path, []byte("PORT=9090\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), "oneenv: reloaded") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no reload logged within 5s:\n%s", out.String())
}

func TestWithWatchKeepsPreviousValuesOnFailedReload(t *testing.T) {
	oneenv.SetPollInterval(20 * time.Millisecond)

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("HOST=localhost\nPORT=8080\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type conf struct {
		Host string `env:"HOST"`
		Port int    `env:"PORT"`
	}
	var (
		cfg  conf
		done = make(chan conf, 4)
	)
	err := oneenv.Load(&cfg,
		oneenv.WithFiles(path),
		oneenv.WithLookuper(oneenv.MapLookuper{}),
		oneenv.WithContext(ctx),
		oneenv.WithWatch(func(err error) {
			if err != nil {
				// Snapshot on the goroutine that just tried the reload.
				select {
				case done <- cfg:
				default:
				}
			}
		}),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// HOST decodes fine, PORT does not: a reload that wrote fields as it went
	// would leave HOST updated and PORT stale.
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(path, []byte("HOST=changed\nPORT=not-a-number\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case got := <-done:
		if got.Host != "localhost" || got.Port != 8080 {
			t.Fatalf("failed reload left a half-updated config: %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no failed reload reported within 5s")
	}
}

func TestWithMutexGuardsTheSwap(t *testing.T) {
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
		mu  sync.RWMutex
		cfg conf
	)
	err := oneenv.Load(&cfg,
		oneenv.WithFiles(path),
		oneenv.WithLookuper(oneenv.MapLookuper{}),
		oneenv.WithContext(ctx),
		oneenv.WithMutex(&mu),
		oneenv.WithWatch(),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Read under the same lock oneenv takes for the swap; -race proves the two
	// no longer overlap.
	read := func() int {
		mu.RLock()
		defer mu.RUnlock()
		return cfg.Port
	}

	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(path, []byte("PORT=9090\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if read() == 9090 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the reload never landed within 5s")
}

// TestWithMutexUnderConcurrentReaders hammers the target from several
// goroutines while the file changes repeatedly. Run under -race it is the proof
// that WithMutex closes the window between a reload's swap and a reader.
func TestWithMutexUnderConcurrentReaders(t *testing.T) {
	oneenv.SetPollInterval(10 * time.Millisecond)

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("HOST=a\nPORT=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type conf struct {
		Host string `env:"HOST"`
		Port int    `env:"PORT"`
	}
	var (
		mu  sync.RWMutex
		cfg conf
	)
	err := oneenv.Load(&cfg,
		oneenv.WithFiles(path),
		oneenv.WithLookuper(oneenv.MapLookuper{}),
		oneenv.WithContext(ctx),
		oneenv.WithMutex(&mu),
		oneenv.WithWatch(),
	)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var (
		wg   sync.WaitGroup
		stop = make(chan struct{})
	)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				mu.RLock()
				// Read every field, so a torn write would be visible: the file
				// always sets HOST and PORT to matching generations.
				host, port := cfg.Host, cfg.Port
				mu.RUnlock()
				if want := strings.Repeat("a", port); host != want {
					t.Errorf("torn configuration: HOST=%q with PORT=%d", host, port)
					return
				}
				time.Sleep(time.Millisecond)
			}
		}()
	}

	for gen := 2; gen <= 12; gen++ {
		body := "HOST=" + strings.Repeat("a", gen) + "\nPORT=" + strconv.Itoa(gen) + "\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
	}

	close(stop)
	wg.Wait()
}

// TestLiveUnderConcurrentReaders is the same hammering as the WithMutex test,
// but with oneenv holding the lock: readers only ever see a snapshot.
func TestLiveUnderConcurrentReaders(t *testing.T) {
	oneenv.SetPollInterval(10 * time.Millisecond)

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("HOST=a\nPORT=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type conf struct {
		Host string `env:"HOST"`
		Port int    `env:"PORT"`
	}
	live, err := oneenv.NewLive[conf](ctx,
		oneenv.WithFiles(path),
		oneenv.WithLookuper(oneenv.MapLookuper{}),
	)
	if err != nil {
		t.Fatalf("NewLive: %v", err)
	}
	if got := live.Get(); got.Port != 1 || got.Host != "a" {
		t.Fatalf("initial value = %+v", got)
	}

	var (
		wg   sync.WaitGroup
		stop = make(chan struct{})
	)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				// Every generation writes HOST as PORT repetitions of "a", so a
				// torn snapshot would not satisfy this.
				cfg := live.Get()
				if want := strings.Repeat("a", cfg.Port); cfg.Host != want {
					t.Errorf("torn snapshot: HOST=%q with PORT=%d", cfg.Host, cfg.Port)
					return
				}
				time.Sleep(time.Millisecond)
			}
		}()
	}

	for gen := 2; gen <= 12; gen++ {
		body := "HOST=" + strings.Repeat("a", gen) + "\nPORT=" + strconv.Itoa(gen) + "\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
	}
	// The last write still has to travel through the notifier.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && live.Get().Port == 1 {
		time.Sleep(10 * time.Millisecond)
	}
	close(stop)
	wg.Wait()

	if got := live.Get(); got.Port == 1 {
		t.Fatal("the live configuration never picked up a change")
	}
	if err := live.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
}

func TestLiveKeepsLastGoodValue(t *testing.T) {
	oneenv.SetPollInterval(10 * time.Millisecond)

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
	live, err := oneenv.NewLive[conf](ctx,
		oneenv.WithFiles(path),
		oneenv.WithLookuper(oneenv.MapLookuper{}),
	)
	if err != nil {
		t.Fatalf("NewLive: %v", err)
	}

	time.Sleep(30 * time.Millisecond)
	if err := os.WriteFile(path, []byte("PORT=not-a-number\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if live.Err() != nil {
			if got := live.Get().Port; got != 8080 {
				t.Fatalf("a failed reload changed the value: %d", got)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the broken file never produced a reload error")
}

// TestLiveConcurrentGetAndLoad runs Get, Err and independent Loads of the same
// struct type side by side with a live reload, so the race detector also covers
// the shared schema cache the decoder keeps.
func TestLiveConcurrentGetAndLoad(t *testing.T) {
	oneenv.SetPollInterval(10 * time.Millisecond)

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("HOST=a\nPORT=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type conf struct {
		Host string `env:"HOST"`
		Port int    `env:"PORT"`
	}
	live, err := oneenv.NewLive[conf](ctx,
		oneenv.WithFiles(path),
		oneenv.WithLookuper(oneenv.MapLookuper{}),
	)
	if err != nil {
		t.Fatalf("NewLive: %v", err)
	}

	var (
		wg   sync.WaitGroup
		stop = make(chan struct{})
	)
	worker := func(fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				fn()
				time.Sleep(time.Millisecond)
			}
		}()
	}
	for i := 0; i < 4; i++ {
		worker(func() { _ = live.Get() })
		worker(func() { _ = live.Err() })
		worker(func() {
			// An unrelated Load of the same type, hitting the same schema cache.
			var other conf
			_ = oneenv.Load(&other, oneenv.WithFiles(path), oneenv.WithLookuper(oneenv.MapLookuper{}))
		})
	}

	for gen := 2; gen <= 8; gen++ {
		body := "HOST=" + strings.Repeat("a", gen) + "\nPORT=" + strconv.Itoa(gen) + "\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	close(stop)
	wg.Wait()
}

// TestLiveCancelStopsWatching checks that cancelling the context ends the
// watcher rather than leaving it running for the life of the process.
func TestLiveCancelStopsWatching(t *testing.T) {
	oneenv.SetPollInterval(10 * time.Millisecond)

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("PORT=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	type conf struct {
		Port int `env:"PORT"`
	}
	live, err := oneenv.NewLive[conf](ctx,
		oneenv.WithFiles(path),
		oneenv.WithLookuper(oneenv.MapLookuper{}),
	)
	if err != nil {
		t.Fatalf("NewLive: %v", err)
	}

	cancel()
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(path, []byte("PORT=9090\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)

	if got := live.Get().Port; got != 1 {
		t.Fatalf("the watcher kept running after cancel: Port = %d", got)
	}
}

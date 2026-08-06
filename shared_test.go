package oneenv_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bakhod1r/oneenv"
)

// atomicAdd and atomicGet keep the counters race-free; the callbacks run on the
// watcher's goroutine.
func atomicAdd(n *int64)       { atomic.AddInt64(n, 1) }
func atomicGet(n *int64) int64 { return atomic.LoadInt64(n) }

type sharedConf struct {
	Port int `env:"PORT" default:"8080"`
}

func TestSharedLoadsOnce(t *testing.T) {
	oneenv.ResetShared[sharedConf]()
	t.Cleanup(oneenv.ResetShared[sharedConf])

	// A mutator counts how many times the value was actually decoded.
	var decodes int64
	count := oneenv.WithMutator(func(_ context.Context, _, value string) (string, error) {
		atomicAdd(&decodes)
		return value, nil
	})

	first, err := oneenv.Shared[sharedConf](oneenv.WithFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{"PORT": "9090"}), count)
	if err != nil {
		t.Fatalf("Shared: %v", err)
	}
	if first.Port != 9090 {
		t.Fatalf("Port = %d, want 9090", first.Port)
	}

	// Different options, and they are ignored: the first call already decided.
	second, err := oneenv.Shared[sharedConf](oneenv.WithFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{"PORT": "1"}), count)
	if err != nil {
		t.Fatalf("Shared: %v", err)
	}
	if second != first {
		t.Fatal("Shared returned a different pointer on the second call")
	}
	if second.Port != 9090 {
		t.Fatalf("the second call re-read the configuration: Port = %d", second.Port)
	}
	if n := atomicGet(&decodes); n != 1 {
		t.Fatalf("the configuration was decoded %d times, want 1", n)
	}
}

func TestSharedConcurrentFirstCalls(t *testing.T) {
	oneenv.ResetShared[sharedConf]()
	t.Cleanup(oneenv.ResetShared[sharedConf])

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		seen = map[*sharedConf]int{}
	)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg, err := oneenv.Shared[sharedConf](oneenv.WithFiles(),
				oneenv.WithLookuper(oneenv.MapLookuper{"PORT": "7000"}))
			if err != nil {
				t.Errorf("Shared: %v", err)
				return
			}
			mu.Lock()
			seen[cfg]++
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(seen) != 1 {
		t.Fatalf("concurrent callers got %d different values, want 1", len(seen))
	}
}

func TestSharedRemembersTheError(t *testing.T) {
	type brokenConf struct {
		Port int `env:"PORT,required"`
	}
	oneenv.ResetShared[brokenConf]()
	t.Cleanup(oneenv.ResetShared[brokenConf])

	_, err := oneenv.Shared[brokenConf](oneenv.WithFiles(), oneenv.WithLookuper(oneenv.MapLookuper{}))
	if err == nil {
		t.Fatal("expected the required field to fail")
	}
	// A later call must not paper over the failure by re-reading a now-valid
	// environment: the process decided its configuration at startup.
	if _, err := oneenv.Shared[brokenConf](oneenv.WithFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{"PORT": "8080"})); err == nil {
		t.Fatal("the second call hid the first call's error")
	}
}

func TestSharedLiveIsOneHolder(t *testing.T) {
	oneenv.SetPollInterval(10 * time.Millisecond)

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("PORT=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oneenv.ResetShared[sharedConf]()
	t.Cleanup(oneenv.ResetShared[sharedConf])

	first, err := oneenv.SharedLive[sharedConf](ctx,
		oneenv.WithFiles(path), oneenv.WithLookuper(oneenv.MapLookuper{}))
	if err != nil {
		t.Fatalf("SharedLive: %v", err)
	}
	second, err := oneenv.SharedLive[sharedConf](context.Background())
	if err != nil {
		t.Fatalf("SharedLive: %v", err)
	}
	if first != second {
		t.Fatal("SharedLive created a second holder")
	}

	// The watcher registers on its own goroutine, so give it a moment before
	// changing the file it is about to watch.
	time.Sleep(30 * time.Millisecond)
	if err := os.WriteFile(path, []byte("PORT=7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if second.Get().Port == 7 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the shared holder never saw the change: %+v", second.Get())
}

// TestWatchStartsOnce checks that repeated Loads of the same target with
// WithWatch do not accumulate watchers: with two, each change would be decoded
// twice and the mutator would count double.
func TestWatchStartsOnce(t *testing.T) {
	oneenv.SetPollInterval(10 * time.Millisecond)

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("PORT=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var reloads int64
	var cfg sharedConf
	opts := []oneenv.Option{
		oneenv.WithFiles(path),
		oneenv.WithLookuper(oneenv.MapLookuper{}),
		oneenv.WithContext(ctx),
		oneenv.WithWatch(func(error) { atomicAdd(&reloads) }),
	}
	for i := 0; i < 3; i++ {
		if err := oneenv.Load(&cfg, opts...); err != nil {
			t.Fatalf("Load: %v", err)
		}
	}

	time.Sleep(30 * time.Millisecond)
	if err := os.WriteFile(path, []byte("PORT=7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Wait for the change to land, then give any duplicate watcher time to
	// report it too.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && atomicGet(&reloads) == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)

	if n := atomicGet(&reloads); n != 1 {
		t.Fatalf("one change produced %d reloads, want 1 (duplicate watchers)", n)
	}
	if got := strconv.Itoa(cfg.Port); got != "7" {
		t.Fatalf("Port = %s, want 7", got)
	}
}

func TestWithOnceReadsOnce(t *testing.T) {
	type onceConf struct {
		Port int `env:"PORT" default:"8080"`
	}
	oneenv.ResetShared[onceConf]()
	t.Cleanup(oneenv.ResetShared[onceConf])

	var decodes int64
	count := oneenv.WithMutator(func(_ context.Context, _, value string) (string, error) {
		atomicAdd(&decodes)
		return value, nil
	})

	var first onceConf
	err := oneenv.Load(&first, oneenv.WithOnce(), oneenv.WithFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{"PORT": "9090"}), count)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if first.Port != 9090 {
		t.Fatalf("Port = %d, want 9090", first.Port)
	}

	// A second target, different options: served from the first decode.
	var second onceConf
	err = oneenv.Load(&second, oneenv.WithOnce(), oneenv.WithFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{"PORT": "1"}), count)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if second.Port != 9090 {
		t.Fatalf("the second Load re-read the configuration: Port = %d", second.Port)
	}
	if n := atomicGet(&decodes); n != 1 {
		t.Fatalf("decoded %d times, want 1", n)
	}

	// Each caller owns its copy.
	second.Port = 1
	if first.Port != 9090 {
		t.Fatal("the targets share memory")
	}

	// Parse takes the same path, and Shared[T] shares the same decode.
	third, err := oneenv.Parse[onceConf](oneenv.WithOnce())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if third.Port != 9090 {
		t.Fatalf("Parse with WithOnce re-read the configuration: Port = %d", third.Port)
	}
	shared, err := oneenv.Shared[onceConf]()
	if err != nil {
		t.Fatalf("Shared: %v", err)
	}
	if shared.Port != 9090 {
		t.Fatalf("Shared saw a different value: Port = %d", shared.Port)
	}
	if n := atomicGet(&decodes); n != 1 {
		t.Fatalf("decoded %d times in total, want 1", n)
	}
}

func TestWithOnceRemembersTheError(t *testing.T) {
	type brokenOnce struct {
		Port int `env:"PORT,required"`
	}
	oneenv.ResetShared[brokenOnce]()
	t.Cleanup(oneenv.ResetShared[brokenOnce])

	var cfg brokenOnce
	if err := oneenv.Load(&cfg, oneenv.WithOnce(), oneenv.WithFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{})); err == nil {
		t.Fatal("expected the required field to fail")
	}
	if err := oneenv.Load(&cfg, oneenv.WithOnce(), oneenv.WithFiles(),
		oneenv.WithLookuper(oneenv.MapLookuper{"PORT": "8080"})); err == nil {
		t.Fatal("the second Load hid the first one's error")
	}
}

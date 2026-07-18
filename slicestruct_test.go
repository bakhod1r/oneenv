package oneenv

import "testing"

type server struct {
	Host string `env:"HOST"`
	Port int    `env:"PORT"`
}

func TestSliceOfStructs(t *testing.T) {
	type Config struct {
		Servers []server `env:"SERVER"`
	}
	var cfg Config
	err := Load(&cfg, WithLookuper(MapLookuper{
		"SERVER_0_HOST": "a", "SERVER_0_PORT": "1",
		"SERVER_1_HOST": "b", "SERVER_1_PORT": "2",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("len=%d, want 2", len(cfg.Servers))
	}
	if cfg.Servers[0].Host != "a" || cfg.Servers[1].Port != 2 {
		t.Errorf("got %+v", cfg.Servers)
	}
}

func TestSliceOfStructsEmpty(t *testing.T) {
	type Config struct {
		Servers []server `env:"SERVER"`
	}
	var cfg Config
	if err := Load(&cfg, WithLookuper(MapLookuper{})); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Servers) != 0 {
		t.Errorf("want empty, got %+v", cfg.Servers)
	}
}

func TestSliceOfStructsStopsAtGap(t *testing.T) {
	// Index 0 present, index 1 absent: decoding stops, ignoring index 2.
	type Config struct {
		Servers []server `env:"SERVER"`
	}
	var cfg Config
	err := Load(&cfg, WithLookuper(MapLookuper{
		"SERVER_0_HOST": "a",
		"SERVER_2_HOST": "c",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Servers) != 1 {
		t.Errorf("len=%d, want 1 (stop at gap)", len(cfg.Servers))
	}
}

func TestSliceOfStructsRoundTrip(t *testing.T) {
	type Config struct {
		Servers []server `env:"SERVER"`
	}
	src := Config{Servers: []server{{Host: "a", Port: 1}, {Host: "b", Port: 2}}}
	data, err := Marshal(src)
	if err != nil {
		t.Fatal(err)
	}
	var got Config
	if err := Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal %q: %v", data, err)
	}
	if len(got.Servers) != 2 || got.Servers[1].Host != "b" {
		t.Errorf("round trip: %+v (from %s)", got.Servers, data)
	}
}

// Separate module: keeps competitor dependencies out of the root module, so
// oneenv itself stays zero-dependency. Run: cd internal/bench && go test -bench .
module github.com/bakhod1r/oneenv/internal/bench

go 1.26.2

require (
	github.com/bakhod1r/oneenv v0.0.0
	github.com/caarlos0/env/v11 v11.2.2
	github.com/ilyakaznacheev/cleanenv v1.5.0
	github.com/joeshaw/envdecode v0.0.0-20200121155833-099f1fc765bd
	github.com/joho/godotenv v1.5.1
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/sethvargo/go-envconfig v1.4.2
	github.com/spf13/viper v1.21.0
)

require (
	github.com/BurntSushi/toml v1.2.1 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	olympos.io/encoding/edn v0.0.0-20201019073823-d3554ca0b0a3 // indirect
)

replace github.com/bakhod1r/oneenv => ../..

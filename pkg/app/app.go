package app

import (
	"fmt"

	"github.com/spf13/viper"
	"github.com/zinc-sig/webhook/pkg/api"
	"github.com/zinc-sig/webhook/pkg/auth"
	"github.com/zinc-sig/webhook/pkg/cache"
	"github.com/zinc-sig/webhook/pkg/logger"
	"github.com/zinc-sig/webhook/pkg/repository"
	"github.com/zinc-sig/webhook/pkg/telemetry"
	"github.com/zinc-sig/webhook/pkg/trigger"
	"github.com/zinc-sig/webhook/pkg/user"
	"go.uber.org/fx"
)

type Options struct {
	ConfigPath string
	Debug      bool
}

type Config struct {
	fx.Out     `yaml:"-"`
	Redis      *cache.Config      `mapstructure:"redis" yaml:"redis"`
	Repository *repository.Config `mapstructure:"repository" yaml:"repository"`
	Auth       *auth.Config       `mapstructure:"auth" yaml:"auth"`
	Telemetry  *telemetry.Config  `mapstructure:"telemetry" yaml:"telemetry"`
}

func Load(filename string) error {
	viper.SetConfigType("yml")
	if len(filename) > 0 {
		viper.SetConfigFile(filename)
	} else {
		viper.SetConfigName("config")
		viper.AddConfigPath("/etc/zinc.d/")
		viper.AddConfigPath(".")
	}
	err := viper.ReadInConfig()
	if err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}
	fmt.Println("==> Loaded configuration from", viper.ConfigFileUsed())
	return nil
}

func New(options Options) *fx.App {
	if err := Load(options.ConfigPath); err != nil {
		panic(fmt.Errorf("failed to load config: %v", err))
	}
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		panic(fmt.Errorf("failed to unmarshal config: %v", err))
	}
	m := []fx.Option{
		fx.Supply(cfg),
		auth.Module,
		repository.Module,
		cache.Module,
		trigger.Module,
		user.Module,
		api.Module,
		logger.Module(options.Debug),
		telemetry.Module,
	}

	fmt.Println("==> ZINC webhook configuration:")
	fmt.Println("==> ZINC webhook started! Log data will stream in below:")
	app := fx.New(m...)
	return app
}

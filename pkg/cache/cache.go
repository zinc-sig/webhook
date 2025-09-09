package cache

import (
	"context"

	"go.uber.org/fx"
)

type Config struct {
	DSN string `mapstructure:"dsn" yaml:"dsn"`
}

const (
	QueueKey = "grader:queues"
)

var Module = fx.Module(
	"cache",
	fx.Provide(
		NewService,
	),
	fx.Invoke(func(lifecycle fx.Lifecycle, cache Service) {
		lifecycle.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				go cache.Subscribe(context.Background())
				return nil
			},
		})
	}),
)

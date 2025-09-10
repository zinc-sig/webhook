package cache

import (
	"context"
	"encoding/json"
	"log/slog"

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
				cache.RegisterHandler("doneGrading", func(ctx context.Context, jobType, queue string, payload json.RawMessage) error {
					slog.Info("Processing job", "type", jobType, "queue", queue, "payload", string(payload))
					return nil
				})
				go cache.Subscribe(context.Background())
				return nil
			},
		})
	}),
)

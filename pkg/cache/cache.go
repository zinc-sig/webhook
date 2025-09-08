package cache

import (
	"context"
	"os"

	"github.com/go-redis/redis/v8"
	"go.uber.org/fx"
)

var Module = fx.Module(
	"cache",
	fx.Supply(
		&redis.Options{
			Addr:     os.Getenv("REDIS_URL"),
			Password: "", // no password set
			DB:       0,  // use default DB
		},
	),
	fx.Provide(
		NewService,
	),
	fx.Invoke(func(lifecycle fx.Lifecycle, cache Service) {
		lifecycle.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				return cache.Subscribe(ctx)
			},
		})
	}),
)

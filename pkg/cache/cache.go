package cache

import (
	"context"

	"go.uber.org/fx"
)

var Module = fx.Module(
	"cache",
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

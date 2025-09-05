package cache

import "go.uber.org/fx"

var Module = fx.Module(
	"cache",
	fx.Provide(
		NewService,
	),
)

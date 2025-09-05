package trigger

import (
	"github.com/zinc-sig/webhook/pkg/api"
	"go.uber.org/fx"
)

var Module = fx.Module(
	"trigger",
	fx.Provide(
		api.AsHandler(NewService),
	),
)

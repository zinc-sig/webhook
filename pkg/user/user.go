package user

import (
	"github.com/zinc-sig/webhook/pkg/api"
	"go.uber.org/fx"
)

var Module = fx.Module(
	"user",
	fx.Provide(
		api.AsHandler(NewService),
	),
)

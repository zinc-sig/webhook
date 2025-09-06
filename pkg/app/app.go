package app

import (
	"fmt"

	"github.com/zinc-sig/webhook/pkg/api"
	"github.com/zinc-sig/webhook/pkg/cache"
	"github.com/zinc-sig/webhook/pkg/graphql"
	"github.com/zinc-sig/webhook/pkg/trigger"
	"github.com/zinc-sig/webhook/pkg/user"
	"go.uber.org/fx"
)

type Options struct {
	Debug bool
}

func New(options Options) *fx.App {
	m := []fx.Option{
		graphql.Module,
		cache.Module,
		trigger.Module,
		user.Module,
		api.Module,
	}
	if options.Debug {
		m = append(m, fx.NopLogger)
	}

	fmt.Println("==> ZINC webhook configuration:")
	fmt.Println("==> ZINC webhook started! Log data will stream in below:")
	app := fx.New(m...)
	return app
}

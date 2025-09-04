package app

import (
	"fmt"
	"os"

	"github.com/go-redis/redis/v8"
	"github.com/zinc-sig/webhook/graphql"
	"github.com/zinc-sig/webhook/handlers"
	"github.com/zinc-sig/webhook/pkg/api"
	"go.uber.org/fx"
)

func New() *fx.App {
	graphqlClient := graphql.NewGraphQLClient()
	rdb := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_URL"),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	fmt.Println("==> ZINC webhook configuration:")
	fmt.Println("==> ZINC webhook started! Log data will stream in below:")
	app := fx.New(
		fx.Supply(
			&handlers.Handler{
				GraphQLClient: graphqlClient,
				RedisClient:   rdb,
			},
		),
		api.Module,
	)
	return app
}

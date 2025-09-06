package graphql

import (
	"os"

	"github.com/machinebox/graphql"
	"go.uber.org/fx"
)

func NewClient() *graphql.Client {
	client := graphql.NewClient(os.Getenv("HASURA_URL"))
	return client
}

var Module = fx.Module(
	"graphql",
	fx.Provide(
		NewClient,
	),
)

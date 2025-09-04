package graphql

import (
	"os"

	"github.com/machinebox/graphql"
)

func NewGraphQLClient() *graphql.Client {
	client := graphql.NewClient(os.Getenv("HASURA_URL"))
	return client
}

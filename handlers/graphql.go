package handlers

import (
	"context"

	"github.com/machinebox/graphql"
)

// GraphQLClient is an interface for a GraphQL client.
type GraphQLClient interface {
	Run(ctx context.Context, req *graphql.Request, resp interface{}) error
}
package repository

import (
	"context"

	"github.com/machinebox/graphql"
)

func (r *repository) UpdateReportEntry(ctx context.Context, report map[string]interface{}) error {
	req := r.WithAdminSecret(graphql.NewRequest(addReportArtifacts))
	for k, v := range report {
		req.Var(k, v)
	}

	var resp struct{}
	return r.client.Run(ctx, req, &resp)
}

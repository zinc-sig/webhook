package repository

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/machinebox/graphql"
)

type Assignment struct {
	AssignmentConfig struct {
		StopCollectionAt *string      `json:"stopCollectionAt"`
		Submissions      []Submission `json:"submissions"`
	} `json:"assignmentConfig"`
}

type Submission struct {
	ID            int       `json:"id"`
	ExtractedPath string    `json:"extracted_path"`
	CreatedAt     time.Time `json:"created_at"`
}

func (r *repository) UpdateExtractedSubmissionEntry(ctx context.Context, id int, extractedPath, failReason string) error {
	req := graphql.NewRequest(updateDecompressionResultForSubmission)
	req.Var("id", id)
	if extractedPath != "" {
		req.Var("extractedPath", extractedPath)
	} else {
		req.Var("extractedPath", nil)
	}
	if failReason != "" {
		req.Var("failReason", failReason)
	} else {
		req.Var("failReason", nil)
	}

	var resp struct{}
	return r.client.Run(ctx, req, &resp)
}

func (r *repository) GetGradingSubmissions(ctx context.Context, assignmentConfigID int) (*Assignment, error) {
	graphqlReq := graphql.NewRequest(getGradingSubmissions)
	graphqlReq.Var("assignmentConfigId", assignmentConfigID)

	var graphqlResp Assignment
	if err := r.client.Run(ctx, graphqlReq, &graphqlResp); err != nil {
		slog.Warn("failed to get grading submissions", "assignmentConfigID", assignmentConfigID, "error", err)
		return nil, fmt.Errorf("failed to get grading submissions: %s", err.Error())
	}
	return &graphqlResp, nil
}

func (r *repository) GetLatestOrSelectedSubmissions(ctx context.Context, assignmentConfigID int, selectedSubmissionIDs []int) ([]Submission, error) {
	query := getSelectedSubmissions
	variables := map[string]interface{}{
		"submissions": selectedSubmissionIDs,
	}
	if len(selectedSubmissionIDs) == 0 {
		query = getLatestSubmissionsForAssignmentConfig
		variables = map[string]interface{}{
			"assignmentConfigId": assignmentConfigID,
		}
	}

	graphqlReq := graphql.NewRequest(query)
	for key, value := range variables {
		graphqlReq.Var(key, value)
	}

	var graphqlResp struct {
		Submissions []Submission `json:"submissions"`
	}

	if len(selectedSubmissionIDs) == 0 {
		var resp Assignment
		if err := r.client.Run(ctx, graphqlReq, &resp); err != nil {
			slog.Warn("Failed to get submissions", "error", err)
			return nil, fmt.Errorf("failed to get submissions: %s", err.Error())
		}
		graphqlResp.Submissions = resp.AssignmentConfig.Submissions
	} else {
		if err := r.client.Run(ctx, graphqlReq, &graphqlResp); err != nil {
			slog.Warn("Failed to get submissions", "error", err)
			return nil, fmt.Errorf("failed to get submissions: %s", err.Error())
		}
	}
	return graphqlResp.Submissions, nil
}

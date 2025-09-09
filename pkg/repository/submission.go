package repository

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

func (r *repository) ExtractZip(submissionID int, storedName string) error {

	file := fmt.Sprintf("%s/%s", r.sharedMountPath, storedName)
	extractToPath := fmt.Sprintf("%s/extracted/%d", r.sharedMountPath, submissionID)
	temporaryResolvePath := fmt.Sprintf("/tmp/%d", submissionID)

	if err := os.MkdirAll(temporaryResolvePath, os.ModePerm); err != nil {
		slog.Warn("Failed to create temporary directory", "error", err)
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}

	reader, err := zip.OpenReader(file)
	if err != nil {
		slog.Warn("Failed to open zip file", "error", err)
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer reader.Close()

	for _, f := range reader.File {
		fpath := filepath.Join(temporaryResolvePath, f.Name)

		if !strings.HasPrefix(fpath, filepath.Clean(temporaryResolvePath)+string(os.PathSeparator)) {
			slog.Warn("Illegal file path in zip", "filePath", fpath)
			return fmt.Errorf("%s: illegal file path", fpath)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			slog.Warn("Failed to create directory for file in zip", "error", err)
			return fmt.Errorf("failed to create directory for file: %w", err)
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			slog.Warn("Failed to open output file", "filePath", fpath, "error", err)
			return fmt.Errorf("failed to open output file: %w", err)
		}

		rc, err := f.Open()
		if err != nil {
			slog.Warn("Failed to open file in zip", "filePath", fpath, "error", err)
			return fmt.Errorf("failed to open file in zip: %w", err)
		}

		_, err = io.Copy(outFile, rc)

		outFile.Close()
		rc.Close()

		if err != nil {
			slog.Warn("Failed to copy file contents", "filePath", fpath, "error", err)
			return fmt.Errorf("failed to copy file contents: %w", err)
		}
	}

	files, err := os.ReadDir(temporaryResolvePath)
	if err != nil {
		slog.Warn("Failed to read directory", "error", err)
		return fmt.Errorf("failed to read directory: %w", err)
	}

	if len(files) >= 1 {
		sourcePath := temporaryResolvePath
		if len(files) == 1 && files[0].IsDir() {
			sourcePath = fmt.Sprintf("%s/%s", temporaryResolvePath, files[0].Name())
		}
		if err := os.Rename(sourcePath, extractToPath); err != nil {
			slog.Warn("Failed to rename directory", "error", err)
			return fmt.Errorf("failed to rename directory: %w", err)
		}
	} else {
		return fmt.Errorf("empty directory")
	}

	return nil
}

package repository

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/machinebox/graphql"
)

type TimeWithoutZone string

func (t TimeWithoutZone) Time() time.Time {
	parsedTime, _ := time.Parse("2006-01-02T15:04:05", string(t))
	return parsedTime
}

type Assignment struct {
	AssignmentConfig AssignmentConfig `json:"assignmentConfig"`
}

type AssignmentConfig struct {
	DueAt            *TimeWithoutZone `json:"dueAt"`
	StopCollectionAt *string          `json:"stopCollectionAt"`
	Submissions      []Submission     `json:"submissions"`
}

type Submission struct {
	ID            int             `json:"id"`
	ExtractedPath string          `json:"extracted_path"`
	CreatedAt     TimeWithoutZone `json:"created_at"`
}

func (r *repository) CreateSubmission(ctx context.Context, userID int, assignmentConfigID int, storedName, uploadName string, fileSize int64, checksum string, cookie *http.Cookie) (int, error) {
	req := r.WithCookie(graphql.NewRequest(createSubmission), cookie)
	req.Var("submission", map[string]any{
		"stored_name":          storedName,
		"upload_name":          uploadName,
		"assignment_config_id": assignmentConfigID,
		"size":                 fileSize,
		"checksum":             checksum,
		"user_id":              userID,
	})

	var resp struct {
		ID int `json:"id"`
	}

	return resp.ID, r.client.Run(ctx, req, &resp)
}

func (r *repository) UpdateExtractedSubmissionEntry(ctx context.Context, id int, extractedPath, failReason string) error {
	req := r.WithAdminSecret(graphql.NewRequest(updateDecompressionResultForSubmission))
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

func (r *repository) GetGradingPolicy(ctx context.Context, assignmentConfigID int, userID int) (bool, bool, error) {
	req := r.WithAdminSecret(graphql.NewRequest(getGradingPolicy))
	req.Var("id", assignmentConfigID)
	req.Var("userId", userID)

	var resp struct {
		AssignmentConfig struct {
			GradeImmediately bool `json:"gradeImmediately"`
			Assignment       struct {
				Course struct {
					Users []struct {
						Permission int `json:"permission"`
					} `json:"users"`
				} `json:"course"`
			} `json:"assignment"`
		} `json:"assignmentConfig"`
	}
	if err := r.client.Run(ctx, req, &resp); err != nil {
		slog.Warn("failed to get grading policy", "assignmentConfigID", assignmentConfigID, "userID", userID, "error", err)
		return false, false, fmt.Errorf("failed to get grading policy: %s", err.Error())
	}

	return resp.AssignmentConfig.GradeImmediately, resp.AssignmentConfig.Assignment.Course.Users[0].Permission > 1, nil
}

func (r *repository) GetGradingSubmissions(ctx context.Context, assignmentConfigID int) (*Assignment, error) {
	graphqlReq := r.WithAdminSecret(graphql.NewRequest(getGradingSubmissions))
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
	variables := map[string]any{
		"submissions": selectedSubmissionIDs,
	}
	if len(selectedSubmissionIDs) == 0 {
		query = getLatestSubmissionsForAssignmentConfig
		variables = map[string]any{
			"assignmentConfigId": assignmentConfigID,
		}
	}

	graphqlReq := r.WithAdminSecret(graphql.NewRequest(query))
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
		if err := moveDirectory(sourcePath, extractToPath); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("empty directory")
	}

	return nil
}

func moveDirectory(src, dest string) error {
	// First, copy the directory
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create a relative path to maintain the structure
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dest, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		// Copy the file
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		destFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer destFile.Close()

		_, err = io.Copy(destFile, srcFile)
		return err
	})

	if err != nil {
		return fmt.Errorf("failed to copy directory: %w", err)
	}

	// If copy is successful, remove the original directory
	return os.RemoveAll(src)
}

func (r *repository) GetSubmissionGrades(ctx context.Context, assignmentConfigID int, response any) error {
	req := r.WithAdminSecret(graphql.NewRequest(getSubmissionGrades))
	req.Var("id", assignmentConfigID)

	if err := r.client.Run(ctx, req, response); err != nil {
		slog.Warn("Failed to get submission grades", "assignmentConfigID", assignmentConfigID, "error", err)
		return fmt.Errorf("failed to get submission grades: %s", err.Error())
	}

	return nil
}

func (r *repository) GetSubmissionByID(ctx context.Context, submissionID int, response any) error {
	req := r.WithAdminSecret(graphql.NewRequest(getSubmissionByID))
	req.Var("id", submissionID)

	if err := r.client.Run(ctx, req, response); err != nil {
		slog.Warn("Failed to get submission by ID", "submissionID", submissionID, "error", err)
		return fmt.Errorf("failed to get submission by ID: %s", err.Error())
	}

	return nil
}

func (r *repository) GetSubmissionFilePath(storedName string) string {
	return fmt.Sprintf("%s/%s", r.sharedMountPath, storedName)
}

func (r *repository) GetAllSubmissionsForAssignmentConfig(ctx context.Context, assignmentConfigID int, response any) error {
	req := r.WithAdminSecret(graphql.NewRequest(getAllSubmissionsForAssignmentConfig))
	req.Var("assignmentConfigId", assignmentConfigID)

	if err := r.client.Run(ctx, req, response); err != nil {
		slog.Warn("Failed to get all submissions for assignment config", "assignmentConfigID", assignmentConfigID, "error", err)
		return fmt.Errorf("failed to get all submissions for assignment config: %s", err.Error())
	}

	return nil
}

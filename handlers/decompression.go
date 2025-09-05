package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/machinebox/graphql"
)

func (h *Handler) DecompressSubmission(c echo.Context) error {
	var req DecompressionRequest
	if err := c.Bind(&req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid request body")
	}

	submission := req.Event.Data.New

	if !strings.HasSuffix(submission.UploadName, ".zip") {
		return h.updateExtractedSubmissionEntry(submission.ID, "", "Unsupported archive format")
	}

	if err := h.extractZip(submission.ID, submission.StoredName); err != nil {
		return h.updateExtractedSubmissionEntry(submission.ID, "", err.Error())
	}

	return h.updateExtractedSubmissionEntry(submission.ID, fmt.Sprintf("extracted/%d", submission.ID), "")
}

var ExecCommand = exec.Command

func (h *Handler) extractZip(submissionID int, storedName string) error {
	mountPath := os.Getenv("SHARED_MOUNT_PATH")
	if mountPath == "" {
		mountPath = "/home/system/workspace"
	}

	file := fmt.Sprintf("%s/%s", mountPath, storedName)
	extractToPath := fmt.Sprintf("%s/extracted/%d", mountPath, submissionID)
	temporaryResolvePath := fmt.Sprintf("/tmp/%d", submissionID)

	if err := os.MkdirAll(temporaryResolvePath, os.ModePerm); err != nil {
		return err
	}

	cmd := ExecCommand("unzip", file, "-d", temporaryResolvePath)
	if err := cmd.Run(); err != nil {
		return err
	}

	files, err := os.ReadDir(temporaryResolvePath)
	if err != nil {
		return err
	}

	if len(files) >= 1 {
		sourcePath := temporaryResolvePath
		if len(files) == 1 && files[0].IsDir() {
			sourcePath = fmt.Sprintf("%s/%s", temporaryResolvePath, files[0].Name())
		}
		if err := os.Rename(sourcePath, extractToPath); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("empty directory")
	}

	return nil
}

func (h *Handler) updateExtractedSubmissionEntry(id int, extractedPath, failReason string) error {
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
	return h.GraphQLClient.Run(context.Background(), req, &resp)
}

package trigger

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/zinc-sig/webhook/pkg/api"
)

func SyncEnrollment(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		if err := s.SyncEnrollment(c.Request().Context()); err != nil {
			return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to sync enrollment", Message: err.Error()})
		}
		return c.JSON(http.StatusOK, api.Response{Status: "ok"})
	}
}

func DecompressSubmission(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req RowTriggerPayload
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body", Message: err.Error()})
		}
		if err := s.DecompressSubmission(c.Request().Context(), req.Event.Data.New); err != nil {
			return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to decompress submission", Message: err.Error()})
		}
		return c.JSON(http.StatusOK, api.Response{Status: "ok"})
	}
}

func PostGradingProcessing(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req RowTriggerPayload
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body", Message: err.Error()})
		}
		if err := s.PostGradingProcessing(c.Request().Context(), req.Event.Data.New); err != nil {
			return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to process post grading", Message: err.Error()})
		}
		return c.JSON(http.StatusOK, api.Response{Status: "ok"})
	}
}

func ScheduleGrading(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req RowTriggerPayload
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body", Message: err.Error()})
		}
		if err := s.ScheduleGrading(c.Request().Context(), &req.Event); err != nil {
			return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to schedule grading", Message: err.Error()})
		}
		return c.JSON(http.StatusOK, api.Response{Status: "ok"})
	}
}

func ManualGradingTask(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req ManualGradingTaskRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body", Message: err.Error()})
		}
		assignmentConfigId, err := strconv.Atoi(c.Param("assignmentConfigId"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid assignmentConfigId", Message: err.Error()})
		}
		if err := s.ManualGradingTask(c.Request().Context(), assignmentConfigId, &req); err != nil {
			return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to process manual grading task", Message: err.Error()})
		}
		return c.JSON(http.StatusOK, api.Response{Status: "ok"})
	}
}

func GradingTask(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req GradingTaskRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body", Message: err.Error()})
		}
		if err := s.GradingTask(c.Request().Context(), &req); err != nil {
			return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to process grading task", Message: err.Error()})
		}
		return c.JSON(http.StatusOK, api.Response{Status: "ok"})
	}
}

func ValidateConfig(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			ConfigYAML string `json:"yaml"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body", Message: err.Error()})
		}
		resp, err := s.cache.ValidateConfig(c.Request().Context(), req.ConfigYAML)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to validate config", Message: err.Error()})
		}
		return c.JSON(http.StatusOK, resp)
	}
}

func UpdateGraderQueues(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			Queues []string `json:"queues"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body", Message: err.Error()})
		}
		if err := s.UpdateGraderQueues(c.Request().Context(), req.Queues); err != nil {
			return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to update grader queues", Message: err.Error()})
		}
		return c.JSON(http.StatusOK, api.Response{Status: "ok"})
	}
}

func DownloadGrades(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Get query parameters
		assignmentConfigIDStr := c.QueryParam("assignmentConfigId")
		viewingTaskAssignedGroups := c.QueryParam("viewingTaskAssignedGroups")

		if assignmentConfigIDStr == "" {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "missing assignmentConfigId query parameter", Message: "assignmentConfigId is required"})
		}

		assignmentConfigID, err := strconv.Atoi(assignmentConfigIDStr)
		if err != nil {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid assignmentConfigId", Message: err.Error()})
		}

		// Generate Excel file
		excelFile, err := s.GenerateGradesExcel(c.Request().Context(), assignmentConfigID, viewingTaskAssignedGroups)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to generate grades Excel", Message: err.Error()})
		}
		defer excelFile.Close()

		// Set response headers
		c.Response().Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		c.Response().Header().Set("Content-Disposition", "attachment; filename=Report.xlsx")

		// Write Excel file to response
		if err := excelFile.Write(c.Response().Writer); err != nil {
			return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to write Excel file", Message: err.Error()})
		}

		return nil
	}
}

func DownloadSubmissions(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Get query parameter
		assignmentConfigIDStr := c.QueryParam("assignmentConfigId")
		if assignmentConfigIDStr == "" {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "missing assignmentConfigId query parameter", Message: "assignmentConfigId is required"})
		}

		assignmentConfigID, err := strconv.Atoi(assignmentConfigIDStr)
		if err != nil {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid assignmentConfigId", Message: err.Error()})
		}

		// Generate zip archive
		zipBuffer, filename, err := s.GenerateSubmissionsZip(c.Request().Context(), assignmentConfigID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to generate submissions zip", Message: err.Error()})
		}

		// Set Content-Disposition header for filename
		c.Response().Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

		// Write zip to response atomically
		return c.Blob(http.StatusOK, "application/octet-stream", zipBuffer.Bytes())
	}
}

func DownloadSubmission(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Get submission ID from path parameter
		submissionIDStr := c.Param("id")
		if submissionIDStr == "" {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "missing submission id", Message: "submission id is required"})
		}

		submissionID, err := strconv.Atoi(submissionIDStr)
		if err != nil {
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "invalid submission id", Message: err.Error()})
		}

		// Get submission metadata
		submission, err := s.GetSubmissionForDownload(c.Request().Context(), submissionID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "failed to get submission", Message: err.Error()})
		}

		// Get file path
		filePath := s.repository.GetSubmissionFilePath(submission.Submission.StoredName)

		// Generate filename with timestamp prefix
		timestamp := submission.Submission.CreatedAt.Time().UnixMilli()
		filename := fmt.Sprintf("%d_%s", timestamp, submission.Submission.UploadName)

		// Set response headers
		c.Response().Header().Set("Content-Type", "application/octet-stream")
		c.Response().Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

		// Stream the file to response
		return c.File(filePath)
	}
}

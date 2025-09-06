package trigger

import (
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

package trigger

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func SyncEnrollment(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Implementation of SyncEnrollment handler
		if err := s.SyncEnrollment(c.Request().Context()); err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}
}

func DecompressSubmission(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req RowTriggerPayload
		if err := c.Bind(&req); err != nil {
			return c.String(http.StatusBadRequest, "Invalid request body")
		}
		if err := s.DecompressSubmission(c.Request().Context(), req.Event.Data.New); err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}
}

func PostGradingProcessing(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Implementation of PostGradingProcessing handler
		return c.String(http.StatusOK, "PostGradingProcessing called")
	}
}

func ScheduleGrading(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Implementation of ScheduleGrading handler
		return c.String(http.StatusOK, "ScheduleGrading called")
	}
}

func ManualGradingTask(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Implementation of ManualGradingTask handler
		return c.String(http.StatusOK, "ManualGradingTask called")
	}
}

func GradingTask(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req GradingTaskRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		}
		if err := s.GradingTask(c.Request().Context(), &req); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}
}

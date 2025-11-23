package user

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/zinc-sig/webhook/pkg/api"
)

func Identity(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req IdentityRequest
		if err := c.Bind(&req); err != nil {
			slog.Warn("failed to bind request", "error", err)
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		}
		user, err := s.ValidateSession(c.Request().Context(), req.Headers.Cookie)
		if err != nil {
			slog.Warn("failed to validate session", "error", err)
			return c.JSON(http.StatusUnauthorized, api.ErrorResponse{Error: "Unauthorized", Message: err.Error()})
		}
		allowedCourses := ""
		if !user.IsAdmin {
			var courseIDs []string
			for _, course := range user.Courses {
				courseIDs = append(courseIDs, fmt.Sprintf("%d", course.CourseID))
			}
			allowedCourses = fmt.Sprintf("{%s}", strings.Join(courseIDs, ","))
		}

		role := "user"
		if user.IsAdmin {
			role = "admin"
		}
		resp := IdentityResponse{
			XHasuraUserId:         user.Itsc,
			XHasuraRole:           role,
			XHasuraAllowedCourses: allowedCourses,
			XHasuraRequestedAt:    time.Now().Format(time.RFC3339),
		}
		return c.JSON(http.StatusOK, resp)
	}
}

func NewSession(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			ExpiresAt time.Time       `json:"expires_at"`
			TokenSet  json.RawMessage `json:"token_set"`
			Name      string          `json:"name"`
			Itsc      string          `json:"itsc"` 
		}
		if err := c.Bind(&req); err != nil {
			slog.Warn("failed to bind request", "error", err)
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		}
		sessionId, userId, err := s.CreateSession(c.Request().Context(), req.TokenSet, req.ExpiresAt, req.Name, req.Itsc)
		if err != nil {
			slog.Warn("failed to create session", "error", err)
			return c.JSON(http.StatusInternalServerError, api.ErrorResponse{Error: "Failed to create session", Message: err.Error()})
		}
		return c.JSON(http.StatusOK, api.Response{Status: "ok", Data: map[string]string{"session_id": sessionId, "user_id": userId}})
	}
}

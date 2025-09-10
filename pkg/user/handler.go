package user

import (
	"fmt"
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
			s.logger.Warnw("failed to bind request", "context", c.Request().Context(), "error", err)
			return c.JSON(http.StatusBadRequest, api.ErrorResponse{Error: "Invalid request body", Message: err.Error()})
		}
		user, err := s.ValidateSession(c.Request().Context(), req.Headers.Cookie)
		if err != nil {
			s.logger.Warnw("failed to validate session", "context", c.Request().Context(), "error", err)
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

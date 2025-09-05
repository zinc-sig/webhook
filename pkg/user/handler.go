package user

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
)

func Identity(s *service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req IdentityRequest
		if err := c.Bind(&req); err != nil {
			return c.String(http.StatusBadRequest, "Invalid request body")
		}

		appSession, err := getCookie(req.Headers.Cookie, "appSession")
		if err != nil {
			return c.String(http.StatusUnauthorized, "Unauthorized")
		}

		var cookieData CookieData
		if err := json.Unmarshal([]byte(cookie), &cookieData); err != nil {
			return c.String(http.StatusInternalServerError, "Failed to parse cookie")
		}

		token, err := verifyToken(cookieData.Data.IdToken)
		if err != nil {
			return c.String(http.StatusUnauthorized, "Invalid token")
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || !token.Valid {
			return c.String(http.StatusUnauthorized, "Invalid token")
		}

		email, ok := claims["email"].(string)
		if !ok {
			return c.String(http.StatusUnauthorized, "Invalid token")
		}
		itsc := strings.Split(email, "@")[0]
		name, _ := claims["name"].(string)

		user, err := h.getUser(itsc, name)
		if err != nil {
			return c.String(http.StatusInternalServerError, "Failed to get user")
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
			XHasuraUserId:         itsc,
			XHasuraRole:           role,
			XHasuraAllowedCourses: allowedCourses,
			XHasuraRequestedAt:    time.Now().Format(time.RFC3339),
		}
	}
}

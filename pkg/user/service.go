package user

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/zinc-sig/webhook/pkg/cache"
	"go.uber.org/fx"
)

type ServiceParams struct {
	fx.In
	cache cache.Service
}

type service struct {
	cache cache.Service
}

func NewService(p ServiceParams) *service {
	return &service{
		cache: p.cache,
	}
}

func (s *service) RegisterRoutes(e *echo.Echo) {
	e.POST("/identity", Identity(s))
}

func (s *service) ValidateSession(ctx context.Context, sessionId string) (bool, error) {
	hmac := hmac.New(sha1.New, []byte(os.Getenv("SESSION_SECRET")))
	hmac.Write([]byte(sessionId))
	sid := base64.StdEncoding.EncodeToString(hmac.Sum(nil))

	cookie, err := s.cache.Read(ctx, sid)
	if err != nil {
		return c.String(http.StatusUnauthorized, "Could not find request session with auth credentials")
	}
}

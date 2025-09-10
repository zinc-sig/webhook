package user

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/zinc-sig/webhook/pkg/auth"
	"github.com/zinc-sig/webhook/pkg/cache"
	"github.com/zinc-sig/webhook/pkg/repository"
	"go.uber.org/fx"
)

type ServiceParams struct {
	fx.In
	Config      *auth.Config
	Cache       cache.Service
	Repository  repository.Repository
	JWTVerifier auth.JWTVerifier
}

type service struct {
	cache         cache.Service
	repository    repository.Repository
	sessionSecret string
	jwtVerifier   auth.JWTVerifier
}

func NewService(p ServiceParams) *service {

	return &service{
		cache:         p.Cache,
		repository:    p.Repository,
		sessionSecret: p.Config.SessionSecret,
		jwtVerifier:   p.JWTVerifier,
	}
}

func (s *service) RegisterRoutes(e *echo.Echo) {
	e.POST("/identity", Identity(s))
}

func getCookie(cookieHeader, cookieName string) (string, error) {
	header := http.Header{}
	header.Add("Cookie", cookieHeader)
	request := http.Request{Header: header}
	cookie, err := request.Cookie(cookieName)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

func (s *service) ValidateSession(ctx context.Context, cookieString string) (*repository.User, error) {
	sessionId, err := getCookie(cookieString, "appSession")
	if err != nil {
		slog.Warn("could not get session cookie", "error", err)
		return nil, fmt.Errorf("could not find request session with auth credentials: %w", err)
	}
	hmac := hmac.New(sha1.New, []byte(s.sessionSecret))
	hmac.Write([]byte(sessionId))
	key := base64.StdEncoding.EncodeToString(hmac.Sum(nil))
	var tokenSet TokenSet
	cookie, err := s.cache.Read(ctx, key)
	if err != nil {
		slog.Warn("could not read session from cache", "error", err)
		return nil, fmt.Errorf("could not find request session with auth credentials: %w", err)
	}
	if err := json.Unmarshal([]byte(cookie), &tokenSet); err != nil {
		return nil, fmt.Errorf("could not parse session token: %w", err)
	}

	token, err := s.jwtVerifier.VerifyToken(ctx, tokenSet.Data.IdToken)
	if err != nil {
		slog.Warn("failed to verify token", "error", err)
		return nil, fmt.Errorf("failed to verify token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		slog.Warn("invalid token claims", "error", err)
		return nil, errors.New("invalid token claims")
	}

	email, ok := claims["email"].(string)
	if !ok {
		slog.Warn("email claim not found in token", "error", err)
		return nil, errors.New("email claim not found in token")
	}
	itsc := strings.Split(email, "@")[0]
	name, _ := claims["name"].(string)

	user, err := s.repository.GetUser(ctx, itsc, name)
	if err != nil {
		slog.Warn("failed to get user from database", "error", err)
		return nil, fmt.Errorf("failed to get user from database: %w", err)
	}

	slog.Info("session validated successfully", "itsc", itsc, "userID", user.ID)
	return user, nil
}

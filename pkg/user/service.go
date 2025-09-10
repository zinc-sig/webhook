package user

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/zinc-sig/webhook/pkg/auth"
	"github.com/zinc-sig/webhook/pkg/cache"
	"github.com/zinc-sig/webhook/pkg/repository"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type ServiceParams struct {
	fx.In
	Config      *auth.Config
	Cache       cache.Service
	Repository  repository.Repository
	JWTVerifier auth.JWTVerifier
	Logger      *zap.SugaredLogger
}

type service struct {
	cache         cache.Service
	repository    repository.Repository
	sessionSecret string
	jwtVerifier   auth.JWTVerifier
	tracer        trace.Tracer
	logger        *zap.SugaredLogger
}

func NewService(p ServiceParams) *service {

	return &service{
		cache:         p.Cache,
		repository:    p.Repository,
		sessionSecret: p.Config.SessionSecret,
		jwtVerifier:   p.JWTVerifier,
		tracer:        otel.Tracer("webhook.user"),
		logger:        p.Logger,
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
	ctx, span := s.tracer.Start(ctx, "user.ValidateSession")
	defer span.End()

	sessionId, err := getCookie(cookieString, "appSession")
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "could not get session cookie")
		s.logger.Warnw("could not get session cookie", "context", ctx, "error", err)
		return nil, fmt.Errorf("could not find request session with auth credentials: %w", err)
	}
	hmac := hmac.New(sha1.New, []byte(s.sessionSecret))
	hmac.Write([]byte(sessionId))
	key := base64.StdEncoding.EncodeToString(hmac.Sum(nil))
	span.SetAttributes(attribute.String("session_key", key))

	var tokenSet TokenSet
	cookie, err := s.cache.Read(ctx, key)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "could not read session from cache")
		s.logger.Warnw("could not read session from cache", "context", ctx, "error", err)
		return nil, fmt.Errorf("could not find request session with auth credentials: %w", err)
	}

	if err := json.Unmarshal([]byte(cookie), &tokenSet); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "could not parse session token")
		return nil, fmt.Errorf("could not parse session token: %w", err)
	}

	token, err := s.jwtVerifier.VerifyToken(ctx, tokenSet.Data.IdToken)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to verify token")
		s.logger.Warnw("failed to verify token", "context", ctx, "error", err)
		return nil, fmt.Errorf("failed to verify token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		span.SetStatus(codes.Error, "invalid token claims")
		s.logger.Warnw("invalid token claims", "context", ctx, "error", err)
		return nil, errors.New("invalid token claims")
	}

	email, ok := claims["email"].(string)
	if !ok {
		span.SetStatus(codes.Error, "email claim not found in token")
		s.logger.Warnw("email claim not found in token", "context", ctx, "error", err)
		return nil, errors.New("email claim not found in token")
	}
	itsc := strings.Split(email, "@")[0]
	name, _ := claims["name"].(string)
	span.SetAttributes(
		attribute.String("user.itsc", itsc),
		attribute.String("user.email", email),
	)

	user, err := s.repository.GetUser(ctx, itsc, name)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get user from database")
		s.logger.Warnw("failed to get user from database", "context", ctx, "error", err)
		return nil, fmt.Errorf("failed to get user from database: %w", err)
	}

	s.logger.Infow("session validated successfully", "context", ctx, "itsc", itsc, "userID", user.ID)
	span.SetAttributes(
		attribute.Int("user.id", user.ID),
		attribute.String("status", "success"),
	)
	return user, nil
}

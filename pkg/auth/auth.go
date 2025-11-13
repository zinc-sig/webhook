package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/golang-jwt/jwt/v4"
	"github.com/lestrrat-go/jwx/jwk"
	"go.uber.org/fx"
)

type Config struct {
	SessionSecret string `mapstructure:"session_secret" yaml:"session_secret"`
	JWKSURL       string `mapstructure:"jwks_url" yaml:"jwks_url"`
}

type JWTVerifier interface {
	VerifyToken(ctx context.Context, tokenString string) (*jwt.Token, error)
}

type Params struct {
	fx.In
	Config *Config
}

type verifier struct {
	jwkEndpoint string
}

func NewVerifier(p Params) JWTVerifier {
	return &verifier{
		jwkEndpoint: p.Config.JWKSURL,
	}
}

func (v *verifier) VerifyToken(ctx context.Context, tokenString string) (*jwt.Token, error) {
	keySet, err := jwk.Fetch(ctx, v.jwkEndpoint)
	if err != nil {
		slog.Warn("Failed to fetch JWK", "error", err)
		return nil, fmt.Errorf("failed to fetch JWK: %w", err)
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok {
			slog.Warn("kid header not found in token")
			return nil, errors.New("kid header not found")
		}

		keys, ok := keySet.LookupKeyID(kid)
		if !ok {
			slog.Warn("No keys found for given kid", "kid", kid)
			return nil, errors.New("key not found")
		}

		var publicKey any
		if err := keys.Raw(&publicKey); err != nil {
			slog.Warn("Failed to get raw key", "error", err)
			return nil, fmt.Errorf("failed to get raw key: %w", err)
		}

		return publicKey, nil
	})

	return token, err
}

func RandomBytes(size int) ([]byte, error) {
	b := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, fmt.Errorf("failed to read random bytes: %v", err)
	}
	return b, nil
}

var Module = fx.Module(
	"auth",
	fx.Provide(
		NewVerifier,
	),
)

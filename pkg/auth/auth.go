package auth

import (
	"context"
	"errors"

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
		return nil, err
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("kid header not found")
		}

		keys, ok := keySet.LookupKeyID(kid)
		if !ok {
			return nil, errors.New("key not found")
		}

		var publicKey interface{}
		if err := keys.Raw(&publicKey); err != nil {
			return nil, err
		}

		return publicKey, nil
	})

	return token, err
}

var Module = fx.Module(
	"auth",
	fx.Provide(
		NewVerifier,
	),
)

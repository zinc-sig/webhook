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
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/machinebox/graphql"
	"github.com/zinc-sig/webhook/pkg/cache"
	"go.uber.org/fx"
)

type ServiceParams struct {
	fx.In
	Cache         cache.Service
	GraphQLClient *graphql.Client
}

type service struct {
	cache   cache.Service
	graphql *graphql.Client
}

func NewService(p ServiceParams) *service {
	return &service{
		cache:   p.Cache,
		graphql: p.GraphQLClient,
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

func (s *service) getUser(ctx context.Context, itsc, name string) (*User, error) {
	req := graphql.NewRequest(GetUserByITSC)
	req.Var("itsc", itsc)

	var resp struct {
		Users []User `json:"users"`
	}

	if err := s.graphql.Run(ctx, req, &resp); err != nil {
		return nil, err
	}

	if len(resp.Users) > 0 {
		return &resp.Users[0], nil
	}

	// Create user if not found
	req = graphql.NewRequest(CreateUser)
	req.Var("itsc", itsc)
	req.Var("name", name)

	var createResp struct {
		InsertUsersOne struct {
			ID int `json:"id"`
		} `json:"insert_users_one"`
	}

	if err := s.graphql.Run(ctx, req, &createResp); err != nil {
		return nil, err
	}

	return &User{ID: createResp.InsertUsersOne.ID, Name: name}, nil
}

func verifyToken(ctx context.Context, tokenString string) (*jwt.Token, error) {
	keySet, err := jwk.Fetch(context.Background(), "https://login.microsoftonline.com/common/discovery/v2.0/keys")
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

func (s *service) ValidateSession(ctx context.Context, cookieString string) (*User, error) {
	sessionId, err := getCookie(cookieString, "appSession")
	if err != nil {
		return nil, fmt.Errorf("could not find request session with auth credentials: %w", err)
	}
	hmac := hmac.New(sha1.New, []byte(os.Getenv("SESSION_SECRET")))
	hmac.Write([]byte(sessionId))
	key := base64.StdEncoding.EncodeToString(hmac.Sum(nil))
	var tokenSet TokenSet
	cookie, err := s.cache.Read(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("could not find request session with auth credentials: %w", err)
	}
	if err := json.Unmarshal([]byte(cookie), &tokenSet); err != nil {
		return nil, fmt.Errorf("could not parse session token: %w", err)
	}

	token, err := verifyToken(ctx, tokenSet.Data.IdToken)
	if err != nil {
		return nil, fmt.Errorf("cailed to verify token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	email, ok := claims["email"].(string)
	if !ok {
		return nil, errors.New("email claim not found in token")
	}
	itsc := strings.Split(email, "@")[0]
	name, _ := claims["name"].(string)

	user, err := s.getUser(ctx, itsc, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get user from database: %w", err)
	}

	return user, nil
}

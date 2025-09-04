package handlers_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/machinebox/graphql"
	"github.com/stretchr/testify/mock"
	"github.com/zinc-sig/webhook/handlers"
)

// MockGraphQLClient is a mock type for the GraphQLClient type
type MockGraphQLClient struct {
	mock.Mock
}

// Run is a mock method for the Run method
func (m *MockGraphQLClient) Run(ctx context.Context, req *graphql.Request, resp interface{}) error {
	args := m.Called(ctx, req, resp)
	return args.Error(0)
}

func generateTestToken(t *testing.T) (string, *rsa.PrivateKey) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	claims := jwt.MapClaims{
		"exp":   time.Now().Add(time.Hour * 24).Unix(),
		"iat":   time.Now().Unix(),
		"nbf":   time.Now().Unix(),
		"sub":   "test",
		"name":  "Test User",
		"email": "test@example.com",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-kid"
	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}

	return tokenString, privateKey
}

func mockJWKFetch(t *testing.T, privateKey *rsa.PrivateKey) func() {
	originalFunc := handlers.JwkFetch
	handlers.JwkFetch = func(ctx context.Context, url string, options ...jwk.FetchOption) (jwk.Set, error) {
		set := jwk.NewSet()
		key, err := jwk.New(privateKey.PublicKey)
		if err != nil {
			t.Fatal(err)
		}
		key.Set(jwk.KeyIDKey, "test-kid")
		set.Add(key)
		return set, nil
	}

	return func() {
		handlers.JwkFetch = originalFunc
	}
}
package mock

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/golang-jwt/jwt/v4"
	"github.com/machinebox/graphql"
	container "github.com/narwhl/mockestra/redis"
	"github.com/stretchr/testify/mock"
	"github.com/testcontainers/testcontainers-go"
	"github.com/zinc-sig/webhook/pkg/cache"
	"go.uber.org/fx"
)

type MockGraphQLClient struct {
	mock.Mock
}

// Run is a mock method for the Run method
func (m *MockGraphQLClient) Run(ctx context.Context, req *graphql.Request, resp interface{}) error {
	args := m.Called(ctx, req, resp)
	return args.Error(0)
}

func ProvideMockCacheService(t *testing.T) fx.Option {
	return fx.Options(
		fx.NopLogger,
		fx.Supply(
			fx.Annotate(
				"8-alpine",
				fx.ResultTags(`name:"redis_version"`),
			),
		),
		fx.Supply(fx.Annotate(
			fmt.Sprintf("redis-test-%x", time.Now().Unix()),
			fx.ResultTags(`name:"prefix"`),
		)),
		container.Module(),
		fx.Provide(func(params struct {
			fx.In
			Container testcontainers.Container `name:"redis"`
		}) *redis.Options {
			endpoint, err := params.Container.PortEndpoint(t.Context(), container.Port, "")
			if err != nil {
				t.Errorf("failed to get endpoint: %v", err)
			}
			return &redis.Options{
				Addr:     endpoint,
				Password: "", // no password set
				DB:       0,  // use default DB
			}
		}),
		fx.Provide(
			cache.NewService,
		),
	)
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

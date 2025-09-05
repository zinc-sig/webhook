package user_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-redis/redismock/v8"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/zinc-sig/webhook/handlers"
)

func TestIdentity(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Setup
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"headers":{"Cookie":"appSession=test"}}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Mocks
		mockGraphQLClient := new(MockGraphQLClient)
		db, mockRedis := redismock.NewClientMock()

		h := &handlers.Handler{GraphQLClient: mockGraphQLClient, RedisClient: db}

		// Generate a test token
		tokenString, privateKey := generateTestToken(t)
		defer mockJWKFetch(t, privateKey)()

		// Expectations
		mockRedis.Regexp().ExpectGet(".*").SetVal(fmt.Sprintf(`{"data":{"id_token":"%s"}}`, tokenString))
		mockGraphQLClient.On("Run", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		// Assertions
		if assert.NoError(t, h.Identity(c)) {
			assert.Equal(t, http.StatusOK, rec.Code)
		}
	})

	t.Run("invalid session", func(t *testing.T) {
		// Setup
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"headers":{"Cookie":"appSession=test"}}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Mocks
		mockGraphQLClient := new(MockGraphQLClient)
		db, mockRedis := redismock.NewClientMock()

		h := &handlers.Handler{GraphQLClient: mockGraphQLClient, RedisClient: db}

		// Expectations
		mockRedis.ExpectGet(mock.Anything).RedisNil()

		// Assertions
		if assert.NoError(t, h.Identity(c)) {
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		}
	})
}

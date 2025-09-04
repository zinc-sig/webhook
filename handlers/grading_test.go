package handlers_test

import (
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

func TestPostGradingProcessing(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Setup
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"event":{"data":{"new":{"id":1,"pipeline_results":{},"is_final":true}}}}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Mocks
		mockGraphQLClient := new(MockGraphQLClient)

		h := &handlers.Handler{GraphQLClient: mockGraphQLClient}

		// Expectations
		mockGraphQLClient.On("Run", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		// Assertions
		if assert.NoError(t, h.PostGradingProcessing(c)) {
			assert.Equal(t, http.StatusOK, rec.Code)
		}
	})
}

func TestScheduleGrading(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Mock Hasura API
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Setup
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"event":{"op":"INSERT","data":{"new":{"id":1,"stop_collection_at":"2025-09-04T12:00:00Z"}}}}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Mocks
		mockGraphQLClient := new(MockGraphQLClient)

		h := &handlers.Handler{GraphQLClient: mockGraphQLClient}

		// Assertions
		if assert.NoError(t, h.ScheduleGrading(c, server.URL)) {
			assert.Equal(t, http.StatusOK, rec.Code)
		}
	})
}

func TestManualGradingTask(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Setup
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"submissions":[1,2,3]}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("assignmentConfigId")
		c.SetParamValues("1")

		// Mocks
		mockGraphQLClient := new(MockGraphQLClient)
		db, mockRedis := redismock.NewClientMock()

		h := &handlers.Handler{GraphQLClient: mockGraphQLClient, RedisClient: db}

		// Expectations
		mockGraphQLClient.On("Run", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockRedis.ExpectRPush(mock.Anything, mock.Anything).SetVal(1)

		// Assertions
		if assert.NoError(t, h.ManualGradingTask(c)) {
			assert.Equal(t, http.StatusOK, rec.Code)
		}
	})
}

func TestGradingTask(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Setup
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"payload":{"assignment_config_id":1,"stop_collection_at":"2025-09-04T12:00:00Z"}}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Mocks
		mockGraphQLClient := new(MockGraphQLClient)
		db, mockRedis := redismock.NewClientMock()

		h := &handlers.Handler{GraphQLClient: mockGraphQLClient, RedisClient: db}

		// Expectations
		mockGraphQLClient.On("Run", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockRedis.ExpectRPush(mock.Anything, mock.Anything).SetVal(1)

		// Assertions
		if assert.NoError(t, h.GradingTask(c)) {
			assert.Equal(t, http.StatusOK, rec.Code)
		}
	})
}
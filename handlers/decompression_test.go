package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/zinc-sig/webhook/handlers"
)

func mockExecCommand(t *testing.T) func() {
	originalExecCommand := handlers.ExecCommand
	handlers.ExecCommand = func(name string, arg ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", name}
		cs = append(cs, arg...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return cmd
	}
	return func() {
		handlers.ExecCommand = originalExecCommand
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	os.Exit(0)
}

func TestDecompressSubmission(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Setup
		defer mockExecCommand(t)()
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"event":{"data":{"new":{"id":1,"upload_name":"test.zip","stored_name":"test.zip"}}}}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Mocks
		mockGraphQLClient := new(MockGraphQLClient)

		h := &handlers.Handler{GraphQLClient: mockGraphQLClient}

		// Expectations
		mockGraphQLClient.On("Run", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		// Assertions
		if assert.NoError(t, h.DecompressSubmission(c)) {
			assert.Equal(t, http.StatusOK, rec.Code)
		}
	})

	t.Run("unsupported format", func(t *testing.T) {
		// Setup
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"event":{"data":{"new":{"id":1,"upload_name":"test.rar","stored_name":"test.rar"}}}}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Mocks
		mockGraphQLClient := new(MockGraphQLClient)

		h := &handlers.Handler{GraphQLClient: mockGraphQLClient}

		// Expectations
		mockGraphQLClient.On("Run", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		// Assertions
		if assert.NoError(t, h.DecompressSubmission(c)) {
			assert.Equal(t, http.StatusOK, rec.Code)
		}
	})
}

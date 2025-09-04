package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/zinc-sig/webhook/handlers"
)

func TestSyncEnrollment(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// Mocks
		mockGraphQLClient := new(MockGraphQLClient)
		mockGraphQLClient.On("Run", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		// Mock external API
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/oauth/token":
				w.Write([]byte(`{"access_token":"test"}`))
			case "/sis/class_enrl":
				enrollmentMap := handlers.EnrollmentMap{
					Term:     "2210",
					CrseCode: "COMP1023",
					Classes: []struct {
						CrseTitle string `json:"crseTitle"`
						Section   string `json:"section"`
						ClassType string `json:"classType"`
						Students  []struct {
							EmailAddr    string `json:"emailAddr"`
							EnrollStatus string `json:"enrollStatus"`
						} `json:"students"`
					}{
						{
							CrseTitle: "Test Course",
							Section:   "T1",
							ClassType: "N",
							Students: []struct {
								EmailAddr    string `json:"emailAddr"`
								EnrollStatus string `json:"enrollStatus"`
							}{
								{
									EmailAddr:    "test@example.com",
									EnrollStatus: "Enrolled",
								},
							},
						},
					},
				}
				json.NewEncoder(w).Encode(enrollmentMap)
			}
		}))
		defer server.Close()

		// Setup
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		h := &handlers.Handler{GraphQLClient: mockGraphQLClient}

		// Assertions
		if assert.NoError(t, h.SyncEnrollment(c, server.URL)) {
			assert.Equal(t, http.StatusOK, rec.Code)
		}
	})
}

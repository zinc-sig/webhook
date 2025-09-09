package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/zinc-sig/webhook/pkg/api"
	"github.com/zinc-sig/webhook/pkg/cache"
	"github.com/zinc-sig/webhook/pkg/repository"
)

// MockCache implements cache.Service for testing
type MockCache struct {
	mock.Mock
}

func (m *MockCache) Put(ctx context.Context, key string, value []byte, expiry time.Duration) error {
	args := m.Called(ctx, key, value, expiry)
	return args.Error(0)
}

func (m *MockCache) Read(ctx context.Context, key string) ([]byte, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockCache) Remove(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *MockCache) Publish(ctx context.Context, channel string, message []byte) error {
	args := m.Called(ctx, channel, message)
	return args.Error(0)
}

func (m *MockCache) LoadBalancePublish(ctx context.Context, channels []string, message []byte) error {
	args := m.Called(ctx, channels, message)
	return args.Error(0)
}

func (m *MockCache) Llen(ctx context.Context, channel string) (int64, error) {
	args := m.Called(ctx, channel)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockCache) Subscribe(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockCache) RegisterHandler(jobType string, handler cache.MessageHandler) {
	m.Called(jobType, handler)
}

// MockJWTVerifier implements JWTVerifier for testing
type MockJWTVerifier struct {
	mock.Mock
}

func (m *MockJWTVerifier) VerifyToken(ctx context.Context, tokenString string) (*jwt.Token, error) {
	args := m.Called(ctx, tokenString)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*jwt.Token), args.Error(1)
}

// MockRepository implements repository.Repository for testing
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) GetUser(ctx context.Context, itsc, name string) (*repository.User, error) {
	args := m.Called(ctx, itsc, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*repository.User), args.Error(1)
}

func (m *MockRepository) UpdateExtractedSubmissionEntry(ctx context.Context, id int, extractedPath, failReason string) error {
	args := m.Called(ctx, id, extractedPath, failReason)
	return args.Error(0)
}

func (m *MockRepository) AddCourse(ctx context.Context, code string, semesterID int, title string) (int, error) {
	args := m.Called(ctx, code, semesterID, title)
	return args.Int(0), args.Error(1)
}

func (m *MockRepository) AddSections(ctx context.Context, courseID int, sectionNames []string) (map[string]int, error) {
	args := m.Called(ctx, courseID, sectionNames)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int), args.Error(1)
}

func (m *MockRepository) AddStudentsToCourseSection(ctx context.Context, studentUserIDs []int, sectionID int) error {
	args := m.Called(ctx, studentUserIDs, sectionID)
	return args.Error(0)
}

func (m *MockRepository) AddStudentsToCourse(ctx context.Context, studentUserIDs []int, courseID int) error {
	args := m.Called(ctx, studentUserIDs, courseID)
	return args.Error(0)
}

func (m *MockRepository) RemoveStudentsFromCourse(ctx context.Context, courseID int) error {
	args := m.Called(ctx, courseID)
	return args.Error(0)
}

func (m *MockRepository) RemoveStudentsFromSection(ctx context.Context, sectionID int) error {
	args := m.Called(ctx, sectionID)
	return args.Error(0)
}

func (m *MockRepository) GetStudentUserIds(ctx context.Context, itscIDs []string) ([]int, error) {
	args := m.Called(ctx, itscIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]int), args.Error(1)
}

func (m *MockRepository) CreateSemesterIfNotExist(ctx context.Context, id int) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockRepository) GetStudentCourseEnrollmentMap(courseCode string) (*repository.EnrollmentMap, error) {
	args := m.Called(courseCode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*repository.EnrollmentMap), args.Error(1)
}

func (m *MockRepository) UpdateReportEntry(ctx context.Context, report map[string]interface{}) error {
	args := m.Called(ctx, report)
	return args.Error(0)
}

func (m *MockRepository) GetGradingSubmissions(ctx context.Context, assignmentConfigID int) (*repository.Assignment, error) {
	args := m.Called(ctx, assignmentConfigID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*repository.Assignment), args.Error(1)
}

func (m *MockRepository) GetLatestOrSelectedSubmissions(ctx context.Context, assignmentConfigID int, selectedSubmissionIDs []int) ([]repository.Submission, error) {
	args := m.Called(ctx, assignmentConfigID, selectedSubmissionIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]repository.Submission), args.Error(1)
}

func (m *MockRepository) ExtractZip(submissionID int, storedName string) error {
	args := m.Called(submissionID, storedName)
	return args.Error(0)
}

func TestIdentity(t *testing.T) {
	tests := []struct {
		name           string
		request        IdentityRequest
		setupMocks     func(*MockCache, *MockJWTVerifier, *MockRepository)
		expectedStatus int
		expectedBody   interface{}
	}{
		{
			name: "successful admin user authentication",
			request: IdentityRequest{
				Headers: struct {
					Cookie string `json:"Cookie"`
				}{
					Cookie: "appSession=test-session-id; other=value",
				},
			},
			setupMocks: func(cache *MockCache, jwtVerifier *MockJWTVerifier, userRepo *MockRepository) {
				// Mock cache read for session
				sessionData := TokenSet{
					Data: struct {
						IdToken string `json:"id_token"`
					}{
						IdToken: "test-token",
					},
				}
				sessionJSON, _ := json.Marshal(sessionData)
				cache.On("Read", mock.Anything, mock.Anything).Return(sessionJSON, nil)

				// Mock JWT verification
				claims := jwt.MapClaims{
					"email": "admin@example.com",
					"name":  "Admin User",
				}
				token := &jwt.Token{
					Claims: claims,
					Valid:  true,
				}
				jwtVerifier.On("VerifyToken", mock.Anything, "test-token").Return(token, nil)

				// Mock user repository
				user := &repository.User{
					ID:      1,
					Itsc:    "admin",
					Name:    "Admin User",
					IsAdmin: true,
					Courses: []struct {
						CourseID int `json:"course_id"`
					}{},
				}
				userRepo.On("GetUser", mock.Anything, "admin", "Admin User").Return(user, nil)
			},
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"X-Hasura-User-Id": "admin",
				"X-Hasura-Role":    "admin",
			},
		},
		{
			name: "successful regular user authentication with courses",
			request: IdentityRequest{
				Headers: struct {
					Cookie string `json:"Cookie"`
				}{
					Cookie: "appSession=test-session-id",
				},
			},
			setupMocks: func(cache *MockCache, jwtVerifier *MockJWTVerifier, userRepo *MockRepository) {
				// Mock cache read for session
				sessionData := TokenSet{
					Data: struct {
						IdToken string `json:"id_token"`
					}{
						IdToken: "test-token",
					},
				}
				sessionJSON, _ := json.Marshal(sessionData)
				cache.On("Read", mock.Anything, mock.Anything).Return(sessionJSON, nil)

				// Mock JWT verification
				claims := jwt.MapClaims{
					"email": "user@example.com",
					"name":  "Regular User",
				}
				token := &jwt.Token{
					Claims: claims,
					Valid:  true,
				}
				jwtVerifier.On("VerifyToken", mock.Anything, "test-token").Return(token, nil)

				// Mock user repository
				user := &repository.User{
					ID:      2,
					Itsc:    "user",
					Name:    "Regular User",
					IsAdmin: false,
					Courses: []struct {
						CourseID int `json:"course_id"`
					}{
						{CourseID: 101},
						{CourseID: 102},
					},
				}
				userRepo.On("GetUser", mock.Anything, "user", "Regular User").Return(user, nil)
			},
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"X-Hasura-User-Id":         "user",
				"X-Hasura-Role":            "user",
				"X-Hasura-Allowed-Courses": "{101,102}",
			},
		},
		{
			name: "missing session cookie",
			request: IdentityRequest{
				Headers: struct {
					Cookie string `json:"Cookie"`
				}{
					Cookie: "other=value",
				},
			},
			setupMocks: func(cache *MockCache, jwtVerifier *MockJWTVerifier, userRepo *MockRepository) {
				// No mocks needed - should fail on cookie parsing
			},
			expectedStatus: http.StatusUnauthorized,
			expectedBody: map[string]interface{}{
				"error":   "Unauthorized",
				"message": "could not find request session with auth credentials: http: named cookie not present",
			},
		},
		{
			name: "session not found in cache",
			request: IdentityRequest{
				Headers: struct {
					Cookie string `json:"Cookie"`
				}{
					Cookie: "appSession=test-session-id",
				},
			},
			setupMocks: func(cache *MockCache, jwtVerifier *MockJWTVerifier, userRepo *MockRepository) {
				cache.On("Read", mock.Anything, mock.Anything).Return(nil, errors.New("key not found"))
			},
			expectedStatus: http.StatusUnauthorized,
			expectedBody: map[string]interface{}{
				"error":   "Unauthorized",
				"message": "could not find request session with auth credentials: key not found",
			},
		},
		{
			name: "invalid JWT token",
			request: IdentityRequest{
				Headers: struct {
					Cookie string `json:"Cookie"`
				}{
					Cookie: "appSession=test-session-id",
				},
			},
			setupMocks: func(cache *MockCache, jwtVerifier *MockJWTVerifier, userRepo *MockRepository) {
				sessionData := TokenSet{
					Data: struct {
						IdToken string `json:"id_token"`
					}{
						IdToken: "invalid-token",
					},
				}
				sessionJSON, _ := json.Marshal(sessionData)
				cache.On("Read", mock.Anything, mock.Anything).Return(sessionJSON, nil)

				jwtVerifier.On("VerifyToken", mock.Anything, "invalid-token").Return(nil, errors.New("invalid token"))
			},
			expectedStatus: http.StatusUnauthorized,
			expectedBody: map[string]interface{}{
				"error":   "Unauthorized",
				"message": "failed to verify token: invalid token",
			},
		},
		{
			name: "user repository error",
			request: IdentityRequest{
				Headers: struct {
					Cookie string `json:"Cookie"`
				}{
					Cookie: "appSession=test-session-id",
				},
			},
			setupMocks: func(cache *MockCache, jwtVerifier *MockJWTVerifier, userRepo *MockRepository) {
				sessionData := TokenSet{
					Data: struct {
						IdToken string `json:"id_token"`
					}{
						IdToken: "test-token",
					},
				}
				sessionJSON, _ := json.Marshal(sessionData)
				cache.On("Read", mock.Anything, mock.Anything).Return(sessionJSON, nil)

				claims := jwt.MapClaims{
					"email": "user@example.com",
					"name":  "Test User",
				}
				token := &jwt.Token{
					Claims: claims,
					Valid:  true,
				}
				jwtVerifier.On("VerifyToken", mock.Anything, "test-token").Return(token, nil)

				userRepo.On("GetUser", mock.Anything, "user", "Test User").Return(nil, errors.New("database error"))
			},
			expectedStatus: http.StatusUnauthorized,
			expectedBody: map[string]interface{}{
				"error":   "Unauthorized",
				"message": "failed to get user from database: database error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockCache := new(MockCache)
			mockJWTVerifier := new(MockJWTVerifier)
			mockRepo := new(MockRepository)

			if tt.setupMocks != nil {
				tt.setupMocks(mockCache, mockJWTVerifier, mockRepo)
			}

			// Create service with mocks
			service := &service{
				cache:         mockCache,
				sessionSecret: "test-secret",
				jwtVerifier:   mockJWTVerifier,
				repository:    mockRepo,
			}

			// Setup Echo
			e := echo.New()
			reqBody, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/identity", bytes.NewReader(reqBody))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			// Call handler
			handler := Identity(service)
			err := handler(c)

			// Assertions
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, rec.Code)

			// Check response body
			var response map[string]interface{}
			json.Unmarshal(rec.Body.Bytes(), &response)

			// Compare expected fields (ignore timestamp and optional fields)
			for key, expectedValue := range tt.expectedBody.(map[string]interface{}) {
				if key != "X-Hasura-Requested-At" {
					actualValue, exists := response[key]
					if expectedValue == "" && !exists {
						// Empty string and missing field are equivalent for optional fields
						continue
					}
					assert.Equal(t, expectedValue, actualValue)
				}
			}

			// Verify timestamp field exists for successful requests
			if tt.expectedStatus == http.StatusOK {
				assert.NotEmpty(t, response["X-Hasura-Requested-At"])
			}

			// Verify mocks were called as expected
			mockCache.AssertExpectations(t)
			mockJWTVerifier.AssertExpectations(t)
			mockRepo.AssertExpectations(t)
		})
	}
}

func TestIdentity_InvalidRequest(t *testing.T) {
	// Test invalid JSON binding
	service := &service{}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/identity", bytes.NewReader([]byte("invalid json")))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := Identity(service)
	err := handler(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var response api.ErrorResponse
	json.Unmarshal(rec.Body.Bytes(), &response)
	assert.Contains(t, response.Message, "invalid character")
}

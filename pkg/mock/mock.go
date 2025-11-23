package mock

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
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
	"github.com/zinc-sig/webhook/pkg/repository"
	"go.uber.org/fx"
)

type MockGraphQLClient struct {
	mock.Mock
}

// Run is a mock method for the Run method
func (m *MockGraphQLClient) Run(ctx context.Context, req *graphql.Request, resp any) error {
	args := m.Called(ctx, req, resp)
	return args.Error(0)
}

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

func (m *MockRepository) UpdateReportEntry(ctx context.Context, report map[string]any) error {
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

func (m *MockRepository) GetGradingPolicy(ctx context.Context, assignmentConfigID, userID int) (bool, bool, error) {
	args := m.Called(ctx, assignmentConfigID, userID)
	return args.Bool(0), args.Bool(1), args.Error(2)
}

func ProvideMockRepository(t *testing.T) fx.Option {
	mockRepo := new(MockRepository)
	return fx.Options(
		fx.NopLogger,
		fx.Supply(mockRepo),
		fx.Supply(
			fx.Annotate(
				mockRepo,
				fx.As(new(repository.Repository)),
			),
		),
	)
}

func ProvideMockCacheService(t *testing.T) fx.Option {
	bytes := make([]byte, 5)
	if _, err := rand.Read(bytes); err != nil {
		t.Fatalf("failed to generate random bytes: %v", err)
	}
	return fx.Options(
		fx.NopLogger,
		fx.Supply(
			fx.Annotate(
				"8-alpine",
				fx.ResultTags(`name:"redis_version"`),
			),
		),
		fx.Supply(fx.Annotate(
			fmt.Sprintf("redis-test-%x", hex.EncodeToString(bytes)),
			fx.ResultTags(`name:"prefix"`),
		)),
		container.Module(),
		fx.Provide(func(params struct {
			fx.In
			Container testcontainers.Container `name:"redis"`
		}) *cache.Config {
			endpoint, err := params.Container.PortEndpoint(t.Context(), container.Port, "")
			if err != nil {
				t.Errorf("failed to get endpoint: %v", err)
			}
			return &cache.Config{
				DSN: endpoint,
			}
		}),
		// Cache Service
		fx.Provide(
			cache.NewService,
		),
		// Redis Client
		fx.Provide(
			func(p struct {
				fx.In
				Config *cache.Config
			}) *redis.Client {
				return redis.NewClient(&redis.Options{
					Addr:     p.Config.DSN, // e.g., "localhost:6379"
					Password: "",           // no password set
					DB:       0,            // use default DB
				})
			},
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

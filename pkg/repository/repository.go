package repository

import (
	"context"
	"fmt"
	"net/http"

	"github.com/machinebox/graphql"
	"go.uber.org/fx"
)

type Config struct {
	HasuraURL         string                  `mapstructure:"hasura_url" yaml:"hasura_url"`
	HasuraAdminSecret string                  `mapstructure:"hasura_admin_secret" yaml:"hasura_admin_secret"`
	SharedMountPath   string                  `mapstructure:"shared_mount_path" yaml:"shared_mount_path"`
	IntegrationConfig EnrollmentServiceConfig `mapstructure:"iso" yaml:"iso"`
}

type EnrollmentServiceConfig struct {
	ApiURL       string `mapstructure:"api_url" yaml:"api_url"`
	ClientID     string `mapstructure:"client_id" yaml:"client_id"`
	ClientSecret string `mapstructure:"client_secret" yaml:"client_secret"`
	Username     string `mapstructure:"username" yaml:"username"`
	Password     string `mapstructure:"password" yaml:"password"`
}

type Repository interface {
	GetUser(ctx context.Context, itsc, name string) (*User, error)
	UpdateExtractedSubmissionEntry(ctx context.Context, id int, extractedPath, failReason string) error
	AddCourse(ctx context.Context, code string, semesterID int, title string) (int, error)
	AddSections(ctx context.Context, courseID int, sectionNames []string) (map[string]int, error)
	AddStudentsToCourseSection(ctx context.Context, studentUserIDs []int, sectionID int) error
	AddStudentsToCourse(ctx context.Context, studentUserIDs []int, courseID int) error
	RemoveStudentsFromCourse(ctx context.Context, courseID int) error
	RemoveStudentsFromSection(ctx context.Context, sectionID int) error
	GetStudentUserIds(ctx context.Context, itscIDs []string) ([]int, error)
	CreateSemesterIfNotExist(ctx context.Context, id int) error
	GetStudentCourseEnrollmentMap(courseCode string) (*EnrollmentMap, error)
	UpdateReportEntry(ctx context.Context, report map[string]interface{}) error
	GetGradingSubmissions(ctx context.Context, assignmentConfigID int) (*Assignment, error)
	GetLatestOrSelectedSubmissions(ctx context.Context, assignmentConfigID int, selectedSubmissionIDs []int) ([]Submission, error)
	ExtractZip(submissionID int, storedName string) error
}

type AuthTransport struct {
	AdminSecret string
}

func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("X-Hasura-Admin-Secret", t.AdminSecret)
	return http.DefaultTransport.RoundTrip(req)
}

type repository struct {
	sharedMountPath string
	client          *graphql.Client
	isoConfig       EnrollmentServiceConfig
}

type Params struct {
	fx.In
	Config *Config
}

func NewRepository(p Params) Repository {
	httpclient := &http.Client{Transport: &AuthTransport{AdminSecret: p.Config.HasuraAdminSecret}}
	client := graphql.NewClient(
		fmt.Sprintf("%s/v1/graphql", p.Config.HasuraURL),
		graphql.WithHTTPClient(httpclient),
	)
	return &repository{
		client:          client,
		isoConfig:       p.Config.IntegrationConfig,
		sharedMountPath: p.Config.SharedMountPath,
	}
}

var Module = fx.Module(
	"repository",
	fx.Provide(
		NewRepository,
	),
)

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
	UpdateReportEntry(ctx context.Context, report map[string]any) error
	GetGradingSubmissions(ctx context.Context, assignmentConfigID int) (*Assignment, error)
	GetLatestOrSelectedSubmissions(ctx context.Context, assignmentConfigID int, selectedSubmissionIDs []int) ([]Submission, error)
	ExtractZip(submissionID int, storedName string) error
	CreateSubmission(ctx context.Context, userID int, assignmentConfigID int, storedName, uploadName string, fileSize int64, checksum string, cookie *http.Cookie) (int, error)
	GetGradingPolicy(ctx context.Context, assignmentConfigID int, userID int) (bool, bool, error)
	GetSubmissionGrades(ctx context.Context, assignmentConfigID int, response any) error
	GetSubmissionByID(ctx context.Context, submissionID int, response any) error
	GetSubmissionFilePath(storedName string) string
	GetAllSubmissionsForAssignmentConfig(ctx context.Context, assignmentConfigID int, response any) error
}

type repository struct {
	sharedMountPath string
	client          *graphql.Client
	isoConfig       EnrollmentServiceConfig
	adminSecret     string
}

type Params struct {
	fx.In
	Config *Config
}

func NewRepository(p Params) Repository {
	client := graphql.NewClient(
		fmt.Sprintf("%s/v1/graphql", p.Config.HasuraURL),
	)
	return &repository{
		client:          client,
		isoConfig:       p.Config.IntegrationConfig,
		sharedMountPath: p.Config.SharedMountPath,
		adminSecret:     p.Config.HasuraAdminSecret,
	}
}

func (r *repository) WithAdminSecret(req *graphql.Request) *graphql.Request {
	req.Header.Add("X-Hasura-Admin-Secret", r.adminSecret)
	return req
}

func (r *repository) WithCookie(req *graphql.Request, cookie *http.Cookie) *graphql.Request {
	req.Header.Add("Cookie", cookie.String())
	return req
}

var Module = fx.Module(
	"repository",
	fx.Provide(
		NewRepository,
	),
)

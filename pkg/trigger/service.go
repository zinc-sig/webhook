package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/zinc-sig/webhook/pkg/api"
	"github.com/zinc-sig/webhook/pkg/cache"
	"github.com/zinc-sig/webhook/pkg/repository"
	"go.uber.org/fx"
)

const (
	LoopbackAddress = "127.0.0.1"
)

type GradingPayload struct {
	ID            int       `json:"id"`
	ExtractedPath string    `json:"extracted_path"`
	CreatedAt     time.Time `json:"created_at"`
}

type ServiceParams struct {
	fx.In
	Config     *repository.Config
	Cache      cache.Service
	Repository repository.Repository
}

type service struct {
	cache      cache.Service
	repository repository.Repository
	config     *repository.Config
}

func NewService(p ServiceParams) *service {
	return &service{
		cache:      p.Cache,
		repository: p.Repository,
		config:     p.Config,
	}
}

func (s *service) RegisterRoutes(e *echo.Echo) {
	e.POST("/trigger/syncEnrollment", SyncEnrollment(s))
	e.POST("/trigger/decompression", DecompressSubmission(s))
	e.POST("/trigger/postGradingProcessing", PostGradingProcessing(s))
	e.POST("/trigger/scheduleGrading", ScheduleGrading(s))
	e.POST("/trigger/manualGradingTask/:assignmentConfigId", ManualGradingTask(s))
	e.POST("/trigger/gradingTask", GradingTask(s))
	e.PUT("/grader/queues", UpdateGraderQueues(s))
}

func (s *service) SyncEnrollment(ctx context.Context) error {
	courses := []string{"COMP1023", "COMP2011", "COMP2012", "COMP2211"}

	for _, course := range courses {
		enrollmentMap, err := s.repository.GetStudentCourseEnrollmentMap(course)
		if err != nil {
			slog.Warn("Failed to get enrollment map", "course", course, "error", err)
			return fmt.Errorf("failed to get enrollment map for course %s: %s", course, err.Error())
		}

		term, err := strconv.Atoi(enrollmentMap.Term)
		if err != nil {
			slog.Warn("Failed to parse term", "course", course, "error", err)
			return fmt.Errorf("failed to parse term for course %s: %s", course, err.Error())
		}
		if err := s.repository.CreateSemesterIfNotExist(ctx, term); err != nil {
			slog.Warn("Failed to create semester", "course", course, "error", err)
			return fmt.Errorf("failed to create semester for course %s: %s", course, err.Error())
		}

		courseID, err := s.repository.AddCourse(ctx, enrollmentMap.CrseCode, term, enrollmentMap.Classes[0].CrseTitle)
		if err != nil {
			slog.Warn("Failed to add course", "course", course, "error", err)
			return fmt.Errorf("failed to add course %s: %s", course, err.Error())
		}

		var sectionNames []string
		for _, class := range enrollmentMap.Classes {
			if class.ClassType == "N" {
				sectionNames = append(sectionNames, class.Section)
			}
		}

		sections, err := s.repository.AddSections(ctx, courseID, sectionNames)
		if err != nil {
			slog.Warn("Failed to add sections", "course", course, "error", err)
			return fmt.Errorf("failed to add sections for course %s: %s", course, err.Error())
		}

		if err := s.repository.RemoveStudentsFromCourse(ctx, courseID); err != nil {
			slog.Warn("Failed to remove students from course", "course", course, "error", err)
			return fmt.Errorf("failed to remove students from course %s: %s", course, err.Error())
		}

		if err := s.repository.RemoveStudentsFromSection(ctx, courseID); err != nil {
			slog.Warn("Failed to remove students from section", "course", course, "error", err)
			return fmt.Errorf("failed to remove students from section for course %s: %s", course, err.Error())
		}

		for _, class := range enrollmentMap.Classes {
			var itscIDs []string
			for _, student := range class.Students {
				if student.EnrollStatus == "Enrolled" {
					itscIDs = append(itscIDs, strings.Split(student.EmailAddr, "@")[0])
				}
			}

			studentUserIDs, err := s.repository.GetStudentUserIds(ctx, itscIDs)
			if err != nil {
				slog.Warn("Failed to get student user ids", "course", course, "error", err)
				return fmt.Errorf("failed to get student user ids for course %s: %s", course, err.Error())
			}

			switch class.ClassType {
			case "N":
				sectionID := sections[class.Section]
				if err := s.repository.AddStudentsToCourseSection(ctx, studentUserIDs, sectionID); err != nil {
					slog.Warn("Failed to add students to course section", "course", course, "error", err)
					return fmt.Errorf("failed to add students to course section for course %s: %s", course, err.Error())
				}
			case "E":
				if err := s.repository.AddStudentsToCourse(ctx, studentUserIDs, courseID); err != nil {
					slog.Warn("Failed to add students to course", "course", course, "error", err)
					return fmt.Errorf("failed to add students to course for course %s: %s", course, err.Error())
				}
			}
		}
	}

	return nil
}

func (s *service) DecompressSubmission(ctx context.Context, payload json.RawMessage) error {
	var submission SubmissionRow
	if err := json.Unmarshal(payload, &submission); err != nil {
		return fmt.Errorf("failed to unmarshal submission data: %s", err.Error())
	}
	if !strings.HasSuffix(submission.UploadName, ".zip") {
		return s.repository.UpdateExtractedSubmissionEntry(ctx, submission.ID, "", "Unsupported archive format")
	}

	if err := s.repository.ExtractZip(submission.ID, submission.StoredName); err != nil {
		return s.repository.UpdateExtractedSubmissionEntry(ctx, submission.ID, "", err.Error())
	}

	if err := s.repository.UpdateExtractedSubmissionEntry(ctx, submission.ID, fmt.Sprintf("extracted/%d", submission.ID), ""); err != nil {
		return fmt.Errorf("failed to update extracted submission entry: %s", err.Error())
	}

	gradeImmediately, isTest, err := s.repository.GetGradingPolicy(ctx, submission.AssignmentConfigID, submission.UserID)
	if err != nil {
		return fmt.Errorf("failed to get grading policy: %s", err.Error())
	}
	submittedAt, err := time.Parse("2006-01-02T15:04:05", submission.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to parse submission created at: %s", err.Error())
	}
	if gradeImmediately {
		slog.Info("triggered grader for:", "submission", submission.ID)
		payload, err := json.Marshal(map[string]interface{}{
			"submissions": []GradingPayload{
				{
					ID:            submission.ID,
					ExtractedPath: fmt.Sprintf("extracted/%d", submission.ID),
					CreatedAt:     submittedAt,
				},
			},
			"isTest":               isTest,
			"assignment_config_id": submission.AssignmentConfigID,
			"initiatedBy":          nil,
		})
		job, err := json.Marshal(map[string]interface{}{
			"job":     "gradingTask",
			"payload": string(payload),
		})
		if err != nil {
			slog.Warn("Failed to marshal grading payload", "error", err)
			return fmt.Errorf("failed to marshal grading payload: %s", err.Error())
		}
		data, err := s.cache.Read(ctx, cache.QueueKey)
		if err != nil {
			slog.Warn("Failed to read grader queues", "error", err)
			return fmt.Errorf("failed to read grader queues: %s", err.Error())
		}
		if data == nil {
			return fmt.Errorf("no grader queues configured")
		}
		var queues []string
		for _, queue := range strings.Split(string(data), ",") {
			queues = append(queues, fmt.Sprintf("%s:grader", queue))
		}
		slog.Info("sending job payload", "payload", string(job))
		if err := s.cache.LoadBalancePublish(ctx, queues, job); err != nil {
			slog.Warn("Failed to publish grading payload", "error", err)
			return fmt.Errorf("failed to publish grading payload: %s", err.Error())
		}
	}
	return nil
}

func (s *service) PostGradingProcessing(ctx context.Context, payload json.RawMessage) error {
	var report ReportRow
	if err := json.Unmarshal(payload, &report); err != nil {
		slog.Warn("Failed to unmarshal report data", "error", err)
		return fmt.Errorf("failed to unmarshal report data: %s", err.Error())
	}
	var pipelineResults PipelineResults
	if err := json.Unmarshal(report.PipelineResults, &pipelineResults); err != nil {
		slog.Warn("Failed to parse pipeline results", "error", err)
		return fmt.Errorf("failed to parse pipeline results: %s", err.Error())
	}

	censoredReports := make(map[string]interface{})
	var grade map[string]interface{}

	for stage, stageReport := range pipelineResults.StageReports {
		switch stage {
		case "valgrind":
			var valgrindReports []ValgrindReport
			if err := json.Unmarshal(stageReport, &valgrindReports); err == nil {
				for i, r := range valgrindReports {
					switch r.Visibility {
					case "ALWAYS_HIDDEN":
						valgrindReports[i].Stdout = []string{}
						valgrindReports[i].Errors = []string{}
					case "VISIBLE_AFTER_GRADING":
						if !report.IsFinal {
							valgrindReports[i].Stdout = []string{}
							valgrindReports[i].Errors = []string{}
						}
					case "VISIBLE_AFTER_GRADING_IF_FAILED":
						if !report.IsFinal || r.IsCorrect {
							valgrindReports[i].Stdout = []string{}
							valgrindReports[i].Errors = []string{}
						}
					}
				}
				censoredReports[stage] = valgrindReports
			}
		case "stdioTest":
			var stdioTestReports []StdioTestReport
			if err := json.Unmarshal(stageReport, &stdioTestReports); err == nil {
				for i, r := range stdioTestReports {
					switch r.Visibility {
					case "ALWAYS_HIDDEN":
						stdioTestReports[i].Stdout = []string{}
						stdioTestReports[i].Expect = []string{}
						stdioTestReports[i].Diff = []string{}
					case "VISIBLE_AFTER_GRADING":
						if !report.IsFinal {
							stdioTestReports[i].Expect = []string{}
							stdioTestReports[i].Diff = []string{}
						}
					case "VISIBLE_AFTER_GRADING_IF_FAILED":
						if !report.IsFinal || r.IsCorrect {
							stdioTestReports[i].Expect = []string{}
							stdioTestReports[i].Diff = []string{}
						}
					}
				}
				censoredReports[stage] = stdioTestReports
			}
		case "score":
			var scoreReportObj []map[string]interface{}
			if err := json.Unmarshal(stageReport, &scoreReportObj); err == nil && len(scoreReportObj) > 0 {
				grade = scoreReportObj[0]
			}
		default:
			censoredReports[stage] = stageReport
		}
	}

	if grade != nil && pipelineResults.ScoreReports != nil {
		var scoreReportsObj interface{}
		if err := json.Unmarshal(pipelineResults.ScoreReports, &scoreReportsObj); err == nil {
			grade["details"] = scoreReportsObj
		}
	}

	update := map[string]interface{}{
		"id":               report.ID,
		"sanitizedReports": censoredReports,
		"grade":            grade,
	}
	if err := s.repository.UpdateReportEntry(ctx, update); err != nil {
		slog.Warn("Failed to update report entry", "reportID", report.ID, "error", err)
		return fmt.Errorf("failed to update report entry: %s", err.Error())
	}
	return nil
}

func (s *service) ManualGradingTask(ctx context.Context, assignmentConfigId int, req *ManualGradingTaskRequest) error {
	submissions, err := s.repository.GetLatestOrSelectedSubmissions(ctx, assignmentConfigId, req.Submissions)
	if err != nil {
		slog.Warn("Failed to get submissions", "assignmentConfigId", assignmentConfigId, "error", err)
		return fmt.Errorf("failed to get submissions: %s", err.Error())
	}
	// Push job to redis
	payload := map[string]interface{}{
		"submissions":          submissions,
		"assignment_config_id": assignmentConfigId,
		"isTest":               false,
		"initiatedBy":          req.InitiatedBy,
	}

	jsonPayload, err := json.Marshal(map[string]interface{}{
		"job":     "manualGradingTask",
		"payload": payload,
	})
	if err != nil {
		slog.Warn("Failed to marshal job payload", "error", err)
		return fmt.Errorf("failed to marshal job payload: %s", err.Error())
	}

	data, err := s.cache.Read(ctx, cache.QueueKey)
	if err != nil {
		slog.Warn("Failed to read grader queues", "error", err)
		return fmt.Errorf("failed to read grader queues: %s", err.Error())
	}
	if data == nil {
		return fmt.Errorf("no grader queues configured")
	}
	var queues []string
	for _, queue := range strings.Split(string(data), ",") {
		queues = append(queues, fmt.Sprintf("%s:grader", queue))
	}
	if err := s.cache.LoadBalancePublish(ctx, queues, jsonPayload); err != nil {
		slog.Warn("Failed to publish grading payload", "error", err)
		return fmt.Errorf("failed to publish grading payload: %s", err.Error())
	}
	return nil
}

func (s *service) GradingTask(ctx context.Context, payload *GradingTaskRequest) error {
	submissions, err := s.repository.GetGradingSubmissions(ctx, payload.Payload.AssignmentConfigID)
	if err != nil {
		slog.Warn("Failed to get grading submissions", "assignmentConfigID", payload.Payload.AssignmentConfigID, "error", err)
		return fmt.Errorf("failed to get grading submissions: %s", err.Error())
	}

	if *submissions.AssignmentConfig.StopCollectionAt == payload.Payload.StopCollectionAt {
		// Push job to redis
		payload := map[string]interface{}{
			"submissions":          submissions.AssignmentConfig.Submissions,
			"assignment_config_id": payload.Payload.AssignmentConfigID,
			"isTest":               false,
		}

		jsonPayload, err := json.Marshal(map[string]interface{}{
			"job":     "gradingTask",
			"payload": payload,
		})
		if err != nil {
			slog.Warn("Failed to marshal job payload", "error", err)
			return fmt.Errorf("failed to marshal job payload: %s", err.Error())
		}

		if err := s.cache.Publish(ctx, "zinc_queue:grader", jsonPayload); err != nil {
			slog.Warn("Failed to push job to redis", "error", err)
			return fmt.Errorf("failed to push job to redis: %s", err.Error())
		}
	}
	return nil
}

func (s *service) ScheduleGrading(ctx context.Context, event *RowTriggerEvent) error {
	var oldGrading GradingRow
	if err := json.Unmarshal(event.Data.Old, &oldGrading); err != nil {
		return fmt.Errorf("failed to unmarshal old grading data: %s", err.Error())
	}
	var newGrading GradingRow
	if err := json.Unmarshal(event.Data.New, &newGrading); err != nil {
		return fmt.Errorf("failed to unmarshal new grading data: %s", err.Error())
	}
	if (event.Op == "UPDATE" && oldGrading.StopCollectionAt != newGrading.StopCollectionAt) || event.Op == "INSERT" {
		webhookURL := fmt.Sprintf("http://%s:%d/trigger/gradingTask", LoopbackAddress, api.Port)

		payload := map[string]interface{}{
			"type": "create_scheduled_event",
			"args": map[string]interface{}{
				"webhook":     webhookURL,
				"schedule_at": newGrading.StopCollectionAt,
				"payload": map[string]interface{}{
					"assignment_config_id": newGrading.ID,
					"stop_collection_at":   newGrading.StopCollectionAt,
				},
			},
		}

		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal request payload: %s", err.Error())
		}

		httpReq, err := http.NewRequest("POST", fmt.Sprintf("%s/query", s.config.HasuraURL), strings.NewReader(string(jsonPayload)))
		if err != nil {
			return fmt.Errorf("failed to create request: %s", err.Error())
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-Hasura-Admin-Secret", s.config.HasuraAdminSecret)

		client := &http.Client{}
		resp, err := client.Do(httpReq)
		if err != nil {
			return fmt.Errorf("failed to send request to hasura: %s", err.Error())
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to schedule grading event")
		}
	}

	return nil
}

func (s *service) UpdateGraderQueues(ctx context.Context, queues []string) error {
	if err := s.cache.Put(ctx, cache.QueueKey, []byte(strings.Join(queues, ",")), 0); err != nil {
		slog.Warn("Failed to update grader queues", "error", err)
		return fmt.Errorf("failed to update grader queues: %s", err.Error())
	}
	return nil
}

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

// buildGradingJobPayload builds and marshals a grading job payload
// It returns the marshaled job payload as []byte ready for publishing to Redis
func (s *service) buildGradingJobPayload(jobType string, gradingPayloads []GradingPayload, assignmentConfigID int, isTest bool, initiatedBy *int) ([]byte, error) {
	// Build the payload map
	payloadMap := map[string]interface{}{
		"submissions":          gradingPayloads,
		"assignment_config_id": assignmentConfigID,
		"isTest":               isTest,
	}

	// Add optional initiatedBy field if provided
	if initiatedBy != nil {
		payloadMap["initiatedBy"] = *initiatedBy
	} else {
		payloadMap["initiatedBy"] = nil
	}

	// Marshal the payload
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		slog.Warn("Failed to marshal grading payload", "error", err)
		return nil, fmt.Errorf("failed to marshal grading payload: %s", err.Error())
	}

	// Create the job wrapper
	job, err := json.Marshal(map[string]interface{}{
		"job":     jobType,
		"payload": string(payload),
	})
	if err != nil {
		slog.Warn("Failed to marshal job payload", "error", err)
		return nil, fmt.Errorf("failed to marshal job payload: %s", err.Error())
	}

	return job, nil
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
		slog.Info("synced enrollment for course", "course", course, "courseID", courseID, "sectionsCount", len(sections), "term", term)
	}

	slog.Info("enrollment sync completed", "coursesProcessed", len(courses))
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

	extractedPath := fmt.Sprintf("extracted/%d", submission.ID)
	if err := s.repository.UpdateExtractedSubmissionEntry(ctx, submission.ID, extractedPath, ""); err != nil {
		return fmt.Errorf("failed to update extracted submission entry: %s", err.Error())
	}
	slog.Info("decompressed submission successfully", "submissionID", submission.ID, "extractedPath", extractedPath)

	gradeImmediately, isTest, err := s.repository.GetGradingPolicy(ctx, submission.AssignmentConfigID, submission.UserID)
	if err != nil {
		return fmt.Errorf("failed to get grading policy: %s", err.Error())
	}
	if gradeImmediately {
		slog.Info("triggered grader for:", "submission", submission.ID)
		gradingPayloads := []GradingPayload{
			{
				ID:            submission.ID,
				ExtractedPath: fmt.Sprintf("extracted/%d", submission.ID),
				CreatedAt:     submission.CreatedAt.Time(),
			},
		}

		job, err := s.buildGradingJobPayload("gradingTask", gradingPayloads, submission.AssignmentConfigID, isTest, nil)
		if err != nil {
			return err
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
		slog.Info("grading job scheduled for immediate processing", "submissionID", submission.ID, "assignmentConfigID", submission.AssignmentConfigID, "isTest", isTest)
	}
	return nil
}

// processValgrindReports processes valgrind reports according to visibility rules
// Replicates TypeScript behavior from /tmp/grading.ts lines 16-39
func processValgrindReports(stageReport json.RawMessage, isFinal bool) []ValgrindReport {
	var valgrindReports []ValgrindReport
	if err := json.Unmarshal(stageReport, &valgrindReports); err != nil {
		return nil
	}
	
	for i, report := range valgrindReports {
		switch report.Visibility {
		case "ALWAYS_HIDDEN":
			valgrindReports[i].Stdout = []string{}
			valgrindReports[i].Errors = []ValgrindReportError{}
			// TypeScript returns the modified report here
			continue
			
		case "VISIBLE_AFTER_GRADING":
			if !isFinal {
				valgrindReports[i].Stdout = []string{}
				valgrindReports[i].Errors = []ValgrindReportError{}
				// TypeScript returns the modified report here
				continue
			}
			// BUG: TypeScript is missing 'return report' here at line 28
			// This causes execution to continue to line 29 and beyond
			// In Go, we don't add 'continue' to replicate this bug
			
		case "VISIBLE_AFTER_GRADING_IF_FAILED":
			// TypeScript condition: !is_final || !report.isCorrect (line 30)
			// This means: hide data if not final OR if test failed
			if !isFinal || !report.IsCorrect {
				valgrindReports[i].Stdout = []string{}
				valgrindReports[i].Errors = []ValgrindReportError{}
				// TypeScript returns the modified report here
				continue
			}
			// BUG: TypeScript is missing 'return report' here at line 34
			// This causes execution to continue to case 'ALWAYS_VISIBLE'
			// In Go, we don't add 'continue' to replicate this bug
			
		case "ALWAYS_VISIBLE":
		default:
			// TypeScript returns unmodified report here
			// In Go, we do nothing and let the loop continue
		}
	}
	
	return valgrindReports
}

// processStdioTestReports processes stdio test reports according to visibility rules
// Replicates TypeScript behavior from /tmp/grading.ts lines 40-64
func processStdioTestReports(stageReport json.RawMessage, isFinal bool) []StdioTestReport {
	var stdioTestReports []StdioTestReport
	if err := json.Unmarshal(stageReport, &stdioTestReports); err != nil {
		return nil
	}
	
	for i, report := range stdioTestReports {
		switch report.Visibility {
		case "ALWAYS_HIDDEN":
			stdioTestReports[i].Stdout = []string{}
			stdioTestReports[i].Expect = []string{}
			stdioTestReports[i].Diff = []string{}
			// TypeScript returns the modified report here
			continue
			
		case "VISIBLE_AFTER_GRADING":
			if !isFinal {
				stdioTestReports[i].Expect = []string{}
				stdioTestReports[i].Diff = []string{}
				// TypeScript returns the modified report here
				continue
			}
			// BUG: TypeScript is missing 'return report' here at line 53
			// This causes execution to continue to line 54 and beyond
			// In Go, we don't add 'continue' to replicate this bug
			
		case "VISIBLE_AFTER_GRADING_IF_FAILED":
			// TypeScript condition: !is_final || !report.isCorrect (line 55)
			// This means: hide data if not final OR if test failed
			if !isFinal || !report.IsCorrect {
				stdioTestReports[i].Expect = []string{}
				stdioTestReports[i].Diff = []string{}
				// TypeScript returns the modified report here
				continue
			}
			// BUG: TypeScript is missing 'return report' here at line 59
			// This causes execution to continue to case 'ALWAYS_VISIBLE'
			// In Go, we don't add 'continue' to replicate this bug
			
		case "ALWAYS_VISIBLE":
		default:
			// TypeScript returns unmodified report here
			// In Go, we do nothing and let the loop continue
		}
	}
	
	return stdioTestReports
}

func (s *service) PostGradingProcessing(ctx context.Context, payload json.RawMessage) error {
	var report ReportRow
	if err := json.Unmarshal(payload, &report); err != nil {
		slog.Warn("Failed to unmarshal report data", "error", err)
		return fmt.Errorf("failed to unmarshal report data: %s", err.Error())
	}

	censoredReports := make(map[string]interface{})
	var grade map[string]interface{}

	for stage, stageReport := range report.PipelineResults.StageReports {
		switch stage {
		case "valgrind":
			// Process valgrind reports
			if processed := processValgrindReports(stageReport, report.IsFinal); processed != nil {
				censoredReports[stage] = processed
			}
			// BUG: TypeScript is missing 'break' here at line 39 of /tmp/grading.ts
			// This causes the 'valgrind' case to fall through to 'stdioTest'
			// In Go, we explicitly use 'fallthrough' to replicate this bug
			fallthrough
			
		case "stdioTest":
			// BUG: Due to fallthrough from 'valgrind' case, this will execute
			// for BOTH 'valgrind' AND 'stdioTest' stages
			// When stage == "valgrind", this will try to unmarshal valgrind data
			// as StdioTestReport which will likely fail silently
			if processed := processStdioTestReports(stageReport, report.IsFinal); processed != nil {
				censoredReports[stage] = processed
			}
			// TypeScript has 'break' here at line 65, Go doesn't need it
			
		case "score":
			// TypeScript extracts first element of score array (lines 66-69)
			var scoreReportObj []map[string]interface{}
			if err := json.Unmarshal(stageReport, &scoreReportObj); err == nil && len(scoreReportObj) > 0 {
				grade = scoreReportObj[0]
			}
			// TypeScript has 'break' here at line 69, Go doesn't need it
			
		default:
			// Pass through any other stages unchanged (lines 70-72)
			censoredReports[stage] = stageReport
			// TypeScript has 'break' here at line 72, Go doesn't need it
		}
	}

	// Handle score details - TypeScript lines 75-77
	if grade != nil && report.PipelineResults.ScoreReports != nil {
		var scoreReportsObj interface{}
		if err := json.Unmarshal(report.PipelineResults.ScoreReports, &scoreReportsObj); err == nil {
			grade["details"] = scoreReportsObj
		}
	}

	// Update the report entry with censored data (lines 78-89)
	update := map[string]interface{}{
		"id":               report.ID,
		"sanitizedReports": censoredReports,
		"grade":            grade,
	}
	if err := s.repository.UpdateReportEntry(ctx, update); err != nil {
		slog.Warn("Failed to update report entry", "reportID", report.ID, "error", err)
		return fmt.Errorf("failed to update report entry: %s", err.Error())
	}
	// TypeScript logs success at line 89
	slog.Info("Post-grading artifacts generation completed", "reportID", report.ID)
	return nil
}

func (s *service) ManualGradingTask(ctx context.Context, assignmentConfigId int, req *ManualGradingTaskRequest) error {
	submissions, err := s.repository.GetLatestOrSelectedSubmissions(ctx, assignmentConfigId, req.Submissions)
	if err != nil {
		slog.Warn("Failed to get submissions", "assignmentConfigId", assignmentConfigId, "error", err)
		return fmt.Errorf("failed to get submissions: %s", err.Error())
	}

	var gradingPayloads []GradingPayload

	for _, submission := range submissions {
		gradingPayloads = append(gradingPayloads, GradingPayload{
			ID:            submission.ID,
			ExtractedPath: submission.ExtractedPath,
			CreatedAt:     submission.CreatedAt.Time(),
		})
	}

	// Build job payload
	jsonPayload, err := s.buildGradingJobPayload("manualGradingTask", gradingPayloads, assignmentConfigId, false, &req.InitiatedBy)
	if err != nil {
		return err
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
	slog.Info("sending job payload", "payload", string(jsonPayload))
	if err := s.cache.LoadBalancePublish(ctx, queues, jsonPayload); err != nil {
		slog.Warn("Failed to publish grading payload", "error", err)
		return fmt.Errorf("failed to publish grading payload: %s", err.Error())
	}
	slog.Info("manual grading task scheduled", "assignmentConfigID", assignmentConfigId, "submissionCount", len(gradingPayloads), "initiatedBy", req.InitiatedBy)
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
		gradingPayloads := make([]GradingPayload, 0, len(submissions.AssignmentConfig.Submissions))
		for _, submission := range submissions.AssignmentConfig.Submissions {
			gradingPayloads = append(gradingPayloads, GradingPayload{
				ID:            submission.ID,
				ExtractedPath: submission.ExtractedPath,
				CreatedAt:     submission.CreatedAt.Time(),
			})
		}
		// Build job payload
		jsonPayload, err := s.buildGradingJobPayload("gradingTask", gradingPayloads, payload.Payload.AssignmentConfigID, false, nil)
		if err != nil {
			return err
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
		slog.Info("sending job payload", "payload", string(jsonPayload))
		if err := s.cache.LoadBalancePublish(ctx, queues, jsonPayload); err != nil {
			slog.Warn("Failed to publish grading payload", "error", err)
			return fmt.Errorf("failed to publish grading payload: %s", err.Error())
		}
		slog.Info("grading task scheduled for batch processing", "assignmentConfigID", payload.Payload.AssignmentConfigID, "submissionCount", len(gradingPayloads), "stopCollectionAt", payload.Payload.StopCollectionAt)
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

		httpReq, err := http.NewRequest("POST", fmt.Sprintf("%s/v1/metadata", s.config.HasuraURL), strings.NewReader(string(jsonPayload)))
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
		slog.Info("scheduled grading event", "gradingID", newGrading.ID, "stopCollectionAt", newGrading.StopCollectionAt)
	}

	return nil
}

func (s *service) UpdateGraderQueues(ctx context.Context, queues []string) error {
	if err := s.cache.Put(ctx, cache.QueueKey, []byte(strings.Join(queues, ",")), 0); err != nil {
		slog.Warn("Failed to update grader queues", "error", err)
		return fmt.Errorf("failed to update grader queues: %s", err.Error())
	}
	slog.Info("updated grader queues", "queues", queues, "count", len(queues))
	return nil
}

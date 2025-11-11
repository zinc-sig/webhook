package trigger

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/xuri/excelize/v2"
	"github.com/zinc-sig/webhook/pkg/api"
	"github.com/zinc-sig/webhook/pkg/cache"
	"github.com/zinc-sig/webhook/pkg/repository"
	"go.uber.org/fx"
)

const (
	LoopbackAddress = "127.0.0.1"
	// Resource limits for file generation
	maxFileSize   = 100 * 1024 * 1024      // 100MB per file
	maxTotalSize  = 2 * 1024 * 1024 * 1024 // 2GB total archive
	maxFileErrors = 10                     // Maximum file errors before failing
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

// sanitizeZipPath sanitizes ITSC and upload name to prevent path traversal attacks
func sanitizeZipPath(itsc, uploadName string) (string, error) {
	// Remove any path separators and parent directory references
	cleanITSC := filepath.Base(filepath.Clean(itsc))
	cleanUpload := filepath.Base(filepath.Clean(uploadName))

	// Validate that cleaning didn't eliminate the entire path
	if cleanITSC == "." || cleanITSC == ".." || cleanITSC == "" {
		return "", fmt.Errorf("invalid ITSC: %s", itsc)
	}
	if cleanUpload == "." || cleanUpload == ".." || cleanUpload == "" {
		return "", fmt.Errorf("invalid upload name: %s", uploadName)
	}

	return filepath.Join(cleanITSC, cleanUpload), nil
}

// formatLateDuration formats a duration into human-readable format
func formatLateDuration(duration time.Duration) string {
	if duration < 0 {
		return "Not late"
	}

	hours := int(duration.Hours())
	if hours >= 24 {
		days := hours / 24
		remainingHours := hours % 24
		if remainingHours > 0 {
			return fmt.Sprintf("%dd %dh", days, remainingHours)
		}
		return fmt.Sprintf("%dd", days)
	}

	if hours > 0 {
		minutes := int(duration.Minutes()) % 60
		if minutes > 0 {
			return fmt.Sprintf("%dh %dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	}

	minutes := int(duration.Minutes())
	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}

	return "< 1m"
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
	e.POST("/validate/config", ValidateConfig(s))
	e.PUT("/grader/queues", UpdateGraderQueues(s))
	e.GET("/download/grades", DownloadGrades(s))
	e.GET("/download/submissions", DownloadSubmissions(s))
	e.GET("/download/submissions/:id", DownloadSubmission(s))
}

func (s *service) SyncEnrollment(ctx context.Context) error {
	courses := []string{"COMP1023", "COMP2011", "COMP2012", "COMP2211", "COMP2012H"}

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
		slog.Info("sending job payload", "payload", string(job))
		if err := s.cache.LoadBalanceGraderPublish(ctx, job, len(gradingPayloads)); err != nil {
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
		slog.Warn("Failed to unmarshal stdio test reports", "error", err)
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

	// Decompose submissions in a batch and send to grader one by one for load balancing
	for _, submission := range submissions {

		gradingPayloads := []GradingPayload{
			{
				ID:            submission.ID,
				ExtractedPath: submission.ExtractedPath,
				CreatedAt:     submission.CreatedAt.Time(),
			},
		}

		// Build job payload
		jsonPayload, err := s.buildGradingJobPayload("gradingTask", gradingPayloads, assignmentConfigId, false, &req.InitiatedBy)
		if err != nil {
			return err
		}
		slog.Info("sending job payload", "payload", string(jsonPayload))
		if err := s.cache.LoadBalanceGraderPublish(ctx, jsonPayload, len(gradingPayloads)); err != nil {
			slog.Warn("Failed to publish grading payload", "error", err)
			return fmt.Errorf("failed to publish grading payload: %s", err.Error())
		}

	}

	slog.Info("manual grading task scheduled", "assignmentConfigID", assignmentConfigId, "submissionCount", len(submissions), "initiatedBy", req.InitiatedBy)
	return nil
}

func (s *service) GradingTask(ctx context.Context, payload *GradingTaskRequest) error {
	submissions, err := s.repository.GetGradingSubmissions(ctx, payload.Payload.AssignmentConfigID)
	if err != nil {
		slog.Warn("Failed to get grading submissions", "assignmentConfigID", payload.Payload.AssignmentConfigID, "error", err)
		return fmt.Errorf("failed to get grading submissions: %s", err.Error())
	}

	if *submissions.AssignmentConfig.StopCollectionAt == payload.Payload.StopCollectionAt {
		// Decompose submissions in a batch and send to grader one by one for load balancing
		for _, submission := range submissions.AssignmentConfig.Submissions {

			gradingPayloads := []GradingPayload{
				{
					ID:            submission.ID,
					ExtractedPath: submission.ExtractedPath,
					CreatedAt:     submission.CreatedAt.Time(),
				},
			}

			// Build job payload
			jsonPayload, err := s.buildGradingJobPayload("gradingTask", gradingPayloads, payload.Payload.AssignmentConfigID, false, nil)
			if err != nil {
				return err
			}
			slog.Info("sending job payload", "payload", string(jsonPayload))
			if err := s.cache.LoadBalanceGraderPublish(ctx, jsonPayload, len(gradingPayloads)); err != nil {
				slog.Warn("Failed to publish grading payload", "error", err)
				return fmt.Errorf("failed to publish grading payload: %s", err.Error())
			}

		}

		slog.Info("grading task scheduled for processing", "assignmentConfigID", payload.Payload.AssignmentConfigID, "submissionCount", len(submissions.AssignmentConfig.Submissions), "stopCollectionAt", payload.Payload.StopCollectionAt)
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

		payload := map[string]any{
			"type": "create_scheduled_event",
			"args": map[string]any{
				"webhook":     webhookURL,
				"schedule_at": newGrading.StopCollectionAt,
				"payload": map[string]any{
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

func (s *service) GetSubmissionGrades(ctx context.Context, assignmentConfigID int) (*GradeResponse, error) {
	var resp GradeResponse
	if err := s.repository.GetSubmissionGrades(ctx, assignmentConfigID, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *service) GetSubmissionForDownload(ctx context.Context, submissionID int) (*SubmissionDownloadResponse, error) {
	var resp SubmissionDownloadResponse
	if err := s.repository.GetSubmissionByID(ctx, submissionID, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *service) GenerateSubmissionsZip(ctx context.Context, assignmentConfigID int) (*bytes.Buffer, string, error) {
	// Fetch all submissions data
	var submissionsData BatchSubmissionsResponse
	if err := s.repository.GetAllSubmissionsForAssignmentConfig(ctx, assignmentConfigID, &submissionsData); err != nil {
		return nil, "", err
	}

	// Validate we have submissions
	if len(submissionsData.AssignmentConfig.Submissions) == 0 {
		return nil, "", fmt.Errorf("no submissions found for assignment config %d", assignmentConfigID)
	}

	// Create a buffer to write the zip archive to
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	// Track statistics
	var totalSize int64
	filesProcessed := 0
	filesSkipped := 0
	errorCount := 0

	// Add each submission to the zip
	for _, submission := range submissionsData.AssignmentConfig.Submissions {
		// Check context cancellation
		select {
		case <-ctx.Done():
			zipWriter.Close()
			return nil, "", ctx.Err()
		default:
		}

		filePath := s.repository.GetSubmissionFilePath(submission.StoredName)

		// Get file info before reading
		fileInfo, err := os.Stat(filePath)
		if os.IsNotExist(err) {
			slog.Warn("Submission file not found, skipping",
				"storedName", submission.StoredName,
				"filePath", filePath)
			filesSkipped++
			errorCount++
			continue
		}
		if err != nil {
			slog.Warn("Failed to stat submission file",
				"storedName", submission.StoredName,
				"error", err)
			filesSkipped++
			errorCount++
			if errorCount > maxFileErrors {
				zipWriter.Close()
				return nil, "", fmt.Errorf("too many file errors (%d), aborting", errorCount)
			}
			continue
		}

		// Check file size limits
		if fileInfo.Size() > maxFileSize {
			slog.Warn("Submission file too large, skipping",
				"storedName", submission.StoredName,
				"size", fileInfo.Size(),
				"maxSize", maxFileSize)
			filesSkipped++
			errorCount++
			if errorCount > maxFileErrors {
				zipWriter.Close()
				return nil, "", fmt.Errorf("too many file errors (%d), aborting", errorCount)
			}
			continue
		}

		if totalSize+fileInfo.Size() > maxTotalSize {
			zipWriter.Close()
			slog.Error("Total archive size would exceed limit",
				"currentSize", totalSize,
				"maxSize", maxTotalSize,
				"filesProcessed", filesProcessed)
			return nil, "", fmt.Errorf("archive size limit exceeded: %d files processed", filesProcessed)
		}

		// Sanitize zip path
		zipPath, err := sanitizeZipPath(submission.User.ITSC, submission.UploadName)
		if err != nil {
			slog.Warn("Invalid path in submission, skipping",
				"storedName", submission.StoredName,
				"itsc", submission.User.ITSC,
				"uploadName", submission.UploadName,
				"error", err)
			filesSkipped++
			errorCount++
			if errorCount > maxFileErrors {
				zipWriter.Close()
				return nil, "", fmt.Errorf("too many file errors (%d), aborting", errorCount)
			}
			continue
		}

		// Create zip entry
		writer, err := zipWriter.Create(zipPath)
		if err != nil {
			zipWriter.Close()
			return nil, "", fmt.Errorf("failed to create zip entry for %s: %w",
				submission.StoredName, err)
		}

		// Stream file directly to zip without loading into memory
		file, err := os.Open(filePath)
		if err != nil {
			slog.Warn("Failed to open submission file",
				"storedName", submission.StoredName,
				"error", err)
			filesSkipped++
			errorCount++
			if errorCount > maxFileErrors {
				zipWriter.Close()
				return nil, "", fmt.Errorf("too many file errors (%d), aborting", errorCount)
			}
			continue
		}

		written, err := io.Copy(writer, file)
		file.Close()

		if err != nil {
			zipWriter.Close()
			return nil, "", fmt.Errorf("failed to write %s to zip: %w",
				submission.StoredName, err)
		}

		totalSize += written
		filesProcessed++
	}

	// Validate we processed at least some files
	if filesProcessed == 0 {
		zipWriter.Close()
		return nil, "", fmt.Errorf("no submission files could be added to archive (all %d submissions failed)",
			len(submissionsData.AssignmentConfig.Submissions))
	}

	// Close the zip writer to finalize the archive
	if err := zipWriter.Close(); err != nil {
		return nil, "", fmt.Errorf("failed to close zip writer: %w", err)
	}

	// Generate filename: {code}_{year}-{term}_submissions_latest.zip
	course := submissionsData.AssignmentConfig.Assignment.Course
	filename := fmt.Sprintf("%s_%d-%s_submissions_latest.zip",
		strings.ToLower(course.Code),
		course.Semester.Year,
		strings.ToLower(course.Semester.Term))

	slog.Info("Generated submissions zip",
		"assignmentConfigID", assignmentConfigID,
		"filesProcessed", filesProcessed,
		"filesSkipped", filesSkipped,
		"totalSize", totalSize,
		"filename", filename)

	return buf, filename, nil
}

func (s *service) GenerateGradesExcel(ctx context.Context, assignmentConfigID int, viewingTaskAssignedGroups string) (*excelize.File, error) {
	// Fetch grades data
	gradesData, err := s.GetSubmissionGrades(ctx, assignmentConfigID)
	if err != nil {
		return nil, err
	}

	// Validate we have submissions
	if len(gradesData.AssignmentConfig.Submissions) == 0 {
		return nil, fmt.Errorf("no submissions found for assignment config %d", assignmentConfigID)
	}

	// PASS 1: Discover all subgrade columns across ALL submissions
	subgradeColumns := make(map[string]string) // hash -> displayName
	var subgradeOrder []string

	for _, submission := range gradesData.AssignmentConfig.Submissions {
		if len(submission.Reports) > 0 && submission.Reports[0].Grade != nil {
			grade := submission.Reports[0].Grade
			if grade.Details != nil {
				for _, subReport := range grade.Details.Reports {
					if _, exists := subgradeColumns[subReport.Hash]; !exists {
						subgradeColumns[subReport.Hash] = subReport.DisplayName
						subgradeOrder = append(subgradeOrder, subReport.Hash)
					}
				}
			}
		}
	}

	// Create a new Excel file
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("Failed to close Excel file", "error", err)
		}
	}()

	// Set workbook properties
	if err := f.SetDocProps(&excelize.DocProperties{
		Creator: "Zinc by HKUST CSE Department",
		Created: time.Now().Format(time.RFC3339),
	}); err != nil {
		return nil, fmt.Errorf("failed to set document properties: %w", err)
	}

	// Create sheet name
	sheetName := "grades"
	if viewingTaskAssignedGroups != "" {
		sheetName = fmt.Sprintf("grades %s", viewingTaskAssignedGroups)
	}

	// Rename default sheet
	defaultSheet := f.GetSheetName(0)
	if err := f.SetSheetName(defaultSheet, sheetName); err != nil {
		return nil, fmt.Errorf("failed to rename sheet: %w", err)
	}

	// Define ALL columns upfront
	defaultColumns := []string{"ITSC", "Name", "Score", "Late Submission"}
	allColumns := make([]string, 0, len(defaultColumns)+len(subgradeOrder))
	allColumns = append(allColumns, defaultColumns...)
	for _, hash := range subgradeOrder {
		allColumns = append(allColumns, subgradeColumns[hash])
	}

	// Write ALL headers in one pass
	for i, col := range allColumns {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return nil, fmt.Errorf("failed to convert coordinates to cell name: %w", err)
		}
		if err := f.SetCellValue(sheetName, cell, col); err != nil {
			return nil, fmt.Errorf("failed to set header cell %s: %w", cell, err)
		}
	}

	// Set column widths for all columns
	columnWidths := map[int]float64{
		1: 16, // ITSC
		2: 32, // Name
		3: 16, // Score
		4: 16, // Late Submission
	}
	// All subgrade columns get width 16
	for i := len(defaultColumns); i < len(allColumns); i++ {
		columnWidths[i+1] = 16
	}

	for colIdx, width := range columnWidths {
		colName, err := excelize.ColumnNumberToName(colIdx)
		if err != nil {
			return nil, fmt.Errorf("failed to convert column number to name: %w", err)
		}
		if err := f.SetColWidth(sheetName, colName, colName, width); err != nil {
			return nil, fmt.Errorf("failed to set column width for %s: %w", colName, err)
		}
	}

	// PASS 2: Write data rows
	rowIdx := 2
	for _, submission := range gradesData.AssignmentConfig.Submissions {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		itsc := submission.User.ITSC
		name := submission.User.Name

		// Calculate late submission time with proper formatting
		var lateStr string
		if submission.IsLate && gradesData.AssignmentConfig.DueAt != nil {
			dueDate := gradesData.AssignmentConfig.DueAt.Time()
			submittedDate := submission.CreatedAt.Time()
			lateDuration := submittedDate.Sub(dueDate)
			lateStr = formatLateDuration(lateDuration)
		} else {
			lateStr = ""
		}

		// Build subgrade score map for this submission
		submissionSubgrades := make(map[string]float64)
		var scoreValue interface{} = "N/A"

		if len(submission.Reports) > 0 && submission.Reports[0].Grade != nil {
			grade := submission.Reports[0].Grade

			if grade.Details != nil {
				// Has detailed grades - use actual number
				scoreValue = grade.Details.AccScore

				// Collect subgrade scores
				for _, subReport := range grade.Details.Reports {
					submissionSubgrades[subReport.Hash] = subReport.Score
				}
			} else if grade.Score != nil {
				// Simple score without details - use actual number
				scoreValue = *grade.Score
			}
		}

		// Write default columns
		if err := f.SetCellValue(sheetName, fmt.Sprintf("A%d", rowIdx), itsc); err != nil {
			return nil, fmt.Errorf("failed to set ITSC cell for row %d: %w", rowIdx, err)
		}
		if err := f.SetCellValue(sheetName, fmt.Sprintf("B%d", rowIdx), name); err != nil {
			return nil, fmt.Errorf("failed to set name cell for row %d: %w", rowIdx, err)
		}
		if err := f.SetCellValue(sheetName, fmt.Sprintf("C%d", rowIdx), scoreValue); err != nil {
			return nil, fmt.Errorf("failed to set score cell for row %d: %w", rowIdx, err)
		}
		if err := f.SetCellValue(sheetName, fmt.Sprintf("D%d", rowIdx), lateStr); err != nil {
			return nil, fmt.Errorf("failed to set late cell for row %d: %w", rowIdx, err)
		}

		// Write subgrade columns in consistent order
		for i, hash := range subgradeOrder {
			colIdx := len(defaultColumns) + i + 1
			cell, err := excelize.CoordinatesToCellName(colIdx, rowIdx)
			if err != nil {
				return nil, fmt.Errorf("failed to convert coordinates to cell name: %w", err)
			}
			if score, exists := submissionSubgrades[hash]; exists {
				if err := f.SetCellValue(sheetName, cell, score); err != nil {
					return nil, fmt.Errorf("failed to set subgrade score cell %s: %w", cell, err)
				}
			} else {
				// Write empty string for missing subgrades
				if err := f.SetCellValue(sheetName, cell, ""); err != nil {
					return nil, fmt.Errorf("failed to set empty subgrade cell %s: %w", cell, err)
				}
			}
		}

		rowIdx++
	}

	slog.Info("Generated grades Excel file",
		"assignmentConfigID", assignmentConfigID,
		"submissionCount", len(gradesData.AssignmentConfig.Submissions),
		"totalColumns", len(allColumns),
		"subgradeColumns", len(subgradeOrder))

	return f, nil
}

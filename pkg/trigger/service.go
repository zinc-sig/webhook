package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/machinebox/graphql"
	"go.uber.org/fx"
)

type ServiceParams struct {
	fx.In
	GraphQLClient *graphql.Client
}

type service struct {
	client  *http.Client
	graphql *graphql.Client
}

func NewService(p ServiceParams) *service {
	return &service{
		client:  &http.Client{},
		graphql: p.GraphQLClient,
	}
}

func (s *service) RegisterRoutes(e *echo.Echo) {
	e.POST("/trigger/syncEnrollment", SyncEnrollment(s))
	e.POST("/trigger/decompression", DecompressSubmission(s))
	e.POST("/trigger/postGradingProcessing", PostGradingProcessing(s))
	e.POST("/trigger/scheduleGrading", ScheduleGrading(s))
	e.POST("/trigger/manualGradingTask/:assignmentConfigId", ManualGradingTask(s))
	e.POST("/trigger/gradingTask", GradingTask(s))
}

func (s *service) SyncEnrollment(ctx context.Context) error {
	apiURL := os.Getenv("ISO_API_URL")
	courses := []string{"COMP1023", "COMP2011", "COMP2012", "COMP2211"}

	for _, course := range courses {
		enrollmentMap, err := s.getStudentCourseEnrollmentMap(course, apiURL)
		if err != nil {
			return fmt.Errorf("failed to get enrollment map for course %s: %s", course, err.Error())
		}

		term, err := strconv.Atoi(enrollmentMap.Term)
		if err != nil {
			return fmt.Errorf("failed to parse term for course %s: %s", course, err.Error())
		}
		if err := s.createSemesterIfNotExist(ctx, term); err != nil {
			return fmt.Errorf("failed to create semester for course %s: %s", course, err.Error())
		}

		courseID, err := s.addCourse(enrollmentMap.CrseCode, term, enrollmentMap.Classes[0].CrseTitle)
		if err != nil {
			return fmt.Errorf("failed to add course %s: %s", course, err.Error())
		}

		var sectionNames []string
		for _, class := range enrollmentMap.Classes {
			if class.ClassType == "N" {
				sectionNames = append(sectionNames, class.Section)
			}
		}

		sections, err := s.addSections(courseID, sectionNames)
		if err != nil {
			return fmt.Errorf("failed to add sections for course %s: %s", course, err.Error())
		}

		if err := s.removeStudentsFromCourse(courseID); err != nil {
			return fmt.Errorf("failed to remove students from course %s: %s", course, err.Error())
		}

		if err := s.removeStudentsFromSection(courseID); err != nil {
			return fmt.Errorf("failed to remove students from section for course %s: %s", course, err.Error())
		}

		for _, class := range enrollmentMap.Classes {
			var itscIDs []string
			for _, student := range class.Students {
				if student.EnrollStatus == "Enrolled" {
					itscIDs = append(itscIDs, strings.Split(student.EmailAddr, "@")[0])
				}
			}

			studentUserIDs, err := s.getStudentUserIds(itscIDs)
			if err != nil {
				return fmt.Errorf("failed to get student user ids for course %s: %s", course, err.Error())
			}

			switch class.ClassType {
			case "N":
				sectionID := sections[class.Section]
				if err := s.addStudentsToCourseSection(studentUserIDs, sectionID); err != nil {
					return fmt.Errorf("failed to add students to course section for course %s: %s", course, err.Error())
				}
			case "E":
				if err := s.addStudentsToCourse(studentUserIDs, courseID); err != nil {
					return fmt.Errorf("failed to add students to course for course %s: %s", course, err.Error())
				}
			}
		}
	}

	return nil
}

func (s *service) getStudentCourseEnrollmentMap(courseCode string, apiURL string) (*EnrollmentMap, error) {
	// Get access token
	tokenURL := fmt.Sprintf("%s/oauth/token", apiURL)
	clientID := os.Getenv("ISO_API_CLIENT_ID")
	clientSecret := os.Getenv("ISO_API_CLIENT_SECRET")
	username := os.Getenv("ISO_API_USERNAME")
	password := os.Getenv("ISO_API_PASSWORD")

	data := fmt.Sprintf("grant_type=password&username=%s&password=%s", username, password)
	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	// Fetch enrollment map
	enrollmentURL := fmt.Sprintf("%s/sis/class_enrl?crseCode=%s", apiURL, courseCode)
	req, err = http.NewRequest("GET", enrollmentURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenResp.AccessToken))

	resp, err = client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var enrollmentMap EnrollmentMap
	if err := json.NewDecoder(resp.Body).Decode(&enrollmentMap); err != nil {
		return nil, err
	}

	return &enrollmentMap, nil
}

func (s *service) createSemesterIfNotExist(ctx context.Context, id int) error {
	name, year := getSemesterNameAndYear(fmt.Sprintf("%d", id))
	req := graphql.NewRequest(createSemester)
	req.Var("id", id)
	req.Var("name", name)
	req.Var("year", year)

	var resp struct{}
	return s.graphql.Run(ctx, req, &resp)
}

func (s *service) addCourse(code string, semesterID int, title string) (int, error) {
	req := graphql.NewRequest(addCourse)
	req.Var("code", code)
	req.Var("semesterId", semesterID)
	req.Var("name", title)

	var resp struct {
		CreateCourse struct {
			ID int `json:"id"`
		} `json:"createCourse"`
	}

	if err := s.graphql.Run(context.Background(), req, &resp); err != nil {
		return 0, err
	}

	return resp.CreateCourse.ID, nil
}

func (s *service) addSections(courseID int, sectionNames []string) (map[string]int, error) {
	var sections []map[string]interface{}
	for _, name := range sectionNames {
		sections = append(sections, map[string]interface{}{"name": name, "course_id": courseID})
	}

	req := graphql.NewRequest(addSections)
	req.Var("sections", sections)

	var resp struct {
		BatchCreateSection struct {
			Returning []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"returning"`
		} `json:"batchCreateSection"`
	}

	if err := s.graphql.Run(context.Background(), req, &resp); err != nil {
		return nil, err
	}

	sectionMap := make(map[string]int)
	for _, section := range resp.BatchCreateSection.Returning {
		sectionMap[section.Name] = section.ID
	}

	return sectionMap, nil
}

func (s *service) removeStudentsFromCourse(courseID int) error {
	req := graphql.NewRequest(removeStudentsFromCourse)
	req.Var("courseId", courseID)

	var resp struct{}
	return s.graphql.Run(context.Background(), req, &resp)
}

func (s *service) removeStudentsFromSection(courseID int) error {
	req := graphql.NewRequest(removeStudentsFromSection)
	req.Var("courseId", courseID)

	var resp struct{}
	return s.graphql.Run(context.Background(), req, &resp)
}

func (s *service) getStudentUserIds(itscIDs []string) ([]int, error) {
	req := graphql.NewRequest(getStudentUserIds)
	req.Var("itscIds", itscIDs)

	var resp struct {
		Users []struct {
			ID   int    `json:"id"`
			ITSC string `json:"itsc"`
		} `json:"users"`
	}

	if err := s.graphql.Run(context.Background(), req, &resp); err != nil {
		return nil, err
	}

	var userIDs []int
	var existingITSCs []string
	for _, user := range resp.Users {
		userIDs = append(userIDs, user.ID)
		existingITSCs = append(existingITSCs, user.ITSC)
	}

	var newITSCs []string
	for _, itscID := range itscIDs {
		found := false
		for _, existingITSC := range existingITSCs {
			if itscID == existingITSC {
				found = true
				break
			}
		}
		if !found {
			newITSCs = append(newITSCs, itscID)
		}
	}

	if len(newITSCs) > 0 {
		var users []map[string]interface{}
		for _, itsc := range newITSCs {
			users = append(users, map[string]interface{}{"itsc": itsc})
		}

		req := graphql.NewRequest(addUsers)
		req.Var("users", users)

		var addResp struct {
			BatchCreateUser struct {
				Returning []struct {
					ID int `json:"id"`
				} `json:"returning"`
			} `json:"batchCreateUser"`
		}

		if err := s.graphql.Run(context.Background(), req, &addResp); err != nil {
			return nil, err
		}

		for _, user := range addResp.BatchCreateUser.Returning {
			userIDs = append(userIDs, user.ID)
		}
	}

	return userIDs, nil
}

func (s *service) addStudentsToCourseSection(studentUserIDs []int, sectionID int) error {
	var users []map[string]interface{}
	for _, userID := range studentUserIDs {
		users = append(users, map[string]interface{}{"user_id": userID, "section_id": sectionID})
	}

	req := graphql.NewRequest(addStudentsToCourseSection)
	req.Var("users", users)

	var resp struct{}
	return s.graphql.Run(context.Background(), req, &resp)
}

func (s *service) addStudentsToCourse(studentUserIDs []int, courseID int) error {
	var users []map[string]interface{}
	for _, userID := range studentUserIDs {
		users = append(users, map[string]interface{}{"user_id": userID, "course_id": courseID, "permission": 1})
	}

	req := graphql.NewRequest(addStudentsToCourse)
	req.Var("users", users)

	var resp struct{}
	return s.graphql.Run(context.Background(), req, &resp)
}

func getSemesterNameAndYear(id string) (string, int) {
	// This is a simplified version of the original logic.
	// It might not cover all cases.
	seasonCode := id[len(id)-2:]
	yearSuffix := id[:len(id)-2]
	year, _ := strconv.Atoi(fmt.Sprintf("20%s", yearSuffix))

	switch seasonCode {
	case "20":
		return fmt.Sprintf("20%s-%d Winter", yearSuffix, year+1), year
	case "30":
		return fmt.Sprintf("20%s-%d Spring", yearSuffix, year+1), year + 1
	case "40":
		return fmt.Sprintf("20%s-%d Summer", yearSuffix, year+1), year + 1
	default:
		return fmt.Sprintf("20%s-%d Fall", yearSuffix, year+1), year
	}
}

func (s *service) DecompressSubmission(ctx context.Context, payload json.RawMessage) error {
	var submission SubmissionRow
	if err := json.Unmarshal(payload, &submission); err != nil {
		return fmt.Errorf("failed to unmarshal submission data: %s", err.Error())
	}
	if !strings.HasSuffix(submission.UploadName, ".zip") {
		return s.updateExtractedSubmissionEntry(ctx, submission.ID, "", "Unsupported archive format")
	}

	if err := extractZip(submission.ID, submission.StoredName); err != nil {
		return s.updateExtractedSubmissionEntry(ctx, submission.ID, "", err.Error())
	}

	return s.updateExtractedSubmissionEntry(ctx, submission.ID, fmt.Sprintf("extracted/%d", submission.ID), "")
}

func (s *service) PostGradingProcessing(ctx context.Context, payload json.RawMessage) error {
	var report ReportRow
	if err := json.Unmarshal(payload, &report); err != nil {
		return fmt.Errorf("failed to unmarshal report data: %s", err.Error())
	}
	var pipelineResults PipelineResults
	if err := json.Unmarshal(report.PipelineResults, &pipelineResults); err != nil {
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

	graphqlReq := graphql.NewRequest(addReportArtifacts)
	graphqlReq.Var("id", report.ID)
	graphqlReq.Var("sanitizedReports", censoredReports)
	graphqlReq.Var("grade", grade)

	var resp struct{}
	if err := s.graphql.Run(ctx, graphqlReq, &resp); err != nil {
		return fmt.Errorf("failed to update report artifacts: %s", err.Error())
	}
	return nil
}

func (s *service) updateExtractedSubmissionEntry(ctx context.Context, id int, extractedPath, failReason string) error {
	req := graphql.NewRequest(updateDecompressionResultForSubmission)
	req.Var("id", id)
	if extractedPath != "" {
		req.Var("extractedPath", extractedPath)
	} else {
		req.Var("extractedPath", nil)
	}
	if failReason != "" {
		req.Var("failReason", failReason)
	} else {
		req.Var("failReason", nil)
	}

	var resp struct{}
	return s.graphql.Run(ctx, req, &resp)
}

func extractZip(submissionID int, storedName string) error {
	mountPath := os.Getenv("SHARED_MOUNT_PATH")
	if mountPath == "" {
		mountPath = "/home/system/workspace"
	}

	file := fmt.Sprintf("%s/%s", mountPath, storedName)
	extractToPath := fmt.Sprintf("%s/extracted/%d", mountPath, submissionID)
	temporaryResolvePath := fmt.Sprintf("/tmp/%d", submissionID)

	if err := os.MkdirAll(temporaryResolvePath, os.ModePerm); err != nil {
		return err
	}

	cmd := exec.Command("unzip", file, "-d", temporaryResolvePath)
	if err := cmd.Run(); err != nil {
		return err
	}

	files, err := os.ReadDir(temporaryResolvePath)
	if err != nil {
		return err
	}

	if len(files) >= 1 {
		sourcePath := temporaryResolvePath
		if len(files) == 1 && files[0].IsDir() {
			sourcePath = fmt.Sprintf("%s/%s", temporaryResolvePath, files[0].Name())
		}
		if err := os.Rename(sourcePath, extractToPath); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("empty directory")
	}

	return nil
}

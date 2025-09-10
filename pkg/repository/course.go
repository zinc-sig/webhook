package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/machinebox/graphql"
)

type EnrollmentMap struct {
	Term     string `json:"term"`
	CrseCode string `json:"crseCode"`
	Classes  []struct {
		CrseTitle string `json:"crseTitle"`
		Section   string `json:"section"`
		ClassType string `json:"classType"`
		Students  []struct {
			EmailAddr    string `json:"emailAddr"`
			EnrollStatus string `json:"enrollStatus"`
		} `json:"students"`
	} `json:"classes"`
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

func (r *repository) GetStudentCourseEnrollmentMap(courseCode string) (*EnrollmentMap, error) {
	// Get access token
	tokenURL := fmt.Sprintf(
		"%s/oauth/token",
		r.isoConfig.ApiURL,
	)

	data := fmt.Sprintf(
		"grant_type=password&username=%s&password=%s",
		r.isoConfig.Username,
		r.isoConfig.Password,
	)
	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(
		r.isoConfig.ClientID,
		r.isoConfig.ClientSecret,
	)

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
	enrollmentURL := fmt.Sprintf(
		"%s/sis/class_enrl?crseCode=%s", r.isoConfig.ApiURL, courseCode)
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

func (r *repository) AddCourse(ctx context.Context, code string, semesterID int, title string) (int, error) {
	req := graphql.NewRequest(addCourse)
	req.Var("code", code)
	req.Var("semesterId", semesterID)
	req.Var("name", title)

	var resp struct {
		CreateCourse struct {
			ID int `json:"id"`
		} `json:"createCourse"`
	}

	if err := r.client.Run(ctx, req, &resp); err != nil {
		return 0, err
	}

	return resp.CreateCourse.ID, nil
}

func (r *repository) AddSections(ctx context.Context, courseID int, sectionNames []string) (map[string]int, error) {
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

	if err := r.client.Run(ctx, req, &resp); err != nil {
		m, _ := json.Marshal(resp)
		r.logger.Errorw("error adding sections", "context", ctx, "response", string(m))
		return nil, fmt.Errorf("failed to add sections: %w", err)
	}

	sectionMap := make(map[string]int)
	for _, section := range resp.BatchCreateSection.Returning {
		sectionMap[section.Name] = section.ID
	}

	return sectionMap, nil
}

func (r *repository) AddStudentsToCourseSection(ctx context.Context, studentUserIDs []int, sectionID int) error {
	var users []map[string]interface{}
	for _, userID := range studentUserIDs {
		users = append(users, map[string]interface{}{"user_id": userID, "section_id": sectionID})
	}

	req := graphql.NewRequest(addStudentsToCourseSection)
	req.Var("users", users)

	var resp map[string]interface{}
	if err := r.client.Run(ctx, req, &resp); err != nil {
		m, _ := json.Marshal(resp)
		r.logger.Errorw("error adding students to course section", "context", ctx, "response", string(m))
		return fmt.Errorf("failed to add students to course section: %w", err)
	}
	return nil
}

func (r *repository) AddStudentsToCourse(ctx context.Context, studentUserIDs []int, courseID int) error {
	var users []map[string]interface{}
	for _, userID := range studentUserIDs {
		users = append(users, map[string]interface{}{"user_id": userID, "course_id": courseID, "permission": 1})
	}

	req := graphql.NewRequest(addStudentsToCourse)
	req.Var("users", users)

	var resp map[string]interface{}
	if err := r.client.Run(ctx, req, &resp); err != nil {
		m, _ := json.Marshal(resp)
		r.logger.Errorw("error adding students to course", "context", ctx, "response", string(m))
		return fmt.Errorf("failed to add students to course: %w", err)
	}
	return nil
}

func (r *repository) RemoveStudentsFromSection(ctx context.Context, courseID int) error {
	req := graphql.NewRequest(removeStudentsFromSection)
	req.Var("courseId", courseID)

	var resp map[string]interface{}
	if err := r.client.Run(ctx, req, &resp); err != nil {
		m, _ := json.Marshal(resp)
		r.logger.Errorw("error removing students from section", "context", ctx, "response", string(m))
		return fmt.Errorf("failed to remove students from section: %w", err)
	}
	return nil
}

func (r *repository) RemoveStudentsFromCourse(ctx context.Context, courseID int) error {
	req := graphql.NewRequest(removeStudentsFromCourse)
	req.Var("courseId", courseID)

	var resp map[string]interface{}
	if err := r.client.Run(ctx, req, &resp); err != nil {
		m, _ := json.Marshal(resp)
		r.logger.Errorw("error removing students from course", "context", ctx, "response", string(m))
		return fmt.Errorf("failed to remove students from course: %w", err)
	}
	return nil
}

func (r *repository) CreateSemesterIfNotExist(ctx context.Context, id int) error {
	name, year := getSemesterNameAndYear(fmt.Sprintf("%d", id))
	req := graphql.NewRequest(createSemester)
	req.Var("id", id)
	req.Var("name", name)
	req.Var("year", year)

	var resp map[string]interface{}
	if err := r.client.Run(ctx, req, &resp); err != nil {
		m, _ := json.Marshal(resp)
		r.logger.Errorw("error creating semester", "context", ctx, "response", string(m))
		return fmt.Errorf("failed to create semester: %w", err)
	}
	return nil
}

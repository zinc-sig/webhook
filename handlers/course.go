package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/machinebox/graphql"
)

const createSemester = `
mutation createSemester($id: bigint!, $name: String!, $year: Int!) {
  createSemester(object: {id: $id, name: $name, year: $year}, on_conflict: {constraint: semesters_pkey, update_columns: [updatedAt]}) {
    createdAt
    updatedAt
  }
}`

const addCourse = `
mutation addCourse($code: String!, $semesterId: bigint!, $name: String!) {
  createCourse(object: {code: $code, semester_id: $semesterId, name: $name}, on_conflict: {constraint: courses_code_semester_id_key, update_columns: updated_at}) {
    id
    semester {
      name
    }
  }
}`

const addSections = `
mutation batchCreateSection($sections: [sections_insert_input!]!) {
  batchCreateSection(objects: $sections, on_conflict: {constraint: sections_course_id_name_key, update_columns: updated_at}) {
    returning {
      id
      name
      course {
        code
      }
    }
  }
}`

const addStudentsToCourseSection = `
mutation addUsersToSection($users: [section_user_insert_input!]!) {
  addUsersToSection(objects: $users) {
    affected_rows
  }
}`

const addStudentsToCourse = `
mutation enrollUsersToCourse($users: [course_user_insert_input!]!) {
  enrollUsersInCourse(objects: $users) {
    affected_rows
  }
}`

const getStudentUserIds = `
query getUserIds($itscIds: [String!]!) {
  users(where: {itsc: {_in: $itscIds}}) {
    id
    itsc
  }
}`

const addUsers = `
mutation registerUsers($users: [users_insert_input!]!) {
  batchCreateUser(objects: $users) {
    returning {
      id
      itsc
    }
    affected_rows
  }
}`

const removeStudentsFromCourse = `
mutation removeStudentsFromCourse($courseId: bigint!) {
  removeUsersFromCourse(where: {course_id: {_eq: $courseId}, permission: {_eq: 1}}) {
    affected_rows
  }
}`

const removeStudentsFromSection = `
mutation removeStudentsFromSection($courseId: bigint!) {
  removeStudentsFromSection(where: {section: {course_id: {_eq: $courseId}}}) {
    affected_rows
  }
}`

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

func (h *Handler) SyncEnrollment(c echo.Context, apiURL string) error {
	courses := []string{"COMP1023", "COMP2011", "COMP2012", "COMP2211"}

	for _, course := range courses {
		enrollmentMap, err := h.getStudentCourseEnrollmentMap(course, apiURL)
		if err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to get enrollment map for course %s: %s", course, err.Error()))
		}

		term, err := strconv.Atoi(enrollmentMap.Term)
		if err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to parse term for course %s: %s", course, err.Error()))
		}
		if err := h.createSemesterIfNotExist(term); err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to create semester for course %s: %s", course, err.Error()))
		}

		courseID, err := h.addCourse(enrollmentMap.CrseCode, term, enrollmentMap.Classes[0].CrseTitle)
		if err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to add course %s: %s", course, err.Error()))
		}

		var sectionNames []string
		for _, class := range enrollmentMap.Classes {
			if class.ClassType == "N" {
				sectionNames = append(sectionNames, class.Section)
			}
		}

		sections, err := h.addSections(courseID, sectionNames)
		if err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to add sections for course %s: %s", course, err.Error()))
		}

		if err := h.removeStudentsFromCourse(courseID); err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to remove students from course %s: %s", course, err.Error()))
		}

		if err := h.removeStudentsFromSection(courseID); err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to remove students from section for course %s: %s", course, err.Error()))
		}

		for _, class := range enrollmentMap.Classes {
			var itscIDs []string
			for _, student := range class.Students {
				if student.EnrollStatus == "Enrolled" {
					itscIDs = append(itscIDs, strings.Split(student.EmailAddr, "@")[0])
				}
			}

			studentUserIDs, err := h.getStudentUserIds(itscIDs)
			if err != nil {
				return c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to get student user ids for course %s: %s", course, err.Error()))
			}

			switch class.ClassType {
			case "N":
				sectionID := sections[class.Section]
				if err := h.addStudentsToCourseSection(studentUserIDs, sectionID); err != nil {
					return c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to add students to course section for course %s: %s", course, err.Error()))
				}
			case "E":
				if err := h.addStudentsToCourse(studentUserIDs, courseID); err != nil {
					return c.String(http.StatusInternalServerError, fmt.Sprintf("Failed to add students to course for course %s: %s", course, err.Error()))
				}
			}
		}
	}

	return c.String(http.StatusOK, "SyncEnrollment endpoint")
}

func (h *Handler) getStudentCourseEnrollmentMap(courseCode string, apiURL string) (*EnrollmentMap, error) {
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

func (h *Handler) createSemesterIfNotExist(id int) error {
	name, year := getSemesterNameAndYear(fmt.Sprintf("%d", id))
	req := graphql.NewRequest(createSemester)
	req.Var("id", id)
	req.Var("name", name)
	req.Var("year", year)

	var resp struct{}
	return h.GraphQLClient.Run(context.Background(), req, &resp)
}

func (h *Handler) addCourse(code string, semesterID int, title string) (int, error) {
	req := graphql.NewRequest(addCourse)
	req.Var("code", code)
	req.Var("semesterId", semesterID)
	req.Var("name", title)

	var resp struct {
		CreateCourse struct {
			ID int `json:"id"`
		} `json:"createCourse"`
	}

	if err := h.GraphQLClient.Run(context.Background(), req, &resp); err != nil {
		return 0, err
	}

	return resp.CreateCourse.ID, nil
}

func (h *Handler) addSections(courseID int, sectionNames []string) (map[string]int, error) {
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

	if err := h.GraphQLClient.Run(context.Background(), req, &resp); err != nil {
		return nil, err
	}

	sectionMap := make(map[string]int)
	for _, section := range resp.BatchCreateSection.Returning {
		sectionMap[section.Name] = section.ID
	}

	return sectionMap, nil
}

func (h *Handler) removeStudentsFromCourse(courseID int) error {
	req := graphql.NewRequest(removeStudentsFromCourse)
	req.Var("courseId", courseID)

	var resp struct{}
	return h.GraphQLClient.Run(context.Background(), req, &resp)
}

func (h *Handler) removeStudentsFromSection(courseID int) error {
	req := graphql.NewRequest(removeStudentsFromSection)
	req.Var("courseId", courseID)

	var resp struct{}
	return h.GraphQLClient.Run(context.Background(), req, &resp)
}

func (h *Handler) getStudentUserIds(itscIDs []string) ([]int, error) {
	req := graphql.NewRequest(getStudentUserIds)
	req.Var("itscIds", itscIDs)

	var resp struct {
		Users []struct {
			ID   int    `json:"id"`
			ITSC string `json:"itsc"`
		} `json:"users"`
	}

	if err := h.GraphQLClient.Run(context.Background(), req, &resp); err != nil {
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

		if err := h.GraphQLClient.Run(context.Background(), req, &addResp); err != nil {
			return nil, err
		}

		for _, user := range addResp.BatchCreateUser.Returning {
			userIDs = append(userIDs, user.ID)
		}
	}

	return userIDs, nil
}

func (h *Handler) addStudentsToCourseSection(studentUserIDs []int, sectionID int) error {
	var users []map[string]interface{}
	for _, userID := range studentUserIDs {
		users = append(users, map[string]interface{}{"user_id": userID, "section_id": sectionID})
	}

	req := graphql.NewRequest(addStudentsToCourseSection)
	req.Var("users", users)

	var resp struct{}
	return h.GraphQLClient.Run(context.Background(), req, &resp)
}

func (h *Handler) addStudentsToCourse(studentUserIDs []int, courseID int) error {
	var users []map[string]interface{}
	for _, userID := range studentUserIDs {
		users = append(users, map[string]interface{}{"user_id": userID, "course_id": courseID, "permission": 1})
	}

	req := graphql.NewRequest(addStudentsToCourse)
	req.Var("users", users)

	var resp struct{}
	return h.GraphQLClient.Run(context.Background(), req, &resp)
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

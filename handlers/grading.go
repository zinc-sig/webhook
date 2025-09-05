package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/machinebox/graphql"
)

const getGradingSubmissions = `
query getGradingSubmissions($assignmentConfigId: bigint!) {
  assignmentConfig: assignment_configs_by_pk(id: $assignmentConfigId) {
    stopCollectionAt: stop_collection_at
    submissions(distinct_on: user_id, order_by: {user_id: asc, created_at: desc}) {
      id
      extracted_path
      created_at
    }
  }
}`

const getLatestSubmissionsForAssignmentConfig = `
query getLatestSubmissionsForAssignmentConfig($assignmentConfigId: bigint!) {
  assignmentConfig: assignment_configs_by_pk(id: $assignmentConfigId) {
    submissions(distinct_on: user_id, order_by: {user_id: asc, created_at: desc}) {
      id
      extracted_path
      created_at
    }
  }
}`

const getSelectedSubmissions = `
query getSelectedSubmissions($submissions: [bigint!]) {
  submissions(where: {id: {_in: $submissions}}) {
    id
    extracted_path
    created_at
  }
}`

const addReportArtifacts = `
mutation addReportArtifacts($id: bigint!, $sanitizedReports: jsonb!, $grade: jsonb!) {
  update_reports_by_pk(pk_columns: {id: $id}, _set: {sanitized_pipeline_results: $sanitizedReports, grade: $grade}) {
    id
  }
}`

type PostGradingProcessingRequest struct {
	Event struct {
		Data struct {
			New struct {
				ID              int             `json:"id"`
				PipelineResults json.RawMessage `json:"pipeline_results"`
				IsFinal         bool            `json:"is_final"`
			} `json:"new"`
		} `json:"data"`
	} `json:"event"`
}

func (h *Handler) PostGradingProcessing(c echo.Context) error {
	var req PostGradingProcessingRequest
	if err := c.Bind(&req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid request body")
	}

	var pipelineResults struct {
		StageReports map[string]json.RawMessage `json:"stageReports"`
		ScoreReports json.RawMessage            `json:"scoreReports"`
	}
	if err := json.Unmarshal(req.Event.Data.New.PipelineResults, &pipelineResults); err != nil {
		return c.String(http.StatusInternalServerError, "Failed to parse pipeline results")
	}

	censoredReports := make(map[string]interface{})
	var grade map[string]interface{}

	for stage, stageReport := range pipelineResults.StageReports {
		switch stage {
		case "valgrind":
			var valgrindReports []struct {
				Visibility string   `json:"visibility"`
				IsCorrect  bool     `json:"isCorrect"`
				Stdout     []string `json:"stdout"`
				Errors     []string `json:"errors"`
			}
			if err := json.Unmarshal(stageReport, &valgrindReports); err == nil {
				for i, r := range valgrindReports {
					switch r.Visibility {
					case "ALWAYS_HIDDEN":
						valgrindReports[i].Stdout = []string{}
						valgrindReports[i].Errors = []string{}
					case "VISIBLE_AFTER_GRADING":
						if !req.Event.Data.New.IsFinal {
							valgrindReports[i].Stdout = []string{}
							valgrindReports[i].Errors = []string{}
						}
					case "VISIBLE_AFTER_GRADING_IF_FAILED":
						if !req.Event.Data.New.IsFinal || r.IsCorrect {
							valgrindReports[i].Stdout = []string{}
							valgrindReports[i].Errors = []string{}
						}
					}
				}
				censoredReports[stage] = valgrindReports
			}
		case "stdioTest":
			var stdioTestReports []struct {
				Visibility string   `json:"visibility"`
				IsCorrect  bool     `json:"isCorrect"`
				Stdout     []string `json:"stdout"`
				Expect     []string `json:"expect"`
				Diff       []string `json:"diff"`
			}
			if err := json.Unmarshal(stageReport, &stdioTestReports); err == nil {
				for i, r := range stdioTestReports {
					switch r.Visibility {
					case "ALWAYS_HIDDEN":
						stdioTestReports[i].Stdout = []string{}
						stdioTestReports[i].Expect = []string{}
						stdioTestReports[i].Diff = []string{}
					case "VISIBLE_AFTER_GRADING":
						if !req.Event.Data.New.IsFinal {
							stdioTestReports[i].Expect = []string{}
							stdioTestReports[i].Diff = []string{}
						}
					case "VISIBLE_AFTER_GRADING_IF_FAILED":
						if !req.Event.Data.New.IsFinal || r.IsCorrect {
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
	graphqlReq.Var("id", req.Event.Data.New.ID)
	graphqlReq.Var("sanitizedReports", censoredReports)
	graphqlReq.Var("grade", grade)

	var resp struct{}
	if err := h.GraphQLClient.Run(context.Background(), graphqlReq, &resp); err != nil {
		return c.String(http.StatusInternalServerError, "Failed to add report artifacts")
	}

	return c.String(http.StatusOK, "PostGradingProcessing endpoint")
}

type ScheduleGradingRequest struct {
	Event struct {
		Op   string `json:"op"`
		Data struct {
			Old struct {
				StopCollectionAt string `json:"stop_collection_at"`
			} `json:"old"`
			New struct {
				ID               int    `json:"id"`
				StopCollectionAt string `json:"stop_collection_at"`
			} `json:"new"`
		} `json:"data"`
	} `json:"event"`
}

func (h *Handler) ScheduleGrading(c echo.Context, hasuraURL string) error {
	var req ScheduleGradingRequest
	if err := c.Bind(&req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid request body")
	}

	if (req.Event.Op == "UPDATE" && req.Event.Data.Old.StopCollectionAt != req.Event.Data.New.StopCollectionAt) || req.Event.Op == "INSERT" {
		webhookURL := fmt.Sprintf("http://%s/trigger/gradingTask", os.Getenv("WEBHOOK_ADDR"))

		payload := map[string]interface{}{
			"type": "create_scheduled_event",
			"args": map[string]interface{}{
				"webhook":     webhookURL,
				"schedule_at": req.Event.Data.New.StopCollectionAt,
				"payload": map[string]interface{}{
					"assignment_config_id": req.Event.Data.New.ID,
					"stop_collection_at":   req.Event.Data.New.StopCollectionAt,
				},
			},
		}

		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			return c.String(http.StatusInternalServerError, "Failed to marshal payload")
		}

		httpReq, err := http.NewRequest("POST", fmt.Sprintf("%s/query", hasuraURL), strings.NewReader(string(jsonPayload)))
		if err != nil {
			return c.String(http.StatusInternalServerError, "Failed to create request")
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-Hasura-Admin-Secret", os.Getenv("HASURA_ADMIN_SECRET"))

		client := &http.Client{}
		resp, err := client.Do(httpReq)
		if err != nil {
			return c.String(http.StatusInternalServerError, "Failed to send request to Hasura")
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return c.String(http.StatusInternalServerError, "Failed to schedule grading event")
		}
	}

	return c.String(http.StatusOK, "ScheduleGrading endpoint")
}

type ManualGradingTaskRequest struct {
	Submissions []int `json:"submissions"`
	InitiatedBy int   `json:"initiatedBy"`
}

func (h *Handler) ManualGradingTask(c echo.Context) error {
	var req ManualGradingTaskRequest
	if err := c.Bind(&req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid request body")
	}

	assignmentConfigId, err := strconv.Atoi(c.Param("assignmentConfigId"))
	if err != nil {
		return c.String(http.StatusBadRequest, "Invalid assignmentConfigId")
	}

	// Get submissions from database
	query := getSelectedSubmissions
	variables := map[string]interface{}{
		"submissions": req.Submissions,
	}
	if len(req.Submissions) == 0 {
		query = getLatestSubmissionsForAssignmentConfig
		variables = map[string]interface{}{
			"assignmentConfigId": assignmentConfigId,
		}
	}

	graphqlReq := graphql.NewRequest(query)
	for key, value := range variables {
		graphqlReq.Var(key, value)
	}

	var graphqlResp struct {
		Submissions []struct {
			ID            int       `json:"id"`
			ExtractedPath string    `json:"extracted_path"`
			CreatedAt     time.Time `json:"created_at"`
		} `json:"submissions"`
	}

	if len(req.Submissions) == 0 {
		var resp struct {
			AssignmentConfig struct {
				Submissions []struct {
					ID            int       `json:"id"`
					ExtractedPath string    `json:"extracted_path"`
					CreatedAt     time.Time `json:"created_at"`
				} `json:"submissions"`
			} `json:"assignmentConfig"`
		}
		if err := h.GraphQLClient.Run(context.Background(), graphqlReq, &resp); err != nil {
			return c.String(http.StatusInternalServerError, "Failed to get submissions")
		}
		graphqlResp.Submissions = resp.AssignmentConfig.Submissions
	} else {
		if err := h.GraphQLClient.Run(context.Background(), graphqlReq, &graphqlResp); err != nil {
			return c.String(http.StatusInternalServerError, "Failed to get submissions")
		}
	}

	// Push job to redis
	payload := map[string]interface{}{
		"submissions":          graphqlResp.Submissions,
		"assignment_config_id": assignmentConfigId,
		"isTest":               false,
		"initiatedBy":          req.InitiatedBy,
	}

	jsonPayload, err := json.Marshal(map[string]interface{}{
		"job":     "manualGradingTask",
		"payload": payload,
	})
	if err != nil {
		return c.String(http.StatusInternalServerError, "Failed to marshal payload")
	}

	if err := h.RedisClient.RPush(context.Background(), "zinc_queue:grader", jsonPayload).Err(); err != nil {
		return c.String(http.StatusInternalServerError, "Failed to push job to redis")
	}

	return c.String(http.StatusOK, "ManualGradingTask endpoint")
}

type GradingTaskRequest struct {
	Payload struct {
		AssignmentConfigID int    `json:"assignment_config_id"`
		StopCollectionAt   string `json:"stop_collection_at"`
	} `json:"payload"`
}

func (h *Handler) GradingTask(c echo.Context) error {
	var req GradingTaskRequest
	if err := c.Bind(&req); err != nil {
		return c.String(http.StatusBadRequest, "Invalid request body")
	}

	// Get submissions from database
	graphqlReq := graphql.NewRequest(getGradingSubmissions)
	graphqlReq.Var("assignmentConfigId", req.Payload.AssignmentConfigID)

	var graphqlResp struct {
		AssignmentConfig struct {
			StopCollectionAt string `json:"stopCollectionAt"`
			Submissions      []struct {
				ID            int       `json:"id"`
				ExtractedPath string    `json:"extracted_path"`
				CreatedAt     time.Time `json:"created_at"`
			} `json:"submissions"`
		} `json:"assignmentConfig"`
	}

	if err := h.GraphQLClient.Run(context.Background(), graphqlReq, &graphqlResp); err != nil {
		return c.String(http.StatusInternalServerError, "Failed to get submissions")
	}

	if graphqlResp.AssignmentConfig.StopCollectionAt == req.Payload.StopCollectionAt {
		// Push job to redis
		payload := map[string]interface{}{
			"submissions":          graphqlResp.AssignmentConfig.Submissions,
			"assignment_config_id": req.Payload.AssignmentConfigID,
			"isTest":               false,
		}

		jsonPayload, err := json.Marshal(map[string]interface{}{
			"job":     "gradingTask",
			"payload": payload,
		})
		if err != nil {
			return c.String(http.StatusInternalServerError, "Failed to marshal payload")
		}

		if err := h.RedisClient.RPush(context.Background(), "zinc_queue:grader", jsonPayload).Err(); err != nil {
			return c.String(http.StatusInternalServerError, "Failed to push job to redis")
		}
	}

	return c.String(http.StatusOK, "GradingTask endpoint")
}

package trigger

import "encoding/json"

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

type RowTriggerPayload struct {
	Event RowTriggerEvent `json:"event"`
}

type RowTriggerEvent struct {
	Data RowSnapshot `json:"data"`
}

type RowSnapshot struct {
	New json.RawMessage `json:"new"`
	Old json.RawMessage `json:"old"`
}

type SubmissionRow struct {
	ID         int    `json:"id"`
	UploadName string `json:"upload_name"`
	StoredName string `json:"stored_name"`
}

type ReportRow struct {
	ID              int             `json:"id"`
	PipelineResults json.RawMessage `json:"pipeline_results"`
	IsFinal         bool            `json:"is_final"`
}

type PipelineResults struct {
	StageReports map[string]json.RawMessage `json:"stageReports"`
	ScoreReports json.RawMessage            `json:"scoreReports"`
}

type StdioTestReport struct {
	Visibility string   `json:"visibility"`
	IsCorrect  bool     `json:"isCorrect"`
	Stdout     []string `json:"stdout"`
	Expect     []string `json:"expect"`
	Diff       []string `json:"diff"`
}

type ValgrindReport struct {
	Visibility string   `json:"visibility"`
	IsCorrect  bool     `json:"isCorrect"`
	Stdout     []string `json:"stdout"`
	Errors     []string `json:"errors"`
}

type ManualGradingTaskRequest struct {
	Submissions []int `json:"submissions"`
	InitiatedBy int   `json:"initiatedBy"`
}

type GradingTaskRequest struct {
	Payload struct {
		AssignmentConfigID int    `json:"assignment_config_id"`
		StopCollectionAt   string `json:"stop_collection_at"`
	} `json:"payload"`
}

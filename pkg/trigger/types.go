package trigger

import (
	"encoding/json"

	"github.com/zinc-sig/webhook/pkg/repository"
)

type RowTriggerPayload struct {
	Event RowTriggerEvent `json:"event"`
}

type RowTriggerEvent struct {
	Op   string      `json:"op"`
	Data RowSnapshot `json:"data"`
}

type RowSnapshot struct {
	New json.RawMessage `json:"new"`
	Old json.RawMessage `json:"old"`
}

type SubmissionRow struct {
	ID                 int                        `json:"id"`
	UploadName         string                     `json:"upload_name"`
	StoredName         string                     `json:"stored_name"`
	AssignmentConfigID int                        `json:"assignment_config_id"`
	UserID             int                        `json:"user_id"`
	CreatedAt          repository.TimeWithoutZone `json:"created_at"`
}

type ReportRow struct {
	ID              int             `json:"id"`
	PipelineResults PipelineResults `json:"pipeline_results"`
	IsFinal         bool            `json:"is_final"`
}

type GradingRow struct {
	ID               int    `json:"id"`
	StopCollectionAt string `json:"stop_collection_at"`
}

type PipelineResults struct {
	StageReports map[string]json.RawMessage `json:"stageReports"`
	ScoreReports json.RawMessage            `json:"scoreReports"`
}

type StdioTestReport struct {
	ID         int      `json:"id"`
	Args       []string `json:"args"`
	Visibility string   `json:"visibility"`
	IsCorrect  bool     `json:"isCorrect"`
	IsSuccess  bool     `json:"isSuccess"`
	File       string   `json:"file"`
	Score      *struct {
		Score float64 `json:"score"`
		Total float64 `json:"total"`
	} `json:"score"`
	StdIn        []string `json:"stdin"`
	Stdout       []string `json:"stdout"`
	Stderr       []string `json:"stderr"`
	DiffStderr   []string `json:"diffStderr"`
	Expect       []string `json:"expect"`
	Diff         []string `json:"diff"`
	DiffExitCode *int     `json:"diffExitCode"`
	ExeExitCode  int      `json:"exeExitCode"`
	HasTimedOut  bool     `json:"hasTimedOut"`
	ExitCode     int      `json:"exitCode"`
}

type ValgrindReportError struct {
	Kind string `json:"kind"`
	What []struct {
		WhatText string `json:"whatText"`
		Stack    []struct {
			IP   string  `json:"ip"`
			Obj  *string `json:"obj"`
			Func *string `json:"fn"`
			Dir  *string `json:"dir"`
			File *string `json:"file"`
			Line *int    `json:"line"`
		}
		Aux bool `json:"aux"`
	} `json:"what"`
}

type ValgrindReport struct {
	ID           int                   `json:"id"`
	Executable   string                `json:"executable"`
	Args         []string              `json:"args"`
	Visibility   string                `json:"visibility"`
	IsCorrect    bool                  `json:"isCorrect"`
	IsSuccess    bool                  `json:"isSuccess"`
	Stdout       []string              `json:"stdout"`
	Stderr       []string              `json:"stderr"`
	DiffExitCode *int                  `json:"diffExitCode"`
	ExeExitCode  int                   `json:"exeExitCode"`
	ExitCode     int                   `json:"exitCode"`
	HasTimedOut  bool                  `json:"hasTimedOut"`
	Errors       []ValgrindReportError `json:"errors"`
	Score        *struct {
		Score float64 `json:"score"`
		Total float64 `json:"total"`
	} `json:"score"`
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

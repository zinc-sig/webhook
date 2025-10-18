package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
)

// getCurrentAcademicTerm calculates the current semester ID based on today's date
func getCurrentAcademicTerm() (int, string, int) {
	now := time.Now()
	year := now.Year()
	month := now.Month()

	var termCode string
	var termName string
	var academicYear int

	switch {
	case month >= time.September && month <= time.December:
		// Fall semester
		termCode = "10"
		termName = fmt.Sprintf("%d-%d Fall", year, year+1)
		academicYear = year
	case month == time.January:
		// Winter semester
		termCode = "20"
		termName = fmt.Sprintf("%d-%d Winter", year-1, year)
		academicYear = year - 1
	case month >= time.February && month <= time.May:
		// Spring semester
		termCode = "30"
		termName = fmt.Sprintf("%d-%d Spring", year-1, year)
		academicYear = year
	default:
		// June-August: Summer semester
		termCode = "40"
		termName = fmt.Sprintf("%d-%d Summer", year-1, year)
		academicYear = year
	}

	// Generate semester ID: last 2 digits of year + term code
	yearSuffix := year % 100
	semesterID := yearSuffix*100 + mustAtoi(termCode)

	return semesterID, termName, academicYear
}

func mustAtoi(s string) int {
	var result int
	fmt.Sscanf(s, "%d", &result)
	return result
}

func seedDatabase(dsn string, createDummy bool) error {
	// Connect to database
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Test connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Start transaction
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get current academic term
	semesterID, semesterName, academicYear := getCurrentAcademicTerm()
	slog.Info("Calculated current academic term",
		"semester_id", semesterID,
		"name", semesterName,
		"year", academicYear)

	// Insert semester (safe - only if doesn't exist)
	result, err := tx.Exec(`
		INSERT INTO semesters (id, year, name, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, semesterID, academicYear, semesterName)
	if err != nil {
		return fmt.Errorf("failed to insert semester: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		slog.Info("Created semester", "id", semesterID, "name", semesterName)
	} else {
		slog.Info("Semester already exists", "id", semesterID)
	}

	// Insert assignment types (safe - only if don't exist)
	assignmentTypes := []string{"PA", "LAB", "GP"}
	for _, typeName := range assignmentTypes {
		result, err := tx.Exec(`
			INSERT INTO assignment_types (name, created_at, updated_at)
			SELECT $1::VARCHAR, NOW(), NOW()
			WHERE NOT EXISTS (
				SELECT 1 FROM assignment_types WHERE name = $1
			)
		`, typeName)
		if err != nil {
			return fmt.Errorf("failed to insert assignment type %s: %w", typeName, err)
		}

		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			slog.Info("Created assignment type", "name", typeName)
		} else {
			slog.Info("Assignment type already exists", "name", typeName)
		}
	}

	// Insert dummy course, assignment, and assignment config if flag is set
	if createDummy {
		// Get the PA assignment type ID
		var assignmentTypeID int
		err := tx.QueryRow(`SELECT id FROM assignment_types WHERE name = 'PA' LIMIT 1`).Scan(&assignmentTypeID)
		if err != nil {
			return fmt.Errorf("failed to get PA assignment type ID: %w", err)
		}

		// Insert or get dummy course
		var courseID int64
		err = tx.QueryRow(`
			INSERT INTO courses (code, name, is_shown, semester_id, created_at, updated_at)
			VALUES ($1, $2, $3, $4, NOW(), NOW())
			ON CONFLICT (code, semester_id) DO UPDATE SET code = EXCLUDED.code
			RETURNING id
		`, "COMP0000", "Introduction to Software Testing", false, semesterID).Scan(&courseID)
		if err != nil {
			return fmt.Errorf("failed to insert/get dummy course: %w", err)
		}
		slog.Info("Ensured dummy course exists", "code", "COMP0000", "id", courseID)

		// Check if dummy assignment already exists
		var assignmentID int64
		err = tx.QueryRow(`
			SELECT id FROM assignments
			WHERE name = $1 AND course_id = $2 AND deleted_at IS NULL
			LIMIT 1
		`, "PA0", courseID).Scan(&assignmentID)

		if err == sql.ErrNoRows {
			// Assignment doesn't exist, create it
			err = tx.QueryRow(`
				INSERT INTO assignments (name, description, description_html, course_id, type, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
				RETURNING id
			`, "PA0", "Sample Programming Assignment", "<p>Sample Programming Assignment</p>", courseID, assignmentTypeID).Scan(&assignmentID)
			if err != nil {
				return fmt.Errorf("failed to insert dummy assignment: %w", err)
			}
			slog.Info("Created dummy assignment", "name", "PA0", "id", assignmentID)
		} else if err != nil {
			return fmt.Errorf("failed to query dummy assignment: %w", err)
		} else {
			slog.Info("Dummy assignment already exists", "name", "PA0", "id", assignmentID)
		}

		// Set up assignment config dates
		now := time.Now()
		dueAt := now.Add(7 * 24 * time.Hour)            // Due in 7 days
		stopCollectionAt := now.Add(8 * 24 * time.Hour) // Stop collection 1 day after due

		// Check if assignment config already exists
		var configID int64
		configYAML := `version: 1
grading:
  - name: "Test Cases"
    weight: 100
`
		err = tx.QueryRow(`
			SELECT id FROM assignment_configs
			WHERE assignment_id = $1 AND deleted_at IS NULL
			LIMIT 1
		`, assignmentID).Scan(&configID)

		if err == sql.ErrNoRows {
			// Config doesn't exist, create it
			err = tx.QueryRow(`
				INSERT INTO assignment_configs (
					due_at, stop_collection_at, assignment_id, is_tested,
					config_yaml, show_immediate_scores, grade_immediately,
					created_at, updated_at
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
				RETURNING id
			`, dueAt, stopCollectionAt, assignmentID, true, configYAML, false, false).Scan(&configID)
			if err != nil {
				return fmt.Errorf("failed to insert dummy assignment config: %w", err)
			}
			slog.Info("Created dummy assignment config", "id", configID, "due_at", dueAt.Format(time.RFC3339))
		} else if err != nil {
			return fmt.Errorf("failed to query dummy assignment config: %w", err)
		} else {
			slog.Info("Dummy assignment config already exists", "id", configID, "assignment_id", assignmentID)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

var seedCmd = &cobra.Command{
	Use:   "seed",
	Short: "seed the database with initial data (current semester and assignment types)",
	Run: func(cmd *cobra.Command, args []string) {
		dsn := cmd.Flag("dsn").Value.String()
		if dsn == "" {
			slog.Error("a valid DSN is required to seed the database")
			os.Exit(1)
		}
		dummy, _ := cmd.Flags().GetBool("dummy")
		if err := seedDatabase(dsn, dummy); err != nil {
			slog.Error("failed to seed database", "error", err)
			os.Exit(1)
		}
		slog.Info("Database seeding completed successfully")
	},
}

func init() {
	rootCmd.AddCommand(seedCmd)
	seedCmd.Flags().String("dsn", os.Getenv("DSN"), "Database connection string")
	seedCmd.Flags().Bool("dummy", false, "Create dummy data: course (COMP0000), assignment (PA0), and assignment config")
}

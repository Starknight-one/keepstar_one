package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/yaml.v3"
)

// Config represents ADW configuration
type Config struct {
	Project struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	} `yaml:"project"`
	Paths struct {
		Backend  string `yaml:"backend"`
		Frontend string `yaml:"frontend"`
		Database string `yaml:"database"`
		Specs    string `yaml:"specs"`
	} `yaml:"paths"`
	Validation []struct {
		Name     string `yaml:"name"`
		Path     string `yaml:"path"`
		Cmd      string `yaml:"cmd"`
		Required bool   `yaml:"required"`
	} `yaml:"validation"`
}

var db *sql.DB
var config Config
var projectRoot string

type PipelineRun struct {
	ID          string
	Prompt      string
	Status      string
	SpecPath    string
	TestOutput  string
	CreatedAt   time.Time
	CompletedAt sql.NullTime
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: ./sdlc \"<feature description>\"")
		fmt.Println("Example: ./sdlc \"add user authentication\"")
		os.Exit(1)
	}

	prompt := os.Args[1]

	// Find project root (where adw.yaml is)
	var err error
	projectRoot, err = findProjectRoot()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Println("Make sure adw.yaml exists in ADW/ directory")
		os.Exit(1)
	}

	// Load configuration
	if err := loadConfig(); err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Initialize database
	if err := initDB(); err != nil {
		fmt.Printf("Error initializing database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create run record
	runID, err := createRun(prompt)
	if err != nil {
		fmt.Printf("Error creating run: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n[%s] Starting SDLC Pipeline\n", runID)
	fmt.Printf("[%s] Project: %s\n", runID, config.Project.Name)
	fmt.Printf("[%s] Feature: %s\n\n", runID, truncate(prompt, 50))

	// Step 1: Run /feature
	specPath, err := runFeature(runID, prompt)
	if err != nil {
		updateRunStatus(runID, "failed", "", err.Error())
		printReport(runID)
		os.Exit(1)
	}

	if specPath == "" {
		updateRunStatus(runID, "failed", "", "Could not find spec file path")
		printReport(runID)
		os.Exit(1)
	}

	// Step 2: Run /build
	if err := runBuild(runID, specPath); err != nil {
		updateRunStatus(runID, "failed", specPath, err.Error())
		printReport(runID)
		os.Exit(1)
	}

	// Step 3: Run /test
	testOutput, err := runTest(runID)
	if err != nil {
		fmt.Printf("[%s] Warning: test step had issues: %v\n", runID, err)
	}

	// Success
	updateRunStatus(runID, "success", specPath, testOutput)
	printReport(runID)
}

func findProjectRoot() (string, error) {
	// Start from current directory, go up until we find ADW/adw.yaml
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		configPath := filepath.Join(dir, "ADW", "adw.yaml")
		if _, err := os.Stat(configPath); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("adw.yaml not found")
		}
		dir = parent
	}
}

func loadConfig() error {
	configPath := filepath.Join(projectRoot, "ADW", "adw.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("cannot read adw.yaml: %v", err)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("cannot parse adw.yaml: %v", err)
	}

	// Set defaults
	if config.Paths.Specs == "" {
		config.Paths.Specs = "ADW/specs"
	}

	return nil
}

func resolvePath(path string) string {
	// Replace {backend}, {frontend}, etc. with actual paths
	path = strings.ReplaceAll(path, "{backend}", config.Paths.Backend)
	path = strings.ReplaceAll(path, "{frontend}", config.Paths.Frontend)
	path = strings.ReplaceAll(path, "{database}", config.Paths.Database)
	return filepath.Join(projectRoot, path)
}

func initDB() error {
	dbPath := filepath.Join(projectRoot, "ADW", "sdlc.db")
	var err error
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS pipeline_runs (
			id TEXT PRIMARY KEY,
			prompt TEXT NOT NULL,
			status TEXT NOT NULL,
			spec_path TEXT DEFAULT '',
			test_output TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			completed_at DATETIME
		)
	`)
	return err
}

func createRun(prompt string) (string, error) {
	id := uuid.New().String()[:8]
	_, err := db.Exec(
		"INSERT INTO pipeline_runs (id, prompt, status) VALUES (?, ?, ?)",
		id, prompt, "running",
	)
	return id, err
}

func updateRunStatus(id, status, specPath, testOutput string) error {
	_, err := db.Exec(
		"UPDATE pipeline_runs SET status = ?, spec_path = ?, test_output = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ?",
		status, specPath, testOutput, id,
	)
	return err
}

func getRun(id string) (*PipelineRun, error) {
	run := &PipelineRun{}
	err := db.QueryRow(
		"SELECT id, prompt, status, spec_path, test_output, created_at, completed_at FROM pipeline_runs WHERE id = ?",
		id,
	).Scan(&run.ID, &run.Prompt, &run.Status, &run.SpecPath, &run.TestOutput, &run.CreatedAt, &run.CompletedAt)
	return run, err
}

func runFeature(id, prompt string) (string, error) {
	fmt.Printf("[%s] Running /feature...\n", id)

	cmd := exec.Command("claude",
		"-p",
		fmt.Sprintf("/feature \"%s\"", prompt),
		"--dangerously-skip-permissions",
	)
	cmd.Dir = projectRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("claude command failed: %v\nOutput: %s", err, string(output))
	}

	specPath := extractSpecPath(string(output))
	if specPath != "" {
		fmt.Printf("[%s] Created: %s\n", id, specPath)
	}

	return specPath, nil
}

func runBuild(id, specPath string) error {
	fmt.Printf("[%s] Running /build %s...\n", id, specPath)

	cmd := exec.Command("claude",
		"-p",
		fmt.Sprintf("/build %s", specPath),
		"--dangerously-skip-permissions",
	)
	cmd.Dir = projectRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("claude command failed: %v", err)
	}

	outputStr := string(output)
	if strings.Contains(strings.ToLower(outputStr), "error") &&
		strings.Contains(strings.ToLower(outputStr), "failed") {
		return fmt.Errorf("build reported errors")
	}

	fmt.Printf("[%s] Build complete\n", id)
	return nil
}

func runTest(id string) (string, error) {
	fmt.Printf("[%s] Running validation...\n", id)
	var results []string
	var lastErr error

	for _, v := range config.Validation {
		if v.Cmd == "" {
			continue
		}

		path := resolvePath(v.Path)
		cmdStr := fmt.Sprintf("cd %s && %s", path, v.Cmd)

		cmd := exec.Command("bash", "-c", cmdStr)
		output, err := cmd.CombinedOutput()

		if err != nil {
			if v.Required {
				results = append(results, fmt.Sprintf("%s: FAILED", v.Name))
				lastErr = fmt.Errorf("%s failed: %s", v.Name, truncate(string(output), 100))
			} else {
				results = append(results, fmt.Sprintf("%s: WARN", v.Name))
			}
		} else {
			results = append(results, fmt.Sprintf("%s: OK", v.Name))
		}
	}

	fmt.Printf("[%s] Validation complete\n", id)
	return strings.Join(results, ", "), lastErr
}

func extractSpecPath(output string) string {
	patterns := []string{
		`ADW/specs/feature-[\w-]+\.md`,
		`ADW/specs/bug-[\w-]+\.md`,
		`ADW/specs/chore-[\w-]+\.md`,
		`specs/feature-[\w-]+\.md`,
		`specs/bug-[\w-]+\.md`,
		`specs/chore-[\w-]+\.md`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if match := re.FindString(output); match != "" {
			return match
		}
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func printReport(id string) {
	run, err := getRun(id)
	if err != nil {
		fmt.Printf("Error getting run: %v\n", err)
		return
	}

	fmt.Println("")
	fmt.Println("════════════════════════════════════════")
	fmt.Println("SDLC Pipeline Complete")
	fmt.Println("════════════════════════════════════════")
	fmt.Printf("Project: %s\n", config.Project.Name)
	fmt.Printf("ADW ID: %s\n", run.ID)
	fmt.Printf("Feature: %s\n", truncate(run.Prompt, 50))
	fmt.Printf("Spec: %s\n", run.SpecPath)
	fmt.Printf("Status: %s\n", run.Status)
	if run.TestOutput != "" {
		fmt.Println("Validation:")
		fmt.Printf("  %s\n", run.TestOutput)
	}
	fmt.Println("════════════════════════════════════════")
}

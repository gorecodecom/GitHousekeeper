package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gorecode/updates/internal/logic"
)

//go:embed assets
var assets embed.FS

type RunRequest struct {
	RootPath            string
	Excluded            []string
	ParentVersion       string
	VersionBumpStrategy string // "major", "minor", "patch"
	RunCleanInstall     bool
	TargetBranch        string // "housekeeping", "custom-name", or ""
	PomReplacements     []logic.Replacement
	ProjectReplacements []logic.Replacement
}

func main() {
	// Setup File Server
	// Check if "assets" folder exists locally (Dev Mode)
	if _, err := os.Stat("assets"); err == nil {
		fmt.Println("Development Mode: Serving assets from local disk")
		http.Handle("/", http.FileServer(http.Dir("assets")))
	} else {
		// Production Mode: Use embedded assets
		// We strip "assets" prefix because embed.FS includes the directory structure
		fsys, err := fs.Sub(assets, "assets")
		if err != nil {
			panic(err)
		}
		http.Handle("/", http.FileServer(http.FS(fsys)))
	}

	// API
	http.HandleFunc("/api/run", handleRun)
	http.HandleFunc("/api/spring-versions", handleSpringVersions)
	http.HandleFunc("/api/scan-spring", handleScanSpring)
	http.HandleFunc("/api/analyze-spring", handleAnalyzeSpring)
	http.HandleFunc("/api/pick-folder", handlePickFolder)
	http.HandleFunc("/api/list-folders", handleListFolders)
	http.HandleFunc("/api/openrewrite-versions", handleOpenRewriteVersions)
	http.HandleFunc("/api/dashboard-stats", handleDashboardStats)

	port := "8080"
	url := "http://localhost:" + port

	fmt.Printf("Starting web interface at %s ...\n", url)

	// Open Browser
	go openBrowser(url)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
	}
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Set headers for streaming
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Find Repos
	var repos []string
	if logic.IsGitRepo(req.RootPath) {
		repos = []string{req.RootPath}
	} else {
		repos = logic.FindGitRepos(req.RootPath, req.Excluded)
	}

	if len(repos) == 0 {
		fmt.Fprintf(w, "No Git projects found under '%s'.\n", req.RootPath)
		flusher.Flush()
		return
	}

	fmt.Fprintf(w, "Found: %d projects\n", len(repos))
	flusher.Flush()

	for _, repo := range repos {
		repoName := filepath.Base(repo)

		// Special prefix for frontend highlighting
		fmt.Fprintf(w, "REPO:%s\n", repoName)
		flusher.Flush()

		// Define logging callback that streams to HTTP response
		logCallback := func(msg string) {
			fmt.Fprintf(w, "%s\n", msg)
			flusher.Flush()
		}

		opts := logic.RepoOptions{
			PomReplacements:     req.PomReplacements,
			ProjectReplacements: req.ProjectReplacements,
			TargetParentVersion: req.ParentVersion,
			VersionBumpStrategy: req.VersionBumpStrategy,
			RunCleanInstall:     req.RunCleanInstall,
			ExcludedFolders:     req.Excluded,
			TargetBranch:        req.TargetBranch,
			Log:                 logCallback,
		}

		entry := logic.ProcessRepo(repo, opts)

		// Deprecation output is handled separately in the UI, so we stream it with markers
		if entry.DeprecationOutput != "" {
			fmt.Fprintf(w, "DEPRECATION_START:%s\n", repoName)
			fmt.Fprintf(w, "%s\n", entry.DeprecationOutput)
			fmt.Fprintf(w, "DEPRECATION_END\n")
			flusher.Flush()
		}

		if entry.Success {
			fmt.Fprintf(w, "✓ %s erfolgreich bearbeitet.\n", repoName)
		} else {
			fmt.Fprintf(w, "✗ %s fehlgeschlagen.\n", repoName)
		}
		flusher.Flush()
	}
}

func handleSpringVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := logic.GetSpringVersions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(versions)
}

// Current OpenRewrite versions used in this app
// Moved to type definition area


func handleOpenRewriteVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := logic.GetOpenRewriteVersions(openRewritePluginVersion, openRewriteRecipeVersion)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(versions)
}

type ScanRequest struct {
	RootPath string
	Excluded []string
}

func handleScanSpring(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	results := logic.ScanProjectsForSpring(req.RootPath, req.Excluded)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

type ListFoldersRequest struct {
	Path string
}

type ListFoldersResponse struct {
	IsRepo  bool
	Folders []string
	Error   string `json:",omitempty"`
}

func handleListFolders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req ListFoldersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp := ListFoldersResponse{}

	// Check if path exists
	info, err := os.Stat(req.Path)
	if err != nil {
		resp.Error = fmt.Sprintf("Path not found: %v", err)
		json.NewEncoder(w).Encode(resp)
		return
	}
	if !info.IsDir() {
		resp.Error = "Path is not a directory"
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Check if it is a git repo
	if logic.IsGitRepo(req.Path) {
		resp.IsRepo = true
		json.NewEncoder(w).Encode(resp)
		return
	}

	// List subdirectories
	entries, err := os.ReadDir(req.Path)
	if err != nil {
		resp.Error = fmt.Sprintf("Could not read directory: %v", err)
		json.NewEncoder(w).Encode(resp)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			resp.Folders = append(resp.Folders, entry.Name())
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		fmt.Printf("Konnte Browser nicht öffnen: %v\n", err)
	}
}

func handlePickFolder(w http.ResponseWriter, r *http.Request) {
	path, err := openFolderDialog()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"path": path})
}

func openFolderDialog() (string, error) {
	switch runtime.GOOS {
	case "windows":
		return openFolderDialogWindows()
	case "darwin":
		return openFolderDialogMac()
	case "linux":
		return openFolderDialogLinux()
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func openFolderDialogWindows() (string, error) {
	psScript := `
		Add-Type -AssemblyName System.Windows.Forms
		$f = New-Object System.Windows.Forms.FolderBrowserDialog
		$f.ShowNewFolderButton = $true
		if ($f.ShowDialog() -eq 'OK') {
			Write-Host $f.SelectedPath
		}
	`
	cmd := exec.Command("powershell", "-NoProfile", "-Command", psScript)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func openFolderDialogMac() (string, error) {
	script := `POSIX path of (choose folder)`
	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func openFolderDialogLinux() (string, error) {
	// Try zenity first (common on GNOME/GTK)
	path, err := runCommandOutput("zenity", "--file-selection", "--directory")
	if err == nil && path != "" {
		return path, nil
	}

	// Try kdialog (common on KDE)
	path, err = runCommandOutput("kdialog", "--getexistingdirectory")
	if err == nil && path != "" {
		return path, nil
	}

	return "", fmt.Errorf("kein GUI-Dialog-Tool gefunden (zenity oder kdialog benötigt)")
}

func runCommandOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

type AnalyzeSpringRequest struct {
	RootPath      string   `json:"RootPath"`
	Excluded      []string `json:"Excluded"`
	TargetVersion string   `json:"TargetVersion"`
	MigrationType string   `json:"MigrationType"` // "spring-boot", "java-version", "jakarta-ee", "quarkus"
}

// AnalysisResult holds the result of analyzing a single repo
type AnalysisResult struct {
	Index    int
	RepoName string
	Output   string
	Success  bool
	Duration time.Duration
}

// Current OpenRewrite versions used in this app
const (
	openRewritePluginVersion      = "6.24.0"
	openRewriteRecipeVersion      = "6.19.0"
	openRewriteMigrateJavaVersion = "3.22.0"
	openRewriteQuarkusVersion     = "2.28.1"
)

func handleAnalyzeSpring(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AnalyzeSpringRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Transfer-Encoding", "chunked")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// 1. Find Repos
	var repos []string
	if logic.IsGitRepo(req.RootPath) {
		repos = []string{req.RootPath}
	} else {
		repos = logic.FindGitRepos(req.RootPath, req.Excluded)
	}

	if len(repos) == 0 {
		fmt.Fprintf(w, "No Git projects found under '%s'.\n", req.RootPath)
		flusher.Flush()
		return
	}

	fmt.Fprintf(w, "PROGRESS_INIT:%d\n", len(repos))
	
	migrationLabel := "Spring Boot"
	switch req.MigrationType {
	case "java-version":
		migrationLabel = "Java"
	case "jakarta-ee":
		migrationLabel = "Jakarta EE"
	case "quarkus":
		migrationLabel = "Quarkus"
	}

	fmt.Fprintf(w, "Found %d projects. Starting parallel analysis for %s %s...\n", len(repos), migrationLabel, req.TargetVersion)
	fmt.Fprintf(w, "(Processing in background, results will appear as each project completes)\n\n")
	flusher.Flush()

	overallStart := time.Now()

	// 2. Determine Recipe and Coordinates
	var recipe string
	var coordinates string

	switch req.MigrationType {
	case "java-version":
		// TargetVersion e.g. "17", "21", "25"
		// Correct recipe name is UpgradeToJava<Version>
		recipe = fmt.Sprintf("org.openrewrite.java.migrate.UpgradeToJava%s", req.TargetVersion)
		coordinates = fmt.Sprintf("org.openrewrite.recipe:rewrite-migrate-java:%s", openRewriteMigrateJavaVersion)
	case "jakarta-ee":
		recipe = "org.openrewrite.java.migrate.jakarta.JavaxMigrationToJakarta"
		coordinates = fmt.Sprintf("org.openrewrite.recipe:rewrite-migrate-java:%s", openRewriteMigrateJavaVersion)
	case "quarkus":
		// Quarkus migration is complex and project-specific
		// Return an informative message instead of running a recipe
		fmt.Fprintf(w, "⚠️ Quarkus Migration Information\n\n")
		fmt.Fprintf(w, "Quarkus migration requires a project-specific approach:\n\n")
		fmt.Fprintf(w, "For Spring Boot → Quarkus:\n")
		fmt.Fprintf(w, "- Use Quarkus CLI: 'quarkus create app' to generate new structure\n")
		fmt.Fprintf(w, "- Migrate dependencies manually using Quarkus guides\n")
		fmt.Fprintf(w, "- Replace Spring annotations with Quarkus equivalents\n\n")
		fmt.Fprintf(w, "For Quarkus Version Upgrades:\n")
		fmt.Fprintf(w, "- Use 'quarkus update' command directly in your project\n")
		fmt.Fprintf(w, "- Or update versions in pom.xml and run tests\n\n")
		fmt.Fprintf(w, "Resources:\n")
		fmt.Fprintf(w, "- Migration Guide: https://quarkus.io/guides/migration-guide\n")
		fmt.Fprintf(w, "- Spring to Quarkus: https://quarkus.io/blog/spring-boot-to-quarkus/\n\n")
		flusher.Flush()
		return
	default: // "spring-boot" or empty
		cleanVersion := strings.ReplaceAll(req.TargetVersion, ".", "_")
		if strings.HasPrefix(req.TargetVersion, "3.") {
			recipe = fmt.Sprintf("org.openrewrite.java.spring.boot3.UpgradeSpringBoot_%s", cleanVersion)
		} else if strings.HasPrefix(req.TargetVersion, "2.") {
			recipe = fmt.Sprintf("org.openrewrite.java.spring.boot2.UpgradeSpringBoot_%s", cleanVersion)
		} else {
			// Fallback
			recipe = fmt.Sprintf("org.openrewrite.java.spring.boot%c.UpgradeSpringBoot_%s", req.TargetVersion[0], cleanVersion)
		}
		coordinates = fmt.Sprintf("org.openrewrite.recipe:rewrite-spring:%s", openRewriteRecipeVersion)
	}

	// Use globally defined plugin versions
	pluginVersion := openRewritePluginVersion

	// 3. Run Analysis in Parallel
	resultChan := make(chan AnalysisResult, len(repos))

	for i, repo := range repos {
		go func(index int, repoPath string) {
			result := analyzeRepo(index, repoPath, recipe, pluginVersion, coordinates)
			resultChan <- result
		}(i, repo)
	}

	// 4. Collect and output results in order of completion
	completed := 0
	var totalDuration time.Duration
	for completed < len(repos) {
		result := <-resultChan
		completed++
		totalDuration += result.Duration

		// Calculate average time per project and estimate remaining
		avgDuration := totalDuration / time.Duration(completed)
		remaining := len(repos) - completed
		estimatedRemaining := avgDuration * time.Duration(remaining)

		// Output progress update
		fmt.Fprintf(w, "PROGRESS_UPDATE:%d:%d:%.1f\n", completed, len(repos), estimatedRemaining.Seconds())

		// Output the complete result block for this repo
		statusIcon := "✓"
		if !result.Success {
			statusIcon = "✗"
		}
		fmt.Fprintf(w, ">>> [%d/%d] %s %s (%.1fs)\n", completed, len(repos), statusIcon, result.RepoName, result.Duration.Seconds())
		fmt.Fprintf(w, "%s", result.Output)
		fmt.Fprintf(w, "\n")
		flusher.Flush()
	}

	close(resultChan)

	// Final summary
	overallDuration := time.Since(overallStart)
	fmt.Fprintf(w, "PROGRESS_DONE:%.1f\n", overallDuration.Seconds())
	flusher.Flush()
}

// analyzeRepo performs the OpenRewrite analysis on a single repository
func analyzeRepo(index int, repoPath, recipe, pluginVersion, recipeArtifactCoordinates string) AnalysisResult {
	startTime := time.Now()
	repoName := filepath.Base(repoPath)
	var output strings.Builder

	// Check if it's a Maven project
	if _, err := os.Stat(filepath.Join(repoPath, "pom.xml")); os.IsNotExist(err) {
		output.WriteString("Skipping (no pom.xml)\n")
		return AnalysisResult{Index: index, RepoName: repoName, Output: output.String(), Success: true, Duration: time.Since(startTime)}
	}

	// Construct Maven Command
	cmd := exec.Command("mvn",
		"-U",
		"-B",
		fmt.Sprintf("org.openrewrite.maven:rewrite-maven-plugin:%s:dryRun", pluginVersion),
		fmt.Sprintf("-Drewrite.recipeArtifactCoordinates=%s", recipeArtifactCoordinates),
		fmt.Sprintf("-Drewrite.activeRecipes=%s", recipe),
	)
	cmd.Dir = repoPath

	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		output.WriteString(fmt.Sprintf("Error running OpenRewrite: %v\n", err))
		lines := strings.Split(string(cmdOutput), "\n")
		start := len(lines) - 10
		if start < 0 {
			start = 0
		}
		for _, line := range lines[start:] {
			output.WriteString(fmt.Sprintf("  %s\n", line))
		}
		return AnalysisResult{Index: index, RepoName: repoName, Output: output.String(), Success: false, Duration: time.Since(startTime)}
	}

	// Check for patch file
	patchFile := filepath.Join(repoPath, "target", "rewrite", "rewrite.patch")
	if _, err := os.Stat(patchFile); err == nil {
		content, err := os.ReadFile(patchFile)
		if err == nil && len(content) > 0 {
			// Parse and summarize the patch
			summary := parsePatchToSummary(string(content))
			output.WriteString(summary)
		} else {
			output.WriteString("✅ No changes required.\n")
		}
	} else {
		if strings.Contains(string(cmdOutput), "No changes") {
			output.WriteString("✅ No changes required.\n")
		} else {
			output.WriteString("Analysis finished (no patch file generated).\n")
		}
	}

	return AnalysisResult{Index: index, RepoName: repoName, Output: output.String(), Success: true, Duration: time.Since(startTime)}
}

// parsePatchToSummary converts a raw patch file into a readable summary
func parsePatchToSummary(patch string) string {
	var summary strings.Builder

	// Track changes by category
	categories := map[string][]string{
		"🔄 Annotation Updates":       {},
		"📦 Import Changes":           {},
		"🛠️ Code Modernization":      {},
		"⚙️ Configuration Changes":   {},
		"🗑️ Deprecated Code Removal": {},
	}

	// Track files changed
	filesChanged := []string{}
	currentFile := ""

	lines := strings.Split(patch, "\n")
	for i, line := range lines {
		// Track file names
		if strings.HasPrefix(line, "diff --git") {
			parts := strings.Split(line, " ")
			if len(parts) >= 4 {
				file := strings.TrimPrefix(parts[2], "a/")
				currentFile = file
				filesChanged = append(filesChanged, file)
			}
			continue
		}

		// Skip header lines
		if strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "@@") {
			continue
		}

		// Analyze removed lines (-)
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed := strings.TrimPrefix(line, "-")
			removed = strings.TrimSpace(removed)

			// Look for the corresponding added line
			added := ""
			for j := i + 1; j < len(lines) && j < i+5; j++ {
				if strings.HasPrefix(lines[j], "+") && !strings.HasPrefix(lines[j], "+++") {
					added = strings.TrimPrefix(lines[j], "+")
					added = strings.TrimSpace(added)
					break
				}
			}

			// Categorize changes
			shortFile := filepath.Base(currentFile)

			// RequestMapping -> GetMapping/PostMapping/etc.
			if strings.Contains(removed, "@RequestMapping") && strings.Contains(removed, "RequestMethod") {
				if strings.Contains(added, "@GetMapping") {
					categories["🔄 Annotation Updates"] = append(categories["🔄 Annotation Updates"],
						fmt.Sprintf("%s: @RequestMapping(method=GET) → @GetMapping", shortFile))
				} else if strings.Contains(added, "@PostMapping") {
					categories["🔄 Annotation Updates"] = append(categories["🔄 Annotation Updates"],
						fmt.Sprintf("%s: @RequestMapping(method=POST) → @PostMapping", shortFile))
				} else if strings.Contains(added, "@PutMapping") {
					categories["🔄 Annotation Updates"] = append(categories["🔄 Annotation Updates"],
						fmt.Sprintf("%s: @RequestMapping(method=PUT) → @PutMapping", shortFile))
				} else if strings.Contains(added, "@DeleteMapping") {
					categories["🔄 Annotation Updates"] = append(categories["🔄 Annotation Updates"],
						fmt.Sprintf("%s: @RequestMapping(method=DELETE) → @DeleteMapping", shortFile))
				}
			}

			// Import changes
			if strings.Contains(removed, "import ") && strings.Contains(added, "import ") {
				oldImport := strings.TrimPrefix(removed, "import ")
				oldImport = strings.TrimSuffix(oldImport, ";")
				newImport := strings.TrimPrefix(added, "import ")
				newImport = strings.TrimSuffix(newImport, ";")
				if oldImport != newImport {
					// Only show significant import changes
					if strings.Contains(oldImport, "RequestMethod") {
						// Skip, already covered by annotation changes
					} else {
						categories["📦 Import Changes"] = append(categories["📦 Import Changes"],
							fmt.Sprintf("%s: %s", shortFile, filepath.Base(newImport)))
					}
				}
			}

			// HibernateProxy pattern matching
			if strings.Contains(removed, "instanceof HibernateProxy") && strings.Contains(removed, "((HibernateProxy)") {
				if strings.Contains(added, "instanceof HibernateProxy hp") {
					categories["🛠️ Code Modernization"] = append(categories["🛠️ Code Modernization"],
						fmt.Sprintf("%s: instanceof + cast → Pattern Matching (Java 16+)", shortFile))
				}
			}

			// String.format -> formatted
			if strings.Contains(removed, "String.format(") && strings.Contains(added, ".formatted(") {
				categories["🛠️ Code Modernization"] = append(categories["🛠️ Code Modernization"],
					fmt.Sprintf("%s: String.format() → String.formatted()", shortFile))
			}

			// @Autowired removal
			if strings.Contains(removed, "@Autowired") && !strings.Contains(added, "@Autowired") {
				categories["🗑️ Deprecated Code Removal"] = append(categories["🗑️ Deprecated Code Removal"],
					fmt.Sprintf("%s: Removed unnecessary @Autowired (constructor injection)", shortFile))
			}

			// Configuration property changes
			if strings.HasSuffix(currentFile, ".properties") || strings.HasSuffix(currentFile, ".yml") || strings.HasSuffix(currentFile, ".yaml") {
				if strings.Contains(removed, "=") || strings.Contains(removed, ":") {
					if strings.Contains(line, "deprecated") || strings.Contains(added, "#") {
						propName := strings.Split(removed, "=")[0]
						propName = strings.Split(propName, ":")[0]
						propName = strings.TrimSpace(propName)
						if propName != "" && !strings.HasPrefix(propName, "#") {
							categories["⚙️ Configuration Changes"] = append(categories["⚙️ Configuration Changes"],
								fmt.Sprintf("%s: Property '%s' deprecated/changed", shortFile, propName))
						}
					}
				}
			}
		}
	}

	// Build HTML summary output for better readability
	summary.WriteString(`<div class="migration-summary">`)
	summary.WriteString(`<h2 style="margin:0 0 15px 0; color:#cdd6f4; border-bottom:2px solid #89b4fa; padding-bottom:10px;">📋 Migration Summary</h2>`)

	// Files overview
	summary.WriteString(fmt.Sprintf(`<div class="summary-section"><h3 style="color:#89b4fa; margin:15px 0 10px 0;">📁 Files affected: %d</h3>`, len(filesChanged)))
	summary.WriteString(`<div style="display:flex; flex-wrap:wrap; gap:5px; margin-left:10px;">`)
	for _, f := range filesChanged {
		shortFile := filepath.Base(f)
		summary.WriteString(fmt.Sprintf(`<span style="background:#313244; padding:3px 8px; border-radius:4px; font-size:0.85em;">%s</span>`, shortFile))
	}
	summary.WriteString(`</div></div>`)

	// Changes by category - use ordered slice for consistent output
	categoryOrder := []string{
		"🔄 Annotation Updates",
		"📦 Import Changes",
		"🛠️ Code Modernization",
		"⚙️ Configuration Changes",
		"🗑️ Deprecated Code Removal",
	}

	hasChanges := false
	for _, category := range categoryOrder {
		changes := categories[category]
		if len(changes) > 0 {
			hasChanges = true
			// Deduplicate
			unique := make(map[string]bool)
			for _, c := range changes {
				unique[c] = true
			}

			summary.WriteString(fmt.Sprintf(`<div class="summary-section" style="margin-top:20px;"><h3 style="color:#a6e3a1; margin:0 0 10px 0;">%s <span style="background:#45475a; padding:2px 8px; border-radius:10px; font-size:0.8em;">%d</span></h3>`, category, len(unique)))
			summary.WriteString(`<table style="width:100%; border-collapse:collapse; font-size:0.9em;">`)
			for change := range unique {
				// Split change into file and description
				parts := strings.SplitN(change, ": ", 2)
				file := parts[0]
				desc := ""
				if len(parts) > 1 {
					desc = parts[1]
				}
				summary.WriteString(fmt.Sprintf(`<tr style="border-bottom:1px solid #313244;"><td style="padding:6px 10px; color:#f9e2af; white-space:nowrap; width:1%%;">%s</td><td style="padding:6px 10px; color:#cdd6f4;">%s</td></tr>`, file, desc))
			}
			summary.WriteString(`</table></div>`)
		}
	}

	if !hasChanges {
		summary.WriteString(`<div class="summary-section" style="margin-top:20px; padding:15px; background:#313244; border-radius:8px;">`)
		summary.WriteString(`<p style="margin:0; color:#f9e2af;">ℹ️ Changes detected but could not be automatically categorized.</p>`)
		summary.WriteString(`<p style="margin:5px 0 0 0; color:#a6adc8;">Run with full patch output for details.</p>`)
		summary.WriteString(`</div>`)
	}

	summary.WriteString(`<div style="margin-top:20px; padding:12px; background:#1e1e2e; border-left:3px solid #89b4fa; border-radius:4px;">`)
	summary.WriteString(`<p style="margin:0; color:#89b4fa;">💡 <strong>Tip:</strong> These are recommended changes for your Spring Boot upgrade. Review each change before applying.</p>`)
	summary.WriteString(`</div>`)
	summary.WriteString(`</div>`)

	return summary.String()
}

func handleDashboardStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Set headers for streaming NDJSON
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	logic.StreamDashboardStats(req.RootPath, req.Excluded, func(result interface{}) {
		json.NewEncoder(w).Encode(result)
		flusher.Flush()
	})
}


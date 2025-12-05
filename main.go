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
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
	Replacements        []logic.Replacement
	ReplacementScope    string // "all", "pom-only", "exclude-pom"
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
	http.HandleFunc("/api/health", handleHealth)
	http.HandleFunc("/api/run", handleRun)
	http.HandleFunc("/api/spring-versions", handleSpringVersions)
	http.HandleFunc("/api/scan-spring", handleScanSpring)
	http.HandleFunc("/api/analyze-spring", handleAnalyzeSpring)
	http.HandleFunc("/api/pick-folder", handlePickFolder)
	http.HandleFunc("/api/list-folders", handleListFolders)
	http.HandleFunc("/api/openrewrite-versions", handleOpenRewriteVersions)
	http.HandleFunc("/api/dashboard-stats", handleDashboardStats)
	http.HandleFunc("/api/list-branches", handleListBranches)
	http.HandleFunc("/api/sync-branches", handleSyncBranches)
	http.HandleFunc("/api/security-scan", handleSecurityScan)
	http.HandleFunc("/api/check-trivy", handleCheckTrivy)
	http.HandleFunc("/api/check-npm", handleCheckNpm)
	http.HandleFunc("/api/check-go", handleCheckGo)
	http.HandleFunc("/api/check-python", handleCheckPython)
	http.HandleFunc("/api/check-php", handleCheckPhp)

	port := "8080"
	url := "http://localhost:" + port

	fmt.Printf("Starting web interface at %s ...\n", url)

	// Open Browser
	go openBrowser(url)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
	}
}

// Health check endpoint for connection monitoring
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
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
			Replacements:        req.Replacements,
			ReplacementScope:    req.ReplacementScope,
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
			fmt.Fprintf(w, "✓ %s processed successfully.\n", repoName)
		} else {
			fmt.Fprintf(w, "✗ %s failed.\n", repoName)
		}
		flusher.Flush()
	}
}

// Cache for Spring versions to avoid repeated Maven Central calls
var (
	springVersionsCache     []logic.SpringVersionInfo
	springVersionsCacheTime time.Time
	springVersionsCacheTTL  = 5 * time.Minute
)

func handleSpringVersions(w http.ResponseWriter, r *http.Request) {
	// Check cache
	if springVersionsCache != nil && time.Since(springVersionsCacheTime) < springVersionsCacheTTL {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		json.NewEncoder(w).Encode(springVersionsCache)
		return
	}

	versions, err := logic.GetSpringVersions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update cache
	springVersionsCache = versions
	springVersionsCacheTime = time.Now()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	json.NewEncoder(w).Encode(versions)
}

// Current OpenRewrite versions used in this app
// Moved to type definition area

// Cache for OpenRewrite versions
var (
	openRewriteVersionsCache     []logic.OpenRewriteVersionInfo
	openRewriteVersionsCacheTime time.Time
	openRewriteVersionsCacheTTL  = 10 * time.Minute
)

func handleOpenRewriteVersions(w http.ResponseWriter, r *http.Request) {
	// Check cache
	if openRewriteVersionsCache != nil && time.Since(openRewriteVersionsCacheTime) < openRewriteVersionsCacheTTL {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		json.NewEncoder(w).Encode(openRewriteVersionsCache)
		return
	}

	versions, err := logic.GetOpenRewriteVersions(openRewritePluginVersion, openRewriteRecipeVersion)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update cache
	openRewriteVersionsCache = versions
	openRewriteVersionsCacheTime = time.Now()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
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
		fmt.Printf("Could not open browser: %v\n", err)
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

	return "", fmt.Errorf("no GUI dialog tool found (zenity or kdialog required)")
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

		// OpenRewrite only has recipes for minor versions (e.g., 3.5), not patch versions (e.g., 3.5.8)
		// Extract only major.minor from the target version
		minorVersion := req.TargetVersion
		versionParts := strings.Split(req.TargetVersion, ".")
		if len(versionParts) >= 2 {
			minorVersion = versionParts[0] + "." + versionParts[1]
		}
		cleanVersion := strings.ReplaceAll(minorVersion, ".", "_")
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

	// 3. Send list of repos that will be analyzed (for live status display)
	for _, repo := range repos {
		fmt.Fprintf(w, "REPO_QUEUED:%s\n", filepath.Base(repo))
	}
	flusher.Flush()

	// 4. Run Analysis in Parallel
	resultChan := make(chan AnalysisResult, len(repos))

	for i, repo := range repos {
		go func(index int, repoPath string) {
			result := analyzeRepo(index, repoPath, recipe, pluginVersion, coordinates)
			resultChan <- result
		}(i, repo)
	}

	// 5. Collect and output results in order of completion
	completed := 0
	var totalDuration time.Duration
	for completed < len(repos) {
		result := <-resultChan
		completed++
		totalDuration += result.Duration

		// Send repo completion status
		statusMarker := "SUCCESS"
		if !result.Success {
			statusMarker = "FAILED"
		}
		fmt.Fprintf(w, "REPO_DONE:%s:%s:%.1f\n", result.RepoName, statusMarker, result.Duration.Seconds())

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

	// Try up to 2 times (retry once on failure - helps with Maven cache issues)
	maxRetries := 2
	var lastError error
	var cmdOutput []byte

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Construct Maven Command
		cmd := exec.Command("mvn",
			"-U",
			"-B",
			fmt.Sprintf("org.openrewrite.maven:rewrite-maven-plugin:%s:dryRun", pluginVersion),
			fmt.Sprintf("-Drewrite.recipeArtifactCoordinates=%s", recipeArtifactCoordinates),
			fmt.Sprintf("-Drewrite.activeRecipes=%s", recipe),
		)
		cmd.Dir = repoPath

		cmdOutput, lastError = cmd.CombinedOutput()
		if lastError == nil {
			// Success - break out of retry loop
			break
		}

		// If this was the first attempt and it failed, retry
		if attempt < maxRetries {
			// Brief pause before retry
			time.Sleep(500 * time.Millisecond)
			continue
		}
	}

	// If still failed after retries
	if lastError != nil {
		output.WriteString(fmt.Sprintf("Error running OpenRewrite: %v\n", lastError))
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

	// Use mutex to protect concurrent writes to ResponseWriter
	var mu sync.Mutex
	logic.StreamDashboardStats(req.RootPath, req.Excluded, func(result interface{}) {
		mu.Lock()
		defer mu.Unlock()
		json.NewEncoder(w).Encode(result)
		flusher.Flush()
	})
}

// BranchInfo represents a branch with its tracking status
type BranchInfo struct {
	Name       string `json:"name"`
	IsTracking bool   `json:"isTracking"`
	Remote     string `json:"remote"`
	Ahead      int    `json:"ahead"`
	Behind     int    `json:"behind"`
}

// RepoWithBranches represents a repository and its branches
type RepoWithBranches struct {
	Name          string       `json:"name"`
	Path          string       `json:"path"`
	DefaultBranch string       `json:"defaultBranch"`
	Branches      []BranchInfo `json:"branches"`
}

type ListBranchesRequest struct {
	RootPath string   `json:"rootPath"`
	Excluded []string `json:"excluded"`
}

func handleListBranches(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ListBranchesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	repos := logic.FindGitRepos(req.RootPath, req.Excluded)
	var result []RepoWithBranches

	for _, repoPath := range repos {
		repoName := filepath.Base(repoPath)

		// Get default branch
		defaultBranch := getRepoDefaultBranch(repoPath)

		// Get all local branches with tracking info
		branches := getRepoBranches(repoPath)

		result = append(result, RepoWithBranches{
			Name:          repoName,
			Path:          repoPath,
			DefaultBranch: defaultBranch,
			Branches:      branches,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func getRepoDefaultBranch(repoPath string) string {
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err == nil {
		branch := strings.TrimPrefix(strings.TrimSpace(string(output)), "refs/remotes/origin/")
		if branch != "" {
			return branch
		}
	}

	// Fallback: check if main exists
	cmd = exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/main")
	cmd.Dir = repoPath
	if cmd.Run() == nil {
		return "main"
	}

	return "master"
}

func getRepoBranches(repoPath string) []BranchInfo {
	var branches []BranchInfo

	// Get local branches with their upstream tracking info
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname:short)|%(upstream:short)|%(upstream:track)", "refs/heads/")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return branches
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		branchName := parts[0]
		remote := ""
		ahead := 0
		behind := 0
		isTracking := false

		if len(parts) > 1 && parts[1] != "" {
			remote = parts[1]
			isTracking = true
		}

		if len(parts) > 2 && parts[2] != "" {
			// Parse [ahead X, behind Y] or [ahead X] or [behind Y]
			track := parts[2]
			if strings.Contains(track, "ahead") {
				fmt.Sscanf(track, "[ahead %d", &ahead)
			}
			if strings.Contains(track, "behind") {
				if strings.Contains(track, "ahead") {
					fmt.Sscanf(track, "[ahead %d, behind %d]", &ahead, &behind)
				} else {
					fmt.Sscanf(track, "[behind %d]", &behind)
				}
			}
		}

		branches = append(branches, BranchInfo{
			Name:       branchName,
			IsTracking: isTracking,
			Remote:     remote,
			Ahead:      ahead,
			Behind:     behind,
		})
	}

	return branches
}

type SyncBranchesRequest struct {
	RootPath string   `json:"rootPath"`
	Excluded []string `json:"excluded"`
}

func handleSyncBranches(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SyncBranchesRequest
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

	repos := logic.FindGitRepos(req.RootPath, req.Excluded)
	total := len(repos)

	fmt.Fprintf(w, "SYNC_INIT:%d\n", total)
	flusher.Flush()

	for i, repoPath := range repos {
		repoName := filepath.Base(repoPath)
		fmt.Fprintf(w, "REPO_START:%s\n", repoName)
		flusher.Flush()

		// Remember current branch
		currentBranch := getCurrentBranch(repoPath)

		// Fetch with prune
		cmd := exec.Command("git", "fetch", "-p", "--all")
		cmd.Dir = repoPath
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(w, "  [WARNING] Fetch failed: %v\n", err)
		} else {
			fmt.Fprintf(w, "  Fetched all remotes\n")
		}
		flusher.Flush()

		// Get all tracking branches and pull them
		branches := getRepoBranches(repoPath)
		for _, branch := range branches {
			if !branch.IsTracking {
				continue
			}

			// Checkout branch
			cmd = exec.Command("git", "checkout", branch.Name)
			cmd.Dir = repoPath
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(w, "  [WARNING] Could not checkout %s: %v\n", branch.Name, err)
				continue
			}

			// Pull
			cmd = exec.Command("git", "pull", "--ff-only")
			cmd.Dir = repoPath
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(w, "  [WARNING] Pull %s failed (maybe conflicts): %v\n", branch.Name, err)
			} else {
				fmt.Fprintf(w, "  ✓ %s updated\n", branch.Name)
			}
		}

		// Switch back to original branch
		if currentBranch != "" {
			cmd = exec.Command("git", "checkout", currentBranch)
			cmd.Dir = repoPath
			cmd.Run()
		}

		fmt.Fprintf(w, "REPO_DONE:%s\n", repoName)
		fmt.Fprintf(w, "SYNC_PROGRESS:%d:%d\n", i+1, total)
		flusher.Flush()
	}

	fmt.Fprintf(w, "SYNC_COMPLETE\n")
	flusher.Flush()
}

func getCurrentBranch(repoPath string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// ==================== SECURITY SCAN ====================

type SecurityScanRequest struct {
	RootPath     string   `json:"rootPath"`
	Excluded     []string `json:"excluded"`
	Scanner      string   `json:"scanner"`      // "owasp", "trivy", "npm", or "auto"
	TargetBranch string   `json:"targetBranch"` // Optional: branch to scan (empty = current branch)
}

type CVEFinding struct {
	CVE         string `json:"cve"`
	Severity    string `json:"severity"` // CRITICAL, HIGH, MEDIUM, LOW
	Package     string `json:"package"`
	Version     string `json:"version"`
	FixedIn     string `json:"fixedIn,omitempty"`
	Description string `json:"description,omitempty"`
}

type RepoSecurityResult struct {
	RepoName      string       `json:"repoName"`
	Findings      []CVEFinding `json:"findings"`
	Error         string       `json:"error,omitempty"`
	Duration      float64      `json:"duration"`
	ProjectType   string       `json:"projectType,omitempty"`   // "maven", "npm", "yarn", "pnpm"
	ScannedBranch string       `json:"scannedBranch,omitempty"` // The branch that was scanned
}

// detectProjectType checks what kind of project this is
func detectProjectType(repoPath string) string {
	// Check for Maven
	if _, err := os.Stat(filepath.Join(repoPath, "pom.xml")); err == nil {
		return "maven"
	}
	// Check for Go
	if _, err := os.Stat(filepath.Join(repoPath, "go.mod")); err == nil {
		return "go"
	}
	// Check for PHP (composer.json)
	if _, err := os.Stat(filepath.Join(repoPath, "composer.json")); err == nil {
		return "php"
	}
	// Check for Python (in priority order)
	if _, err := os.Stat(filepath.Join(repoPath, "requirements.txt")); err == nil {
		return "python"
	}
	if _, err := os.Stat(filepath.Join(repoPath, "pyproject.toml")); err == nil {
		return "python"
	}
	if _, err := os.Stat(filepath.Join(repoPath, "setup.py")); err == nil {
		return "python"
	}
	if _, err := os.Stat(filepath.Join(repoPath, "Pipfile")); err == nil {
		return "python"
	}
	// Check for pnpm
	if _, err := os.Stat(filepath.Join(repoPath, "pnpm-lock.yaml")); err == nil {
		return "pnpm"
	}
	// Check for Yarn
	if _, err := os.Stat(filepath.Join(repoPath, "yarn.lock")); err == nil {
		return "yarn"
	}
	// Check for npm
	if _, err := os.Stat(filepath.Join(repoPath, "package-lock.json")); err == nil {
		return "npm"
	}
	// Check for package.json without lockfile (default to npm)
	if _, err := os.Stat(filepath.Join(repoPath, "package.json")); err == nil {
		return "npm"
	}
	// Check for .py files in root as a fallback for Python projects
	if hasPythonFiles(repoPath) {
		return "python-no-deps"
	}
	return ""
}

// hasPythonFiles checks if there are .py files in the root directory
func hasPythonFiles(repoPath string) bool {
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".py") {
			return true
		}
	}
	return false
}

// checkNpmAvailable checks if npm is available
func checkNpmAvailable() bool {
	cmd := exec.Command("npm", "--version")
	return cmd.Run() == nil
}

// checkYarnAvailable checks if yarn is available
func checkYarnAvailable() bool {
	cmd := exec.Command("yarn", "--version")
	return cmd.Run() == nil
}

// checkPnpmAvailable checks if pnpm is available
func checkPnpmAvailable() bool {
	cmd := exec.Command("pnpm", "--version")
	return cmd.Run() == nil
}

// checkGovulncheckAvailable checks if govulncheck is available
func checkGovulncheckAvailable() bool {
	cmd := exec.Command("govulncheck", "-version")
	return cmd.Run() == nil
}

// checkPipAuditAvailable checks if pip-audit is available
func checkPipAuditAvailable() bool {
	cmd := exec.Command("pip-audit", "--version")
	return cmd.Run() == nil
}

// checkComposerAvailable checks if composer is available
func checkComposerAvailable() bool {
	cmd := exec.Command("composer", "--version")
	return cmd.Run() == nil
}

func handleCheckTrivy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check if trivy is available
	cmd := exec.Command("which", "trivy")
	if runtime.GOOS == "windows" {
		cmd = exec.Command("where", "trivy")
	}

	if err := cmd.Run(); err != nil {
		json.NewEncoder(w).Encode(map[string]bool{"available": false})
		return
	}

	// Get trivy version
	cmd = exec.Command("trivy", "--version")
	output, err := cmd.Output()
	version := ""
	if err == nil {
		lines := strings.Split(string(output), "\n")
		if len(lines) > 0 {
			version = strings.TrimSpace(lines[0])
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"available": true,
		"version":   version,
	})
}

func handleCheckNpm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	result := map[string]bool{
		"npm":  checkNpmAvailable(),
		"yarn": checkYarnAvailable(),
		"pnpm": checkPnpmAvailable(),
	}

	json.NewEncoder(w).Encode(result)
}

func handleCheckGo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	available := checkGovulncheckAvailable()
	version := ""

	if available {
		cmd := exec.Command("govulncheck", "-version")
		output, err := cmd.Output()
		if err == nil {
			version = strings.TrimSpace(string(output))
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"available": available,
		"version":   version,
	})
}

func handleCheckPython(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	available := checkPipAuditAvailable()
	version := ""

	if available {
		cmd := exec.Command("pip-audit", "--version")
		output, err := cmd.Output()
		if err == nil {
			version = strings.TrimSpace(string(output))
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"available": available,
		"version":   version,
	})
}

func handleCheckPhp(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	available := checkComposerAvailable()
	version := ""

	if available {
		cmd := exec.Command("composer", "--version")
		output, err := cmd.Output()
		if err == nil {
			// Extract version from "Composer version 2.x.x ..."
			version = strings.TrimSpace(string(output))
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"available": available,
		"version":   version,
	})
}

func handleSecurityScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SecurityScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Default to OWASP
	if req.Scanner == "" {
		req.Scanner = "owasp"
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

	// Debug: Log the request parameters
	fmt.Printf("[SecurityScan] RootPath: %s, Excluded: %v, Scanner: %s\n", req.RootPath, req.Excluded, req.Scanner)

	repos := logic.FindGitRepos(req.RootPath, req.Excluded)
	total := len(repos)

	// Debug: Log found repos
	fmt.Printf("[SecurityScan] Found %d repos: %v\n", total, repos)

	fmt.Fprintf(w, "SCAN_INIT:%d:%s\n", total, req.Scanner)
	flusher.Flush()

	// Determine worker count (parallel scans)
	workerCount := 4
	if total < workerCount {
		workerCount = total
	}
	if workerCount < 1 {
		workerCount = 1
	}

	// Create channels for work distribution
	type scanJob struct {
		repoPath     string
		repoName     string
		index        int
		targetBranch string
	}

	type scanResult struct {
		result RepoSecurityResult
		index  int
	}

	jobs := make(chan scanJob, total)
	results := make(chan scanResult, total)

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				start := time.Now()
				var result RepoSecurityResult
				result.RepoName = job.repoName

				// Handle branch switching if targetBranch is specified
				var originalBranch string
				branchSwitched := false
				if job.targetBranch != "" {
					// Get current branch
					originalBranch = getCurrentBranch(job.repoPath)

					// Only switch if we're not already on the target branch
					if originalBranch != job.targetBranch {
						// Check if there are uncommitted changes
						cmd := exec.Command("git", "status", "--porcelain")
						cmd.Dir = job.repoPath
						statusOutput, _ := cmd.Output()

						if len(statusOutput) > 0 {
							// Stash changes
							stashCmd := exec.Command("git", "stash", "push", "-m", "GitHousekeeper security scan")
							stashCmd.Dir = job.repoPath
							stashCmd.Run()
						}

						// Switch to target branch
						checkoutCmd := exec.Command("git", "checkout", job.targetBranch)
						checkoutCmd.Dir = job.repoPath
						if err := checkoutCmd.Run(); err != nil {
							result.Error = fmt.Sprintf("Failed to checkout branch %s: %v", job.targetBranch, err)
							result.Duration = time.Since(start).Seconds()
							results <- scanResult{result: result, index: job.index}
							continue
						}
						branchSwitched = true
						result.ScannedBranch = job.targetBranch
					} else {
						result.ScannedBranch = originalBranch
					}
				} else {
					result.ScannedBranch = getCurrentBranch(job.repoPath)
				}

				// Store scanned branch for result preservation
				scannedBranch := result.ScannedBranch

				// Detect project type
				projectType := detectProjectType(job.repoPath)
				result.ProjectType = projectType

				// Determine which scanner to use
				scannerToUse := req.Scanner
				if req.Scanner == "auto" {
					// Auto-detect based on project type
					switch projectType {
					case "maven":
						scannerToUse = "owasp"
					case "npm", "yarn", "pnpm":
						scannerToUse = "npm"
					case "go":
						scannerToUse = "govulncheck"
					case "python":
						scannerToUse = "pip-audit"
					case "php":
						scannerToUse = "composer-audit"
					case "python-no-deps":
						result.Error = "Python project found but no requirements.txt, pyproject.toml, setup.py, or Pipfile. Cannot scan without dependency file."
						result.ProjectType = "python"
						result.Duration = time.Since(start).Seconds()
						if branchSwitched {
							exec.Command("git", "checkout", originalBranch).Run()
							exec.Command("git", "stash", "pop").Run()
						}
						results <- scanResult{result: result, index: job.index}
						continue
					default:
						result.Error = "No supported project type found (pom.xml, package.json, go.mod, requirements.txt, or composer.json)"
						result.Duration = time.Since(start).Seconds()
						// Switch back to original branch before returning error
						if branchSwitched {
							exec.Command("git", "checkout", originalBranch).Run()
							exec.Command("git", "stash", "pop").Run()
						}
						results <- scanResult{result: result, index: job.index}
						continue
					}
				}

				// Run appropriate scanner
				switch scannerToUse {
				case "npm":
					if projectType == "" || (projectType != "npm" && projectType != "yarn" && projectType != "pnpm") {
						result.Error = "No package.json found"
					} else {
						result = runNpmAudit(job.repoPath, job.repoName, projectType)
					}
				case "trivy":
					if projectType == "" {
						result.Error = "No supported project files found"
					} else {
						result = runTrivyScan(job.repoPath, job.repoName)
						result.ProjectType = projectType
					}
				case "owasp":
					if projectType != "maven" {
						result.Error = "No pom.xml found (OWASP requires Maven project)"
					} else {
						result = runOwaspScan(job.repoPath, job.repoName)
						result.ProjectType = projectType
					}
				case "govulncheck":
					if projectType != "go" {
						result.Error = "No go.mod found (govulncheck requires Go project)"
					} else {
						result = runGovulncheck(job.repoPath, job.repoName)
						result.ProjectType = projectType
					}
				case "pip-audit":
					if projectType != "python" {
						result.Error = "No Python project found (requires requirements.txt or pyproject.toml)"
					} else {
						result = runPipAudit(job.repoPath, job.repoName)
						result.ProjectType = projectType
					}
				case "composer-audit":
					if projectType != "php" {
						result.Error = "No PHP project found (requires composer.json)"
					} else {
						result = runComposerAudit(job.repoPath, job.repoName)
						result.ProjectType = projectType
					}
				default:
					result.Error = "Unknown scanner type"
				}

				// Restore the scanned branch info (may be lost in scanner functions)
				result.ScannedBranch = scannedBranch
				result.Duration = time.Since(start).Seconds()

				// Switch back to original branch if we switched
				if branchSwitched {
					checkoutCmd := exec.Command("git", "checkout", originalBranch)
					checkoutCmd.Dir = job.repoPath
					checkoutCmd.Run()

					// Try to restore stashed changes
					stashPopCmd := exec.Command("git", "stash", "pop")
					stashPopCmd.Dir = job.repoPath
					stashPopCmd.Run()
				}

				results <- scanResult{result: result, index: job.index}
			}
		}()
	}

	// Send all repos that are being scanned
	for _, repoPath := range repos {
		repoName := filepath.Base(repoPath)
		fmt.Fprintf(w, "REPO_START:%s\n", repoName)
	}
	flusher.Flush()

	// Submit jobs
	go func() {
		for i, repoPath := range repos {
			jobs <- scanJob{
				repoPath:     repoPath,
				repoName:     filepath.Base(repoPath),
				index:        i,
				targetBranch: req.TargetBranch,
			}
		}
		close(jobs)
	}()

	// Collect results in a goroutine
	go func() {
		wg.Wait()
		close(results)
	}()

	// Process results as they come in
	allResults := make([]RepoSecurityResult, total)
	totalCritical, totalHigh, totalMedium, totalLow := 0, 0, 0, 0
	completed := 0
	scanStart := time.Now()

	for res := range results {
		completed++
		allResults[res.index] = res.result

		// Count severities
		for _, f := range res.result.Findings {
			switch f.Severity {
			case "CRITICAL":
				totalCritical++
			case "HIGH":
				totalHigh++
			case "MEDIUM":
				totalMedium++
			case "LOW":
				totalLow++
			}
		}

		// Stream result as JSON (even skipped repos)
		resultJSON, _ := json.Marshal(res.result)
		fmt.Fprintf(w, "REPO_RESULT:%s\n", string(resultJSON))

		// Calculate ETA
		elapsed := time.Since(scanStart).Seconds()
		avgTimePerRepo := elapsed / float64(completed)
		remainingRepos := total - completed
		eta := avgTimePerRepo * float64(remainingRepos)

		fmt.Fprintf(w, "REPO_DONE:%s:%.1f\n", res.result.RepoName, res.result.Duration)
		fmt.Fprintf(w, "SCAN_PROGRESS:%d:%d:%.0f\n", completed, total, eta)
		flusher.Flush()
	}

	// Send summary
	fmt.Fprintf(w, "SCAN_SUMMARY:%d:%d:%d:%d\n", totalCritical, totalHigh, totalMedium, totalLow)
	fmt.Fprintf(w, "SCAN_COMPLETE\n")
	flusher.Flush()
}

func runTrivyScan(repoPath, repoName string) RepoSecurityResult {
	result := RepoSecurityResult{RepoName: repoName}

	// Run trivy fs with JSON output
	cmd := exec.Command("trivy", "fs", "--scanners", "vuln", "--format", "json", "--quiet", ".")
	cmd.Dir = repoPath
	output, err := cmd.Output()

	if err != nil {
		// Trivy returns exit code 1 if vulnerabilities found, but still outputs JSON
		if len(output) == 0 {
			result.Error = fmt.Sprintf("Trivy scan failed: %v", err)
			return result
		}
	}

	// Parse Trivy JSON output
	var trivyResult struct {
		Results []struct {
			Vulnerabilities []struct {
				VulnerabilityID  string `json:"VulnerabilityID"`
				PkgName          string `json:"PkgName"`
				InstalledVersion string `json:"InstalledVersion"`
				FixedVersion     string `json:"FixedVersion"`
				Severity         string `json:"Severity"`
				Description      string `json:"Description"`
			} `json:"Vulnerabilities"`
		} `json:"Results"`
	}

	if err := json.Unmarshal(output, &trivyResult); err != nil {
		result.Error = fmt.Sprintf("Failed to parse Trivy output: %v", err)
		return result
	}

	for _, r := range trivyResult.Results {
		for _, v := range r.Vulnerabilities {
			result.Findings = append(result.Findings, CVEFinding{
				CVE:         v.VulnerabilityID,
				Severity:    strings.ToUpper(v.Severity),
				Package:     v.PkgName,
				Version:     v.InstalledVersion,
				FixedIn:     v.FixedVersion,
				Description: truncateString(v.Description, 200),
			})
		}
	}

	return result
}

func runOwaspScan(repoPath, repoName string) RepoSecurityResult {
	result := RepoSecurityResult{RepoName: repoName}

	// Run OWASP dependency-check via Maven with JSON output
	cmd := exec.Command("mvn",
		"org.owasp:dependency-check-maven:12.1.0:check",
		"-DfailBuildOnCVSS=11", // Never fail build
		"-Dformat=JSON",
		"-DprettyPrint=true",
		"-DskipTestScope=true",
		"-q", // Quiet mode
	)
	cmd.Dir = repoPath
	cmd.Run() // Ignore exit code, we'll parse the output file

	// Find and parse the JSON report
	reportPath := filepath.Join(repoPath, "target", "dependency-check-report.json")
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		result.Error = "OWASP scan completed but no report found. First scan may take 10+ minutes to download NVD database."
		return result
	}

	// Parse OWASP JSON output
	var owaspResult struct {
		Dependencies []struct {
			FileName        string `json:"fileName"`
			Vulnerabilities []struct {
				Name        string `json:"name"`
				Severity    string `json:"severity"`
				Description string `json:"description"`
			} `json:"vulnerabilities"`
		} `json:"dependencies"`
	}

	if err := json.Unmarshal(reportData, &owaspResult); err != nil {
		result.Error = fmt.Sprintf("Failed to parse OWASP report: %v", err)
		return result
	}

	for _, dep := range owaspResult.Dependencies {
		for _, v := range dep.Vulnerabilities {
			severity := strings.ToUpper(v.Severity)
			// OWASP uses different severity names
			switch severity {
			case "CRITICAL", "HIGH", "MEDIUM", "LOW":
				// Keep as is
			case "MODERATE":
				severity = "MEDIUM"
			default:
				severity = "LOW"
			}

			result.Findings = append(result.Findings, CVEFinding{
				CVE:         v.Name,
				Severity:    severity,
				Package:     dep.FileName,
				Description: truncateString(v.Description, 200),
			})
		}
	}

	return result
}

// detectYarnVersion detects Yarn version from package.json packageManager field or yarn --version
func detectYarnVersion(repoPath string) (version string, useCorepack bool) {
	// First check package.json for packageManager field
	pkgPath := filepath.Join(repoPath, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
		var pkg struct {
			PackageManager string `json:"packageManager"`
		}
		if json.Unmarshal(data, &pkg) == nil && pkg.PackageManager != "" {
			// Format: "yarn@4.0.2" or "yarn@4.0.2+sha256.xxx"
			if strings.HasPrefix(pkg.PackageManager, "yarn@") {
				parts := strings.Split(pkg.PackageManager, "@")
				if len(parts) >= 2 {
					ver := strings.Split(parts[1], "+")[0] // Remove hash
					return ver, true                       // Use corepack for packageManager-managed yarn
				}
			}
		}
	}

	// Fallback to global yarn version
	versionCmd := exec.Command("yarn", "--version")
	versionCmd.Dir = repoPath
	if versionOutput, err := versionCmd.Output(); err == nil {
		return strings.TrimSpace(string(versionOutput)), false
	}

	return "1.0.0", false // Default to classic
}

// runNpmAudit runs npm/yarn/pnpm audit for Node.js projects
func runNpmAudit(repoPath, repoName, packageManager string) RepoSecurityResult {
	result := RepoSecurityResult{RepoName: repoName, ProjectType: packageManager}

	var cmd *exec.Cmd
	var isYarnBerry bool
	var useCorepack bool

	switch packageManager {
	case "yarn":
		var yarnVersion string
		yarnVersion, useCorepack = detectYarnVersion(repoPath)

		// Determine if Yarn Berry (v2+)
		isYarnBerry = !strings.HasPrefix(yarnVersion, "1.")

		if isYarnBerry {
			// Yarn Modern (v2+/Berry) - try text output first (more reliable than JSON which often fails)
			if useCorepack {
				cmd = exec.Command("corepack", "yarn", "npm", "audit")
			} else {
				cmd = exec.Command("yarn", "npm", "audit")
			}
		} else {
			// Yarn Classic (v1) - use "yarn audit --json"
			cmd = exec.Command("yarn", "audit", "--json")
		}
	case "pnpm":
		// pnpm audit with JSON output
		cmd = exec.Command("pnpm", "audit", "--json")
	default:
		// npm audit with JSON output
		cmd = exec.Command("npm", "audit", "--json")
	}
	cmd.Dir = repoPath

	// Use CombinedOutput because npm/yarn/pnpm may write to stderr
	// and return non-zero exit code when vulnerabilities are found
	output, err := cmd.CombinedOutput()

	// npm/yarn/pnpm audit returns non-zero exit code if vulnerabilities found
	// but still outputs valid JSON, so we check if there's any output to parse
	if len(output) == 0 {
		if err != nil {
			result.Error = fmt.Sprintf("%s audit failed: %v", packageManager, err)
		} else {
			result.Error = fmt.Sprintf("%s audit returned no output", packageManager)
		}
		return result
	}

	// Parse based on package manager
	if packageManager == "yarn" {
		if isYarnBerry {
			// Parse text output for Yarn Berry (more reliable)
			result = parseYarnBerryTextOutput(output, repoName)
		} else {
			result = parseYarnClassicAuditOutput(output, repoName)
		}
	} else if packageManager == "pnpm" {
		result = parsePnpmAuditOutput(output, repoName)
	} else {
		result = parseNpmAuditOutput(output, repoName)
	}
	result.ProjectType = packageManager

	return result
}

// parseNpmAuditOutput parses npm audit JSON output
func parseNpmAuditOutput(output []byte, repoName string) RepoSecurityResult {
	result := RepoSecurityResult{RepoName: repoName}

	// npm audit JSON structure (v7+)
	var npmResult struct {
		Vulnerabilities map[string]struct {
			Name         string        `json:"name"`
			Severity     string        `json:"severity"`
			Via          []interface{} `json:"via"`
			Effects      []string      `json:"effects"`
			Range        string        `json:"range"`
			FixAvailable interface{}   `json:"fixAvailable"`
		} `json:"vulnerabilities"`
		Metadata struct {
			Vulnerabilities struct {
				Total    int `json:"total"`
				Critical int `json:"critical"`
				High     int `json:"high"`
				Moderate int `json:"moderate"`
				Low      int `json:"low"`
			} `json:"vulnerabilities"`
		} `json:"metadata"`
	}

	if err := json.Unmarshal(output, &npmResult); err != nil {
		// Try older npm audit format
		result = parseNpmAuditOutputLegacy(output, repoName)
		return result
	}

	for pkgName, vuln := range npmResult.Vulnerabilities {
		severity := normalizeSeverity(vuln.Severity)
		cveID, description := extractCVEFromVia(vuln.Via, pkgName)
		fixedIn := extractFixInfo(vuln.FixAvailable)

		result.Findings = append(result.Findings, CVEFinding{
			CVE:         cveID,
			Severity:    severity,
			Package:     pkgName,
			Version:     vuln.Range,
			FixedIn:     fixedIn,
			Description: truncateString(description, 200),
		})
	}

	return result
}

// parseNpmAuditOutputLegacy parses older npm audit JSON format
func parseNpmAuditOutputLegacy(output []byte, repoName string) RepoSecurityResult {
	result := RepoSecurityResult{RepoName: repoName}

	var legacyResult struct {
		Advisories map[string]struct {
			ID                 int    `json:"id"`
			ModuleName         string `json:"module_name"`
			Severity           string `json:"severity"`
			Title              string `json:"title"`
			URL                string `json:"url"`
			VulnerableVersions string `json:"vulnerable_versions"`
			PatchedVersions    string `json:"patched_versions"`
		} `json:"advisories"`
	}

	if err := json.Unmarshal(output, &legacyResult); err != nil {
		result.Error = "Failed to parse npm audit output"
		return result
	}

	for _, adv := range legacyResult.Advisories {
		severity := normalizeSeverity(adv.Severity)

		result.Findings = append(result.Findings, CVEFinding{
			CVE:         fmt.Sprintf("npm:%d", adv.ID),
			Severity:    severity,
			Package:     adv.ModuleName,
			Version:     adv.VulnerableVersions,
			FixedIn:     adv.PatchedVersions,
			Description: truncateString(adv.Title, 200),
		})
	}

	return result
}

// parseYarnBerryTextOutput parses Yarn Berry (v2+/v4) "yarn npm audit" text output
// This is more reliable than JSON output which often fails with HTTP 500 errors
// Format example:
// ├─ next
// │  ├─ ID: 1105949
// │  ├─ Issue: Next.js has a Cache poisoning vulnerability...
// │  ├─ URL: https://github.com/advisories/GHSA-r2fc-ccr8-96c4
// │  ├─ Severity: low
// │  ├─ Vulnerable Versions: >=15.3.0 <15.3.3
// │  │
// │  ├─ Tree Versions
// │  │  └─ 15.3.1
func parseYarnBerryTextOutput(output []byte, repoName string) RepoSecurityResult {
	result := RepoSecurityResult{RepoName: repoName}

	lines := strings.Split(string(output), "\n")
	var currentPackage string
	var currentID string
	var currentIssue string
	var currentURL string
	var currentSeverity string
	var currentVulnVersions string
	var currentTreeVersion string

	for _, line := range lines {
		// Remove tree drawing characters
		cleanLine := strings.TrimSpace(line)
		cleanLine = strings.TrimPrefix(cleanLine, "├─ ")
		cleanLine = strings.TrimPrefix(cleanLine, "│  ├─ ")
		cleanLine = strings.TrimPrefix(cleanLine, "│  │  └─ ")
		cleanLine = strings.TrimPrefix(cleanLine, "│  └─ ")
		cleanLine = strings.TrimPrefix(cleanLine, "└─ ")
		cleanLine = strings.TrimSpace(cleanLine)

		if cleanLine == "" || cleanLine == "│" {
			continue
		}

		// Check for new package entry (package name line - not prefixed with known fields)
		if !strings.Contains(cleanLine, ":") && !strings.HasPrefix(cleanLine, "Tree") && !strings.HasPrefix(cleanLine, "Dependents") && len(cleanLine) > 0 {
			// Save previous entry if we have one
			if currentPackage != "" && currentSeverity != "" {
				cveID := currentID
				if currentURL != "" && strings.Contains(currentURL, "GHSA-") {
					parts := strings.Split(currentURL, "/")
					for _, p := range parts {
						if strings.HasPrefix(p, "GHSA-") {
							cveID = p
							break
						}
					}
				}

				result.Findings = append(result.Findings, CVEFinding{
					CVE:         cveID,
					Severity:    normalizeSeverity(currentSeverity),
					Package:     currentPackage,
					Version:     currentTreeVersion,
					FixedIn:     currentVulnVersions,
					Description: truncateString(currentIssue, 200),
				})
			}

			// Start new package
			currentPackage = cleanLine
			currentID = ""
			currentIssue = ""
			currentURL = ""
			currentSeverity = ""
			currentVulnVersions = ""
			currentTreeVersion = ""
			continue
		}

		// Parse fields
		if strings.HasPrefix(cleanLine, "ID: ") {
			currentID = fmt.Sprintf("GHSA:%s", strings.TrimPrefix(cleanLine, "ID: "))
		} else if strings.HasPrefix(cleanLine, "Issue: ") {
			currentIssue = strings.TrimPrefix(cleanLine, "Issue: ")
		} else if strings.HasPrefix(cleanLine, "URL: ") {
			currentURL = strings.TrimPrefix(cleanLine, "URL: ")
		} else if strings.HasPrefix(cleanLine, "Severity: ") {
			currentSeverity = strings.TrimPrefix(cleanLine, "Severity: ")
		} else if strings.HasPrefix(cleanLine, "Vulnerable Versions: ") {
			currentVulnVersions = strings.TrimPrefix(cleanLine, "Vulnerable Versions: ")
		} else if !strings.HasPrefix(cleanLine, "Tree") && !strings.HasPrefix(cleanLine, "Dependents") {
			// This might be a version number under Tree Versions
			if matched, _ := regexp.MatchString(`^\d+\.\d+`, cleanLine); matched {
				currentTreeVersion = cleanLine
			}
		}
	}

	// Don't forget the last entry
	if currentPackage != "" && currentSeverity != "" {
		cveID := currentID
		if currentURL != "" && strings.Contains(currentURL, "GHSA-") {
			parts := strings.Split(currentURL, "/")
			for _, p := range parts {
				if strings.HasPrefix(p, "GHSA-") {
					cveID = p
					break
				}
			}
		}

		result.Findings = append(result.Findings, CVEFinding{
			CVE:         cveID,
			Severity:    normalizeSeverity(currentSeverity),
			Package:     currentPackage,
			Version:     currentTreeVersion,
			FixedIn:     currentVulnVersions,
			Description: truncateString(currentIssue, 200),
		})
	}

	return result
}

// parseYarnBerryAuditOutput parses Yarn Berry (v2+/v4) "yarn npm audit --json" NDJSON output
// Format: {"value":"pkg","children":{"ID":123,"Issue":"...","Severity":"critical","Vulnerable Versions":"...","Tree Versions":[...]}}
func parseYarnBerryAuditOutput(output []byte, repoName string) RepoSecurityResult {
	result := RepoSecurityResult{RepoName: repoName}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Yarn Berry format
		var entry struct {
			Value    string `json:"value"`
			Children struct {
				ID                 int      `json:"ID"`
				Issue              string   `json:"Issue"`
				URL                string   `json:"URL"`
				Severity           string   `json:"Severity"`
				VulnerableVersions string   `json:"Vulnerable Versions"`
				TreeVersions       []string `json:"Tree Versions"`
				Dependents         []string `json:"Dependents"`
			} `json:"children"`
		}

		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		// Skip if no package name
		if entry.Value == "" {
			continue
		}

		severity := normalizeSeverity(entry.Children.Severity)

		// Extract version from TreeVersions
		version := ""
		if len(entry.Children.TreeVersions) > 0 {
			version = entry.Children.TreeVersions[0]
		}

		// Try to extract CVE from URL
		cveID := fmt.Sprintf("GHSA:%d", entry.Children.ID)
		if strings.Contains(entry.Children.URL, "GHSA-") {
			parts := strings.Split(entry.Children.URL, "/")
			for _, p := range parts {
				if strings.HasPrefix(p, "GHSA-") {
					cveID = p
					break
				}
			}
		}

		result.Findings = append(result.Findings, CVEFinding{
			CVE:         cveID,
			Severity:    severity,
			Package:     entry.Value,
			Version:     version,
			FixedIn:     entry.Children.VulnerableVersions, // Shows what's vulnerable, fix is upgrading out of range
			Description: truncateString(entry.Children.Issue, 200),
		})
	}

	return result
}

// parseYarnClassicAuditOutput parses Yarn Classic (v1) NDJSON format
func parseYarnClassicAuditOutput(output []byte, repoName string) RepoSecurityResult {
	result := RepoSecurityResult{RepoName: repoName}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry struct {
			Type string `json:"type"`
			Data struct {
				Advisory struct {
					ID                 int    `json:"id"`
					ModuleName         string `json:"module_name"`
					Severity           string `json:"severity"`
					Title              string `json:"title"`
					URL                string `json:"url"`
					VulnerableVersions string `json:"vulnerable_versions"`
					PatchedVersions    string `json:"patched_versions"`
				} `json:"advisory"`
			} `json:"data"`
		}

		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entry.Type != "auditAdvisory" {
			continue
		}

		adv := entry.Data.Advisory
		severity := normalizeSeverity(adv.Severity)

		result.Findings = append(result.Findings, CVEFinding{
			CVE:         fmt.Sprintf("yarn:%d", adv.ID),
			Severity:    severity,
			Package:     adv.ModuleName,
			Version:     adv.VulnerableVersions,
			FixedIn:     adv.PatchedVersions,
			Description: truncateString(adv.Title, 200),
		})
	}

	return result
}

// parseYarnAuditOutput is a legacy wrapper - now splits into Berry and Classic
func parseYarnAuditOutput(output []byte, repoName string) RepoSecurityResult {
	result := RepoSecurityResult{RepoName: repoName}

	outputStr := string(output)

	// Try to detect format: Yarn Modern (v2+) outputs similar to npm
	// Check if it starts with { (single JSON object) vs multiple lines (NDJSON)
	trimmedOutput := strings.TrimSpace(outputStr)

	// Yarn Modern (Berry) format - similar to npm audit
	if strings.HasPrefix(trimmedOutput, "{") && !strings.Contains(trimmedOutput, "\n{") {
		// Try npm-like format first (Yarn Modern)
		var yarnModernResult struct {
			Advisories map[string]struct {
				ID                 int    `json:"id"`
				ModuleName         string `json:"module_name"`
				Severity           string `json:"severity"`
				Title              string `json:"title"`
				URL                string `json:"url"`
				VulnerableVersions string `json:"vulnerable_versions"`
				PatchedVersions    string `json:"patched_versions"`
			} `json:"advisories"`
			// Alternative structure for yarn npm audit
			Vulnerabilities map[string]struct {
				Name         string        `json:"name"`
				Severity     string        `json:"severity"`
				Via          []interface{} `json:"via"`
				Range        string        `json:"range"`
				FixAvailable interface{}   `json:"fixAvailable"`
			} `json:"vulnerabilities"`
		}

		if err := json.Unmarshal(output, &yarnModernResult); err == nil {
			// Check if we have vulnerabilities (npm v7+ format)
			if len(yarnModernResult.Vulnerabilities) > 0 {
				for pkgName, vuln := range yarnModernResult.Vulnerabilities {
					severity := normalizeSeverity(vuln.Severity)
					cveID, description := extractCVEFromVia(vuln.Via, pkgName)
					fixedIn := extractFixInfo(vuln.FixAvailable)

					result.Findings = append(result.Findings, CVEFinding{
						CVE:         cveID,
						Severity:    severity,
						Package:     pkgName,
						Version:     vuln.Range,
						FixedIn:     fixedIn,
						Description: truncateString(description, 200),
					})
				}
				return result
			}

			// Check if we have advisories (older format)
			if len(yarnModernResult.Advisories) > 0 {
				for _, adv := range yarnModernResult.Advisories {
					severity := normalizeSeverity(adv.Severity)
					result.Findings = append(result.Findings, CVEFinding{
						CVE:         fmt.Sprintf("yarn:%d", adv.ID),
						Severity:    severity,
						Package:     adv.ModuleName,
						Version:     adv.VulnerableVersions,
						FixedIn:     adv.PatchedVersions,
						Description: truncateString(adv.Title, 200),
					})
				}
				return result
			}
		}
	}

	// Yarn Classic (v1) NDJSON format - each line is a separate JSON object
	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry struct {
			Type string `json:"type"`
			Data struct {
				Advisory struct {
					ID                 int    `json:"id"`
					ModuleName         string `json:"module_name"`
					Severity           string `json:"severity"`
					Title              string `json:"title"`
					URL                string `json:"url"`
					VulnerableVersions string `json:"vulnerable_versions"`
					PatchedVersions    string `json:"patched_versions"`
				} `json:"advisory"`
			} `json:"data"`
		}

		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entry.Type != "auditAdvisory" {
			continue
		}

		adv := entry.Data.Advisory
		severity := normalizeSeverity(adv.Severity)

		result.Findings = append(result.Findings, CVEFinding{
			CVE:         fmt.Sprintf("yarn:%d", adv.ID),
			Severity:    severity,
			Package:     adv.ModuleName,
			Version:     adv.VulnerableVersions,
			FixedIn:     adv.PatchedVersions,
			Description: truncateString(adv.Title, 200),
		})
	}

	return result
}

// normalizeSeverity converts various severity names to standard format
func normalizeSeverity(severity string) string {
	switch strings.ToUpper(severity) {
	case "CRITICAL", "HIGH", "MEDIUM", "LOW":
		return strings.ToUpper(severity)
	case "MODERATE":
		return "MEDIUM"
	default:
		return "LOW"
	}
}

// extractCVEFromVia extracts CVE ID and description from npm's via field
func extractCVEFromVia(via []interface{}, pkgName string) (string, string) {
	cveID := ""
	description := ""

	for _, v := range via {
		if viaMap, ok := v.(map[string]interface{}); ok {
			if source, exists := viaMap["source"]; exists {
				if sourceNum, ok := source.(float64); ok {
					cveID = fmt.Sprintf("GHSA-%d", int(sourceNum))
				}
			}
			if url, exists := viaMap["url"]; exists {
				if urlStr, ok := url.(string); ok {
					// Extract CVE or GHSA from URL
					if strings.Contains(urlStr, "CVE-") {
						parts := strings.Split(urlStr, "/")
						for _, p := range parts {
							if strings.HasPrefix(p, "CVE-") {
								cveID = p
								break
							}
						}
					} else if strings.Contains(urlStr, "GHSA-") {
						parts := strings.Split(urlStr, "/")
						for _, p := range parts {
							if strings.HasPrefix(p, "GHSA-") {
								cveID = p
								break
							}
						}
					}
				}
			}
			if title, exists := viaMap["title"]; exists {
				if titleStr, ok := title.(string); ok {
					description = titleStr
				}
			}
		}
	}

	if cveID == "" {
		cveID = fmt.Sprintf("npm:%s", pkgName)
	}

	return cveID, description
}

// extractFixInfo extracts fix information from FixAvailable field
func extractFixInfo(fixAvailable interface{}) string {
	if fix, ok := fixAvailable.(map[string]interface{}); ok {
		if version, exists := fix["version"]; exists {
			return fmt.Sprintf("%v", version)
		}
	} else if fixAvailable == true {
		return "Update available"
	}
	return ""
}

// parsePnpmAuditOutput parses pnpm audit JSON output
func parsePnpmAuditOutput(output []byte, repoName string) RepoSecurityResult {
	result := RepoSecurityResult{RepoName: repoName}

	// pnpm audit JSON structure (similar to npm)
	var pnpmResult struct {
		Advisories map[string]struct {
			ID                 int    `json:"id"`
			ModuleName         string `json:"module_name"`
			Severity           string `json:"severity"`
			Title              string `json:"title"`
			URL                string `json:"url"`
			VulnerableVersions string `json:"vulnerable_versions"`
			PatchedVersions    string `json:"patched_versions"`
		} `json:"advisories"`
	}

	if err := json.Unmarshal(output, &pnpmResult); err != nil {
		result.Error = "Failed to parse pnpm audit output"
		return result
	}

	for _, adv := range pnpmResult.Advisories {
		severity := normalizeSeverity(adv.Severity)

		result.Findings = append(result.Findings, CVEFinding{
			CVE:         fmt.Sprintf("pnpm:%d", adv.ID),
			Severity:    severity,
			Package:     adv.ModuleName,
			Version:     adv.VulnerableVersions,
			FixedIn:     adv.PatchedVersions,
			Description: truncateString(adv.Title, 200),
		})
	}

	return result
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// runGovulncheck runs govulncheck for Go projects
func runGovulncheck(repoPath, repoName string) RepoSecurityResult {
	result := RepoSecurityResult{RepoName: repoName, ProjectType: "go"}

	// Check if govulncheck is available
	if !checkGovulncheckAvailable() {
		result.Error = "govulncheck not installed. Install with: go install golang.org/x/vuln/cmd/govulncheck@latest"
		return result
	}

	// Run govulncheck with JSON output
	cmd := exec.Command("govulncheck", "-json", "./...")
	cmd.Dir = repoPath
	output, err := cmd.Output()

	// govulncheck returns exit code 3 if vulnerabilities found
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 3 && len(output) == 0 {
				result.Error = fmt.Sprintf("govulncheck failed: %v", err)
				return result
			}
		} else if len(output) == 0 {
			result.Error = fmt.Sprintf("govulncheck failed: %v", err)
			return result
		}
	}

	// Parse JSON output (NDJSON format - one JSON object per line)
	lines := strings.Split(string(output), "\n")
	vulnMap := make(map[string]CVEFinding) // Deduplicate by CVE

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry struct {
			Finding *struct {
				OSV   string `json:"osv"`
				Trace []struct {
					Module  string `json:"module"`
					Version string `json:"version"`
					Package string `json:"package"`
				} `json:"trace"`
			} `json:"finding"`
			OSV *struct {
				ID       string `json:"id"`
				Summary  string `json:"summary"`
				Severity []struct {
					Type  string `json:"type"`
					Score string `json:"score"`
				} `json:"severity"`
				Affected []struct {
					Package struct {
						Name string `json:"name"`
					} `json:"package"`
					Ranges []struct {
						Events []struct {
							Fixed string `json:"fixed"`
						} `json:"events"`
					} `json:"ranges"`
				} `json:"affected"`
			} `json:"osv"`
		}

		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		// Process OSV entries (vulnerability details)
		if entry.OSV != nil {
			osv := entry.OSV
			severity := "MEDIUM" // Default

			// Parse CVSS score if available
			for _, sev := range osv.Severity {
				if sev.Type == "CVSS_V3" {
					if score, err := strconv.ParseFloat(sev.Score, 64); err == nil {
						if score >= 9.0 {
							severity = "CRITICAL"
						} else if score >= 7.0 {
							severity = "HIGH"
						} else if score >= 4.0 {
							severity = "MEDIUM"
						} else {
							severity = "LOW"
						}
					}
				}
			}

			// Get package name and fixed version
			pkgName := ""
			fixedIn := ""
			if len(osv.Affected) > 0 {
				pkgName = osv.Affected[0].Package.Name
				for _, r := range osv.Affected[0].Ranges {
					for _, ev := range r.Events {
						if ev.Fixed != "" {
							fixedIn = ev.Fixed
						}
					}
				}
			}

			vulnMap[osv.ID] = CVEFinding{
				CVE:         osv.ID,
				Severity:    severity,
				Package:     pkgName,
				FixedIn:     fixedIn,
				Description: truncateString(osv.Summary, 200),
			}
		}

		// Process finding entries (actual usage in code)
		if entry.Finding != nil && len(entry.Finding.Trace) > 0 {
			trace := entry.Finding.Trace[0]
			if existing, ok := vulnMap[entry.Finding.OSV]; ok {
				existing.Version = trace.Version
				if existing.Package == "" {
					existing.Package = trace.Module
				}
				vulnMap[entry.Finding.OSV] = existing
			}
		}
	}

	// Convert map to slice
	for _, finding := range vulnMap {
		result.Findings = append(result.Findings, finding)
	}

	return result
}

// runPipAudit runs pip-audit for Python projects
func runPipAudit(repoPath, repoName string) RepoSecurityResult {
	result := RepoSecurityResult{RepoName: repoName, ProjectType: "python"}

	// Check if pip-audit is available
	if !checkPipAuditAvailable() {
		result.Error = "pip-audit not installed. Install with: pip install pip-audit"
		return result
	}

	// Determine which requirements file to use
	var requirementsArg string
	if _, err := os.Stat(filepath.Join(repoPath, "requirements.txt")); err == nil {
		requirementsArg = "-r requirements.txt"
	} else if _, err := os.Stat(filepath.Join(repoPath, "pyproject.toml")); err == nil {
		// pip-audit can handle pyproject.toml directly in the directory
		requirementsArg = ""
	} else {
		result.Error = "No requirements.txt or pyproject.toml found"
		return result
	}

	// Run pip-audit with JSON output
	var cmd *exec.Cmd
	if requirementsArg != "" {
		cmd = exec.Command("pip-audit", "-r", "requirements.txt", "--format", "json", "--progress-spinner=off")
	} else {
		cmd = exec.Command("pip-audit", "--format", "json", "--progress-spinner=off")
	}
	cmd.Dir = repoPath
	output, err := cmd.Output()

	// pip-audit returns exit code 1 if vulnerabilities found
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 1 && len(output) == 0 {
				// Check stderr for more info
				if len(exitErr.Stderr) > 0 {
					result.Error = fmt.Sprintf("pip-audit failed: %s", string(exitErr.Stderr))
				} else {
					result.Error = fmt.Sprintf("pip-audit failed: %v", err)
				}
				return result
			}
		} else if len(output) == 0 {
			result.Error = fmt.Sprintf("pip-audit failed: %v", err)
			return result
		}
	}

	// Parse pip-audit JSON output
	var pipResult struct {
		Dependencies []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Vulns   []struct {
				ID          string   `json:"id"`
				FixVersions []string `json:"fix_versions"`
				Description string   `json:"description"`
			} `json:"vulns"`
		} `json:"dependencies"`
	}

	// pip-audit outputs an array directly
	var vulnArray []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Vulns   []struct {
			ID          string   `json:"id"`
			FixVersions []string `json:"fix_versions"`
			Description string   `json:"description"`
		} `json:"vulns"`
	}

	// Try parsing as array first (newer pip-audit format)
	if err := json.Unmarshal(output, &vulnArray); err == nil {
		for _, dep := range vulnArray {
			for _, vuln := range dep.Vulns {
				severity := determinePythonSeverity(vuln.ID)
				fixedIn := ""
				if len(vuln.FixVersions) > 0 {
					fixedIn = vuln.FixVersions[len(vuln.FixVersions)-1] // Latest fix version
				}

				result.Findings = append(result.Findings, CVEFinding{
					CVE:         vuln.ID,
					Severity:    severity,
					Package:     dep.Name,
					Version:     dep.Version,
					FixedIn:     fixedIn,
					Description: truncateString(vuln.Description, 200),
				})
			}
		}
		return result
	}

	// Try parsing as object (older format)
	if err := json.Unmarshal(output, &pipResult); err != nil {
		result.Error = fmt.Sprintf("Failed to parse pip-audit output: %v", err)
		return result
	}

	for _, dep := range pipResult.Dependencies {
		for _, vuln := range dep.Vulns {
			severity := determinePythonSeverity(vuln.ID)
			fixedIn := ""
			if len(vuln.FixVersions) > 0 {
				fixedIn = vuln.FixVersions[len(vuln.FixVersions)-1]
			}

			result.Findings = append(result.Findings, CVEFinding{
				CVE:         vuln.ID,
				Severity:    severity,
				Package:     dep.Name,
				Version:     dep.Version,
				FixedIn:     fixedIn,
				Description: truncateString(vuln.Description, 200),
			})
		}
	}

	return result
}

// determinePythonSeverity determines severity from CVE/PYSEC ID
// Since pip-audit doesn't always include severity, we default to MEDIUM
// but could be enhanced with OSV API lookup
func determinePythonSeverity(id string) string {
	// pip-audit doesn't provide severity directly
	// Could be enhanced to lookup from OSV API
	// For now, use heuristics based on ID prefix
	if strings.HasPrefix(id, "GHSA-") {
		// GitHub Security Advisories are usually at least MEDIUM
		return "MEDIUM"
	}
	if strings.HasPrefix(id, "CVE-") || strings.HasPrefix(id, "PYSEC-") {
		return "MEDIUM"
	}
	return "LOW"
}

// runComposerAudit runs composer audit for PHP projects
func runComposerAudit(repoPath, repoName string) RepoSecurityResult {
	result := RepoSecurityResult{RepoName: repoName, ProjectType: "php"}

	// Check if composer is available
	if !checkComposerAvailable() {
		result.Error = "Composer not installed. Install from: https://getcomposer.org/download/"
		return result
	}

	// Check if composer.json exists
	composerPath := filepath.Join(repoPath, "composer.json")
	if _, err := os.Stat(composerPath); os.IsNotExist(err) {
		result.Error = "No composer.json found"
		return result
	}

	// Run composer audit with JSON output
	cmd := exec.Command("composer", "audit", "--format=json")
	cmd.Dir = repoPath
	output, err := cmd.Output()

	// composer audit returns exit code 1 if vulnerabilities found
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 1 && len(output) == 0 {
				// Check stderr for more info
				if len(exitErr.Stderr) > 0 {
					result.Error = fmt.Sprintf("composer audit failed: %s", string(exitErr.Stderr))
				} else {
					result.Error = fmt.Sprintf("composer audit failed: %v", err)
				}
				return result
			}
		} else if len(output) == 0 {
			result.Error = fmt.Sprintf("composer audit failed: %v", err)
			return result
		}
	}

	// Parse composer audit JSON output
	var auditResult struct {
		Advisories map[string][]struct {
			AdvisoryID       string `json:"advisoryId"`
			PackageName      string `json:"packageName"`
			AffectedVersions string `json:"affectedVersions"`
			Title            string `json:"title"`
			CVE              string `json:"cve"`
			Link             string `json:"link"`
			ReportedAt       string `json:"reportedAt"`
			Severity         string `json:"severity"`
		} `json:"advisories"`
	}

	if err := json.Unmarshal(output, &auditResult); err != nil {
		// Check if it's empty (no vulnerabilities)
		if len(output) == 0 || string(output) == "{}" || string(output) == "{\"advisories\":[]}" {
			return result // No vulnerabilities
		}
		result.Error = fmt.Sprintf("Failed to parse composer audit output: %v", err)
		return result
	}

	// Process advisories
	for packageName, advisories := range auditResult.Advisories {
		for _, advisory := range advisories {
			cveID := advisory.CVE
			if cveID == "" {
				cveID = advisory.AdvisoryID
			}

			severity := strings.ToUpper(advisory.Severity)
			if severity == "" {
				severity = "MEDIUM"
			}

			result.Findings = append(result.Findings, CVEFinding{
				CVE:         cveID,
				Severity:    severity,
				Package:     packageName,
				Version:     advisory.AffectedVersions,
				Description: truncateString(advisory.Title, 200),
			})
		}
	}

	return result
}

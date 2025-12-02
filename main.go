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
}

// AnalysisResult holds the result of analyzing a single repo
type AnalysisResult struct {
	Index    int
	RepoName string
	Output   string
	Success  bool
	Duration time.Duration
}

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
	fmt.Fprintf(w, "Found %d projects. Starting parallel analysis for Spring Boot %s...\n", len(repos), req.TargetVersion)
	fmt.Fprintf(w, "(Processing in background, results will appear as each project completes)\n\n")
	flusher.Flush()

	overallStart := time.Now()

	// 2. Determine Recipe
	var recipe string
	cleanVersion := strings.ReplaceAll(req.TargetVersion, ".", "_")

	if strings.HasPrefix(req.TargetVersion, "3.") {
		recipe = fmt.Sprintf("org.openrewrite.java.spring.boot3.UpgradeSpringBoot_%s", cleanVersion)
	} else if strings.HasPrefix(req.TargetVersion, "2.") {
		recipe = fmt.Sprintf("org.openrewrite.java.spring.boot2.UpgradeSpringBoot_%s", cleanVersion)
	} else {
		recipe = fmt.Sprintf("org.openrewrite.java.spring.boot%c.UpgradeSpringBoot_%s", req.TargetVersion[0], cleanVersion)
	}

	// Plugin versions
	pluginVersion := "6.24.0"
	recipeVersion := "6.19.0"

	// 3. Run Analysis in Parallel
	resultChan := make(chan AnalysisResult, len(repos))

	for i, repo := range repos {
		go func(index int, repoPath string) {
			result := analyzeRepo(index, repoPath, recipe, pluginVersion, recipeVersion)
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
func analyzeRepo(index int, repoPath, recipe, pluginVersion, recipeVersion string) AnalysisResult {
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
		fmt.Sprintf("-Drewrite.recipeArtifactCoordinates=org.openrewrite.recipe:rewrite-spring:%s", recipeVersion),
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
			output.WriteString("Changes detected:\n")
			output.WriteString(string(content))
			output.WriteString("\n")
		} else {
			output.WriteString("No changes required (or empty patch).\n")
		}
	} else {
		if strings.Contains(string(cmdOutput), "No changes") {
			output.WriteString("No changes required.\n")
		} else {
			output.WriteString("Analysis finished (no patch file generated).\n")
		}
	}

	return AnalysisResult{Index: index, RepoName: repoName, Output: output.String(), Success: true, Duration: time.Since(startTime)}
}

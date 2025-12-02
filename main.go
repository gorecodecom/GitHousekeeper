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

	fmt.Fprintf(w, "Found %d projects. Starting analysis for Spring Boot %s...\n\n", len(repos), req.TargetVersion)
	flusher.Flush()

	// 2. Determine Recipe
	// Mapping Target Version -> OpenRewrite Recipe
	// Using latest stable rewrite-spring recipes
	// See: https://docs.openrewrite.org/recipes/java/spring/boot3
	var recipe string

	// Dynamic Recipe Construction
	// We assume the recipe follows the pattern: org.openrewrite.java.spring.boot3.UpgradeSpringBoot_X_Y
	// e.g. UpgradeSpringBoot_3_3, UpgradeSpringBoot_3_4
	// For 4.x, it might be boot4.UpgradeSpringBoot_4_0 (Need to verify if 4.x recipes exist yet in rewrite-spring)
	// As of now, Spring Boot 4 is not out, but user asked for "4er Versionen".
	// If they mean 3.4, 3.5 etc, the pattern holds.

	cleanVersion := strings.ReplaceAll(req.TargetVersion, ".", "_")

	if strings.HasPrefix(req.TargetVersion, "3.") {
		recipe = fmt.Sprintf("org.openrewrite.java.spring.boot3.UpgradeSpringBoot_%s", cleanVersion)
	} else if strings.HasPrefix(req.TargetVersion, "2.") {
		recipe = fmt.Sprintf("org.openrewrite.java.spring.boot2.UpgradeSpringBoot_%s", cleanVersion)
	} else {
		// Fallback for future versions (e.g. 4.0) - assuming naming convention persists
		// Note: This might fail if the recipe doesn't exist in the plugin version used.
		recipe = fmt.Sprintf("org.openrewrite.java.spring.boot%c.UpgradeSpringBoot_%s", req.TargetVersion[0], cleanVersion)
	}

	// 3. Run Analysis per Repo
	for _, repo := range repos {
		repoName := filepath.Base(repo)
		fmt.Fprintf(w, ">>> Analyzing %s...\n", repoName)
		flusher.Flush()

		// Check if it's a Maven project
		if _, err := os.Stat(filepath.Join(repo, "pom.xml")); os.IsNotExist(err) {
			fmt.Fprintf(w, "Skipping (no pom.xml)\n")
			continue
		}

		// Construct Maven Command
		// We use the rewrite-maven-plugin directly from command line
		// Command: mvn -U org.openrewrite.maven:rewrite-maven-plugin:RELEASE:dryRun
		//          -Drewrite.recipeArtifactCoordinates=org.openrewrite.recipe:rewrite-spring:RELEASE
		//          -Drewrite.activeRecipes=<recipe>

		// Note: We use "RELEASE" to get latest, or pin specific versions if needed.
		// Pinning is safer for stability.
		pluginVersion := "5.41.0" // Recent stable as of late 2024
		recipeVersion := "5.18.0" // Recent stable rewrite-spring

		cmd := exec.Command("mvn",
			"-U", // Force update snapshots
			"-B", // Batch mode
			fmt.Sprintf("org.openrewrite.maven:rewrite-maven-plugin:%s:dryRun", pluginVersion),
			fmt.Sprintf("-Drewrite.recipeArtifactCoordinates=org.openrewrite.recipe:rewrite-spring:%s", recipeVersion),
			fmt.Sprintf("-Drewrite.activeRecipes=%s", recipe),
		)
		cmd.Dir = repo

		// Capture output
		// We want to capture stdout/stderr to show progress or errors
		// But mainly we are interested in the generated patch file or the "Changes" log.
		// dryRun usually prints a summary to stdout and generates target/rewrite/rewrite.patch

		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Fprintf(w, "Error running OpenRewrite: %v\n", err)
			// Print last few lines of output for debug
			lines := strings.Split(string(output), "\n")
			start := len(lines) - 10
			if start < 0 {
				start = 0
			}
			for _, line := range lines[start:] {
				fmt.Fprintf(w, "  %s\n", line)
			}
			flusher.Flush()
			continue
		}

		// Check for patch file
		patchFile := filepath.Join(repo, "target", "rewrite", "rewrite.patch")
		if _, err := os.Stat(patchFile); err == nil {
			// Patch exists -> Changes found
			content, err := os.ReadFile(patchFile)
			if err == nil && len(content) > 0 {
				fmt.Fprintf(w, "Changes detected:\n")
				fmt.Fprintf(w, "%s\n", string(content))
			} else {
				fmt.Fprintf(w, "No changes required (or empty patch).\n")
			}
		} else {
			// No patch file -> No changes or failed to generate
			// Check output for "No changes" message
			if strings.Contains(string(output), "No changes") {
				fmt.Fprintf(w, "No changes required.\n")
			} else {
				fmt.Fprintf(w, "Analysis finished (no patch file generated).\n")
			}
		}

		fmt.Fprintf(w, "\n")
		flusher.Flush()
	}
}

package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"

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
	PomReplacements     []logic.Replacement
	ProjectReplacements []logic.Replacement
}

func main() {
	// Setup File Server
	// We strip "assets" prefix because embed.FS includes the directory structure
	fsys, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err)
	}
	http.Handle("/", http.FileServer(http.FS(fsys)))

	// API
	http.HandleFunc("/api/run", handleRun)

	port := "8080"
	url := "http://localhost:" + port

	fmt.Printf("Starte Web-Interface auf %s ...\n", url)
	
	// Open Browser
	go openBrowser(url)

	err = http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Printf("Fehler beim Starten des Servers: %v\n", err)
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
		fmt.Fprintf(w, "Keine Git-Projekte unter '%s' gefunden.\n", req.RootPath)
		flusher.Flush()
		return
	}

	fmt.Fprintf(w, "Gefunden: %d Projekte\n", len(repos))
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

package logic

import (
	"bufio"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// DashboardStats holds the aggregated data for the dashboard
type DashboardStats struct {
	TotalRepos      int            `json:"totalRepos"`
	AvgHealthScore  int            `json:"avgHealthScore"`
	TotalTodos      int            `json:"totalTodos"`
	TopDependencies []NameCount    `json:"topDependencies"`
	RepoDetails     []RepoHealth   `json:"repoDetails"`
	SpringVersions  map[string]int `json:"springVersions"` // e.g. "3.2.0" -> 5
}

type NameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type RepoHealth struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	HealthScore    int    `json:"healthScore"`
	TodoCount      int    `json:"todoCount"`
	SpringBootVer  string `json:"springBootVer"`
	JavaVersion    string `json:"javaVersion"`
	LastCommit     string `json:"lastCommit"`
	HasBuildErrors bool   `json:"hasBuildErrors"`
	// New fields for enhanced dashboard
	Framework      string `json:"framework"`      // React, Angular, Vue, Next.js, Express, Spring Boot, Go, Python, PHP, etc.
	NodeVersion    string `json:"nodeVersion"`    // Node.js version from package.json or .nvmrc
	GoVersion      string `json:"goVersion"`      // Go version from go.mod
	PythonVersion  string `json:"pythonVersion"`  // Python version from .python-version or pyproject.toml
	PhpVersion     string `json:"phpVersion"`     // PHP version from composer.json
	OutdatedDeps   int    `json:"outdatedDeps"`   // Count of outdated dependencies
	ProjectType    string `json:"projectType"`    // "maven", "npm", "yarn", "pnpm", "go", "python", "php", "unknown"
}

// StreamDashboardStats scans and streams results in real-time
func StreamDashboardStats(rootPath string, excluded []string, onResult func(interface{})) {
	repos := FindGitRepos(rootPath, excluded)

	// 1. Send Init Event
	onResult(map[string]interface{}{
		"type":       "init",
		"totalRepos": len(repos),
	})

	if len(repos) == 0 {
		return
	}

	var wg sync.WaitGroup
	var mu sync.Mutex // Mutex to protect concurrent writes
	// Limit concurrency to avoid overwhelming the system/Maven
	sem := make(chan struct{}, 5)

	for _, repo := range repos {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire token
			defer func() { <-sem }() // Release token

			health, deps := analyzeRepoHealth(path)

			// Send Repo Result - protected by mutex
			mu.Lock()
			onResult(map[string]interface{}{
				"type": "repo",
				"data": health,
				"deps": deps,
			})
			mu.Unlock()
		}(repo)
	}

	wg.Wait()

	// Send Done Event
	onResult(map[string]interface{}{
		"type": "done",
	})
}

func analyzeRepoHealth(path string) (RepoHealth, []string) {
	repoName := filepath.Base(path)
	health := RepoHealth{
		Name:        repoName,
		Path:        path,
		HealthScore: 100,
	}

	var dependencies []string

	// 1. Get Last Commit Date
	// git log -1 --format=%cd --date=short
	cmd := exec.Command("git", "log", "-1", "--format=%cd", "--date=short")
	cmd.Dir = path
	out, err := cmd.Output()
	if err == nil {
		health.LastCommit = strings.TrimSpace(string(out))
	} else {
		health.LastCommit = "-"
	}

	// 2. Scan for TODOs/FIXMEs
	err = filepath.WalkDir(path, func(filePath string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "target" || d.Name() == "node_modules" || d.Name() == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(filePath))
		if ext == ".java" || ext == ".xml" || ext == ".md" || ext == ".properties" || ext == ".yml" || ext == ".yaml" || ext == ".js" || ext == ".ts" {
			count := countKeywordsInFile(filePath, []string{"TODO", "FIXME"})
			health.TodoCount += count
		}
		return nil
	})

	todoPenalty := health.TodoCount / 5
	if todoPenalty > 20 {
		todoPenalty = 20
	}
	health.HealthScore -= todoPenalty

	// 3. Robust Scan: Use Maven Effective POM to resolve versions (handles BOMs, Properties, Parent inheritance)
	// This is slower but accurate.
	sbVer, javaVer, err := getEffectivePomInfo(path)
	if err == nil {
		if sbVer != "" {
			health.SpringBootVer = sbVer
		}
		if javaVer != "" {
			health.JavaVersion = javaVer
		}
	}

	// 4. Parse POM (Fast) for Dependencies (and fallback for versions if Maven failed)
	pomPath := filepath.Join(path, "pom.xml")
	if _, err := os.Stat(pomPath); err == nil {
		project, err := ParsePOM(pomPath)
		if err == nil {
			// Fallback: Check Parent for Spring Boot if not found by effective pom
			if health.SpringBootVer == "" && strings.Contains(project.Parent.GroupId, "spring-boot") {
				health.SpringBootVer = project.Parent.Version
			}

			// Fallback: Check Properties for Java Version
			if health.JavaVersion == "" {
				if project.JavaVersion != "" {
					health.JavaVersion = project.JavaVersion
				} else if project.CompilerSrc != "" {
					health.JavaVersion = project.CompilerSrc
				}
			}

			// Collect Dependencies and check for Spring Boot if not found in parent
			for _, dep := range project.Dependencies {
				// Filter out standard Spring Boot starters to make the stats more interesting
				if dep.GroupId != "org.springframework.boot" {
					displayDep := dep.ArtifactId
					dependencies = append(dependencies, displayDep)
				}

				if health.SpringBootVer == "" && dep.GroupId == "org.springframework.boot" {
					// Try to guess version from dependency if explicit
					if dep.Version != "" && !strings.Contains(dep.Version, "$") {
						health.SpringBootVer = dep.Version
					}
				}

				if dep.ArtifactId == "junit" {
					health.HealthScore -= 5
				}
			}

			// Penalize old Spring Boot
			if health.SpringBootVer != "" {
				if strings.HasPrefix(health.SpringBootVer, "2.") {
					health.HealthScore -= 20
				} else if strings.HasPrefix(health.SpringBootVer, "1.") {
					health.HealthScore -= 40
				}
			}
		}
	}

	if health.HealthScore < 0 {
		health.HealthScore = 0
	}

	// 5. Detect Project Type and Framework
	health.ProjectType, health.Framework = detectProjectTypeAndFramework(path)

	// 6. Get Runtime Version and Dependencies based on project type
	switch health.ProjectType {
	case "npm", "yarn", "pnpm":
		health.NodeVersion = getNodeVersion(path)
		// Collect Node.js dependencies
		nodeDeps := getNodeDependencies(path)
		dependencies = append(dependencies, nodeDeps...)
	case "go":
		health.GoVersion = getGoVersion(path)
		// Collect Go dependencies
		goDeps := getGoDependencies(path)
		dependencies = append(dependencies, goDeps...)
	case "python":
		health.PythonVersion = getPythonVersion(path)
		// Collect Python dependencies
		pythonDeps := getPythonDependencies(path)
		dependencies = append(dependencies, pythonDeps...)
	case "php":
		health.PhpVersion = getPhpVersion(path)
		// Collect PHP dependencies
		phpDeps := getPhpDependencies(path)
		dependencies = append(dependencies, phpDeps...)
	}

	// 7. Check for Outdated Dependencies
	health.OutdatedDeps = getOutdatedDependencyCount(path, health.ProjectType)

	// Set Framework to Spring Boot if detected
	if health.SpringBootVer != "" && health.Framework == "" {
		health.Framework = "Spring Boot"
	}

	return health, dependencies
}

func countKeywordsInFile(path string, keywords []string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		for _, kw := range keywords {
			if strings.Contains(line, kw) {
				count++
			}
		}
	}
	return count
}

// MinimalProjectSimple is used to parse POM files for dashboard stats
type MinimalProjectSimple struct {
	XMLName      xml.Name          `xml:"project"`
	Parent       Parent            `xml:"parent"`
	Properties   map[string]string `xml:"properties"` // Map for properties
	Dependencies []Dep             `xml:"dependencies>dependency"`
}

// Custom Unmarshal for Properties to handle dynamic tag names
func (p *MinimalProjectSimple) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	type Alias MinimalProjectSimple
	aux := &Alias{
		Properties: make(map[string]string),
	}

	// We need to decode the whole element into the alias struct
	// But standard xml unmarshal doesn't support map[string]string for arbitrary tags automatically.
	// So we need a custom approach for properties.
	// Let's simplify: Just read properties as a raw struct with common fields for now,
	// or use a separate struct for properties.
	// To avoid complexity, I will revert to explicit fields for Java Version.
	return d.DecodeElement(aux, &start)
}

// Re-defining MinimalProjectSimple to use specific fields for common properties to avoid map complexity
type MinimalProjectSimpleFixed struct {
	XMLName      xml.Name `xml:"project"`
	Parent       Parent   `xml:"parent"`
	Dependencies []Dep    `xml:"dependencies>dependency"`
	JavaVersion  string   `xml:"properties>java.version"`
	CompilerSrc  string   `xml:"properties>maven.compiler.source"`
}

type Parent struct {
	GroupId    string `xml:"groupId"`
	ArtifactId string `xml:"artifactId"`
	Version    string `xml:"version"`
}

type Dep struct {
	GroupId    string `xml:"groupId"`
	ArtifactId string `xml:"artifactId"`
	Version    string `xml:"version"`
}

func ParsePOM(path string) (*MinimalProjectSimpleFixed, error) {
	xmlFile, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer xmlFile.Close()

	byteValue, _ := io.ReadAll(xmlFile)

	var project MinimalProjectSimpleFixed
	err = xml.Unmarshal(byteValue, &project)
	if err != nil {
		return nil, err
	}

	// Map fields to a common structure if needed, but here we just return the struct.
	// The caller expects Properties map? No, I updated caller to check fields.
	// Wait, I updated caller to check `project.Properties["java.version"]`.
	// I need to update caller to check `project.JavaVersion` instead.
	return &project, nil
}

func getEffectivePomInfo(dir string) (springVer, javaVer string, err error) {
	// Use help:effective-pom to see the resolved versions
	// Add timeout context to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if strings.Contains(strings.ToLower(os.Getenv("OS")), "windows") {
		cmd = exec.CommandContext(ctx, "cmd", "/C", "mvn", "help:effective-pom", "-N")
	} else {
		cmd = exec.CommandContext(ctx, "mvn", "help:effective-pom", "-N")
	}
	cmd.Dir = dir

	// Capture output
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", "", fmt.Errorf("timeout after 30 seconds")
		}
		return "", "", err
	}
	output := string(outputBytes)

	// 1. Find Spring Boot Version
	// Look for spring-boot dependency version
	// Try to find:  <groupId>org.springframework.boot</groupId> ... <version>...</version>
	// We use a regex that is loose on whitespace
	re := regexp.MustCompile(`(?s)<groupId>org\.springframework\.boot</groupId>\s*<artifactId>spring-boot(-starter)?</artifactId>\s*<version>(.*?)</version>`)
	match := re.FindStringSubmatch(output)
	if len(match) > 2 {
		springVer = match[2]
	} else {
		// Fallback: Check for spring-boot-dependencies in dependencyManagement
		reDep := regexp.MustCompile(`(?s)<artifactId>spring-boot-dependencies</artifactId>\s*<version>(.*?)</version>`)
		matchDep := reDep.FindStringSubmatch(output)
		if len(matchDep) > 1 {
			springVer = matchDep[1]
		}
	}

	// 2. Find Java Version
	// Look for <maven.compiler.source> or <java.version> in the effective pom output
	// Effective POM usually resolves properties, so we might see <maven.compiler.source>17</maven.compiler.source>
	reJava := regexp.MustCompile(`(?s)<maven\.compiler\.source>(.*?)</maven\.compiler\.source>`)
	matchJava := reJava.FindStringSubmatch(output)
	if len(matchJava) > 1 {
		javaVer = matchJava[1]
	} else {
		reJava2 := regexp.MustCompile(`(?s)<java\.version>(.*?)</java\.version>`)
		matchJava2 := reJava2.FindStringSubmatch(output)
		if len(matchJava2) > 1 {
			javaVer = matchJava2[1]
		}
	}

	return springVer, javaVer, nil
}

// detectProjectTypeAndFramework detects the project type (npm, yarn, pnpm, maven, go, python) and framework
func detectProjectTypeAndFramework(repoPath string) (projectType string, framework string) {
	// Check for Maven project
	if _, err := os.Stat(filepath.Join(repoPath, "pom.xml")); err == nil {
		projectType = "maven"
		// Framework detection for Maven projects is handled via Spring Boot detection
		return projectType, ""
	}

	// Check for Go project
	if _, err := os.Stat(filepath.Join(repoPath, "go.mod")); err == nil {
		projectType = "go"
		framework = detectGoFramework(repoPath)
		return projectType, framework
	}

	// Check for Python project
	if isPythonProject(repoPath) {
		projectType = "python"
		framework = detectPythonFramework(repoPath)
		return projectType, framework
	}

	// Check for PHP project (composer.json)
	if isPhpProject(repoPath) {
		projectType = "php"
		framework = detectPhpFramework(repoPath)
		return projectType, framework
	}

	// Check for pnpm
	if _, err := os.Stat(filepath.Join(repoPath, "pnpm-lock.yaml")); err == nil {
		projectType = "pnpm"
	} else if _, err := os.Stat(filepath.Join(repoPath, "yarn.lock")); err == nil {
		// Check for Yarn
		projectType = "yarn"
	} else if _, err := os.Stat(filepath.Join(repoPath, "package-lock.json")); err == nil {
		// Check for npm
		projectType = "npm"
	} else if _, err := os.Stat(filepath.Join(repoPath, "package.json")); err == nil {
		// Fallback to npm if package.json exists
		projectType = "npm"
	} else {
		return "unknown", ""
	}

	// Detect JavaScript/TypeScript framework from package.json
	framework = detectJSFramework(repoPath)

	return projectType, framework
}

// PackageJSON represents the structure of package.json for framework detection
type PackageJSON struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Engines         struct {
		Node string `json:"node"`
	} `json:"engines"`
}

// detectJSFramework reads package.json and determines the framework
func detectJSFramework(repoPath string) string {
	pkgPath := filepath.Join(repoPath, "package.json")
	file, err := os.Open(pkgPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	var pkg PackageJSON
	if err := json.NewDecoder(file).Decode(&pkg); err != nil {
		return ""
	}

	// Merge dependencies for checking
	allDeps := make(map[string]bool)
	for dep := range pkg.Dependencies {
		allDeps[dep] = true
	}
	for dep := range pkg.DevDependencies {
		allDeps[dep] = true
	}

	// Framework detection priority (more specific first)
	if allDeps["next"] {
		return "Next.js"
	}
	if allDeps["nuxt"] {
		return "Nuxt.js"
	}
	if allDeps["@angular/core"] {
		return "Angular"
	}
	if allDeps["vue"] {
		return "Vue.js"
	}
	if allDeps["react"] {
		// Check for specific React frameworks
		if allDeps["gatsby"] {
			return "Gatsby"
		}
		if allDeps["remix"] || allDeps["@remix-run/react"] {
			return "Remix"
		}
		return "React"
	}
	if allDeps["svelte"] || allDeps["@sveltejs/kit"] {
		return "Svelte"
	}
	if allDeps["express"] {
		return "Express"
	}
	if allDeps["fastify"] {
		return "Fastify"
	}
	if allDeps["nest"] || allDeps["@nestjs/core"] {
		return "NestJS"
	}
	if allDeps["koa"] {
		return "Koa"
	}
	if allDeps["electron"] {
		return "Electron"
	}

	return ""
}

// getNodeVersion reads the Node.js version from package.json engines or .nvmrc
func getNodeVersion(repoPath string) string {
	// First, try to read from .nvmrc
	nvmrcPath := filepath.Join(repoPath, ".nvmrc")
	if data, err := os.ReadFile(nvmrcPath); err == nil {
		version := strings.TrimSpace(string(data))
		// Clean up version string (remove 'v' prefix if present)
		version = strings.TrimPrefix(version, "v")
		if version != "" {
			return version
		}
	}

	// Try .node-version (used by nodenv, volta, etc.)
	nodeVersionPath := filepath.Join(repoPath, ".node-version")
	if data, err := os.ReadFile(nodeVersionPath); err == nil {
		version := strings.TrimSpace(string(data))
		version = strings.TrimPrefix(version, "v")
		if version != "" {
			return version
		}
	}

	// Fall back to package.json engines.node
	pkgPath := filepath.Join(repoPath, "package.json")
	file, err := os.Open(pkgPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	var pkg PackageJSON
	if err := json.NewDecoder(file).Decode(&pkg); err != nil {
		return ""
	}

	if pkg.Engines.Node != "" {
		return pkg.Engines.Node
	}

	return ""
}

// getNodeDependencies collects top dependencies from package.json
func getNodeDependencies(repoPath string) []string {
	pkgPath := filepath.Join(repoPath, "package.json")
	file, err := os.Open(pkgPath)
	if err != nil {
		return nil
	}
	defer file.Close()

	var pkg PackageJSON
	if err := json.NewDecoder(file).Decode(&pkg); err != nil {
		return nil
	}

	var deps []string
	// Collect production dependencies (limit to top 10 to avoid noise)
	count := 0
	for dep := range pkg.Dependencies {
		// Skip common framework dependencies that are already tracked
		if dep == "react" || dep == "next" || dep == "vue" || dep == "angular" || dep == "svelte" {
			continue
		}
		deps = append(deps, dep)
		count++
		if count >= 10 {
			break
		}
	}
	return deps
}

// getGoDependencies collects dependencies from go.mod
func getGoDependencies(repoPath string) []string {
	goModPath := filepath.Join(repoPath, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return nil
	}

	var deps []string
	lines := strings.Split(string(data), "\n")
	inRequire := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Handle require block
		if strings.HasPrefix(line, "require (") || strings.HasPrefix(line, "require(") {
			inRequire = true
			continue
		}
		if inRequire && line == ")" {
			inRequire = false
			continue
		}

		// Handle single require or require block entries
		if inRequire || strings.HasPrefix(line, "require ") {
			var depLine string
			if strings.HasPrefix(line, "require ") {
				depLine = strings.TrimPrefix(line, "require ")
			} else {
				depLine = line
			}

			// Skip indirect dependencies
			if strings.Contains(depLine, "// indirect") {
				continue
			}

			// Extract module name (first part before version)
			parts := strings.Fields(depLine)
			if len(parts) >= 1 {
				modPath := parts[0]
				// Get short name (last part of module path)
				pathParts := strings.Split(modPath, "/")
				shortName := pathParts[len(pathParts)-1]
				if shortName != "" && shortName != "(" {
					deps = append(deps, shortName)
				}
			}

			if len(deps) >= 10 {
				break
			}
		}
	}
	return deps
}

// getPythonDependencies collects dependencies from requirements.txt or pyproject.toml
func getPythonDependencies(repoPath string) []string {
	var deps []string

	// Try requirements.txt first
	reqPath := filepath.Join(repoPath, "requirements.txt")
	if data, err := os.ReadFile(reqPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			// Skip comments and empty lines
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
				continue
			}
			// Extract package name (before ==, >=, <=, ~=, etc.)
			re := regexp.MustCompile(`^([a-zA-Z0-9_-]+)`)
			if match := re.FindStringSubmatch(line); len(match) > 1 {
				deps = append(deps, match[1])
			}
			if len(deps) >= 10 {
				break
			}
		}
		return deps
	}

	// Fall back to pyproject.toml
	pyprojectPath := filepath.Join(repoPath, "pyproject.toml")
	if data, err := os.ReadFile(pyprojectPath); err == nil {
		// Simple regex to find dependencies
		re := regexp.MustCompile(`(?m)^\s*"?([a-zA-Z0-9_-]+)"?\s*[=><~]`)
		content := string(data)
		matches := re.FindAllStringSubmatch(content, 10)
		for _, match := range matches {
			if len(match) > 1 {
				deps = append(deps, match[1])
			}
		}
	}

	return deps
}

// OutdatedResult represents npm/yarn outdated output
type OutdatedResult struct {
	Current string `json:"current"`
	Wanted  string `json:"wanted"`
	Latest  string `json:"latest"`
}

// getOutdatedDependencyCount checks for outdated dependencies
func getOutdatedDependencyCount(repoPath string, projectType string) int {
	switch projectType {
	case "npm":
		return getNpmOutdatedCount(repoPath)
	case "yarn":
		return getYarnOutdatedCount(repoPath)
	case "pnpm":
		return getPnpmOutdatedCount(repoPath)
	case "maven":
		// Maven outdated checking is more complex and slow, skip for now
		return 0
	}
	return 0
}

// getNpmOutdatedCount runs npm outdated --json and counts outdated packages
func getNpmOutdatedCount(repoPath string) int {
	cmd := exec.Command("npm", "outdated", "--json")
	cmd.Dir = repoPath
	output, _ := cmd.Output() // npm outdated returns exit code 1 if there are outdated packages

	if len(output) == 0 {
		return 0
	}

	var outdated map[string]OutdatedResult
	if err := json.Unmarshal(output, &outdated); err != nil {
		return 0
	}

	return len(outdated)
}

// getYarnOutdatedCount runs yarn outdated --json and counts outdated packages
func getYarnOutdatedCount(repoPath string) int {
	// Check if it's Yarn Berry (2+) or Classic (1.x)
	yarnrcPath := filepath.Join(repoPath, ".yarnrc.yml")
	isYarnBerry := false
	if _, err := os.Stat(yarnrcPath); err == nil {
		isYarnBerry = true
	}

	if isYarnBerry {
		// Yarn Berry uses different output format
		cmd := exec.Command("yarn", "npm", "audit", "--json")
		cmd.Dir = repoPath
		// Yarn Berry outdated is complex, return 0 for now
		return 0
	}

	// Yarn Classic
	cmd := exec.Command("yarn", "outdated", "--json")
	cmd.Dir = repoPath
	output, _ := cmd.Output()

	if len(output) == 0 {
		return 0
	}

	// Yarn Classic outputs NDJSON - each line is a separate JSON object
	count := 0
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			continue
		}
		if result["type"] == "table" {
			if data, ok := result["data"].(map[string]interface{}); ok {
				if body, ok := data["body"].([]interface{}); ok {
					count = len(body)
				}
			}
		}
	}

	return count
}

// getPnpmOutdatedCount runs pnpm outdated --json and counts outdated packages
func getPnpmOutdatedCount(repoPath string) int {
	cmd := exec.Command("pnpm", "outdated", "--json")
	cmd.Dir = repoPath
	output, _ := cmd.Output()

	if len(output) == 0 {
		return 0
	}

	var outdated map[string]interface{}
	if err := json.Unmarshal(output, &outdated); err != nil {
		// Try parsing as array
		var outdatedArr []interface{}
		if err := json.Unmarshal(output, &outdatedArr); err != nil {
			return 0
		}
		return len(outdatedArr)
	}

	return len(outdated)
}

// isPythonProject checks if the directory is a Python project
func isPythonProject(repoPath string) bool {
	// Check for common Python project files
	pythonFiles := []string{
		"requirements.txt",
		"setup.py",
		"pyproject.toml",
		"Pipfile",
		"setup.cfg",
		"poetry.lock",
	}

	for _, f := range pythonFiles {
		if _, err := os.Stat(filepath.Join(repoPath, f)); err == nil {
			return true
		}
	}

	// Check for .py files in root directory
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

// detectPythonFramework detects the Python framework used
func detectPythonFramework(repoPath string) string {
	// Read requirements.txt if exists
	reqPath := filepath.Join(repoPath, "requirements.txt")
	if content, err := os.ReadFile(reqPath); err == nil {
		reqContent := strings.ToLower(string(content))

		if strings.Contains(reqContent, "django") {
			return "Django"
		}
		if strings.Contains(reqContent, "flask") {
			return "Flask"
		}
		if strings.Contains(reqContent, "fastapi") {
			return "FastAPI"
		}
		if strings.Contains(reqContent, "streamlit") {
			return "Streamlit"
		}
		if strings.Contains(reqContent, "pytorch") || strings.Contains(reqContent, "torch") {
			return "PyTorch"
		}
		if strings.Contains(reqContent, "tensorflow") {
			return "TensorFlow"
		}
		if strings.Contains(reqContent, "pandas") || strings.Contains(reqContent, "numpy") {
			return "Data Science"
		}
	}

	// Read pyproject.toml if exists
	pyprojectPath := filepath.Join(repoPath, "pyproject.toml")
	if content, err := os.ReadFile(pyprojectPath); err == nil {
		pyContent := strings.ToLower(string(content))

		if strings.Contains(pyContent, "django") {
			return "Django"
		}
		if strings.Contains(pyContent, "flask") {
			return "Flask"
		}
		if strings.Contains(pyContent, "fastapi") {
			return "FastAPI"
		}
	}

	return "Python"
}

// detectGoFramework detects the Go framework used
func detectGoFramework(repoPath string) string {
	// Read go.mod to check dependencies
	goModPath := filepath.Join(repoPath, "go.mod")
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return "Go"
	}

	modContent := string(content)

	// Check for popular Go frameworks/libraries
	if strings.Contains(modContent, "github.com/gin-gonic/gin") {
		return "Gin"
	}
	if strings.Contains(modContent, "github.com/gofiber/fiber") {
		return "Fiber"
	}
	if strings.Contains(modContent, "github.com/labstack/echo") {
		return "Echo"
	}
	if strings.Contains(modContent, "github.com/gorilla/mux") {
		return "Gorilla Mux"
	}
	if strings.Contains(modContent, "github.com/beego/beego") {
		return "Beego"
	}
	if strings.Contains(modContent, "github.com/go-chi/chi") {
		return "Chi"
	}
	if strings.Contains(modContent, "github.com/revel/revel") {
		return "Revel"
	}
	if strings.Contains(modContent, "google.golang.org/grpc") {
		return "gRPC"
	}

	return "Go"
}

// getGoVersion reads the Go version from go.mod
func getGoVersion(repoPath string) string {
	goModPath := filepath.Join(repoPath, "go.mod")
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return ""
	}

	// Parse go version from go.mod (e.g., "go 1.21")
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "go ") {
			return strings.TrimPrefix(line, "go ")
		}
	}

	return ""
}

// getPythonVersion tries to detect Python version from project files
func getPythonVersion(repoPath string) string {
	// Check .python-version (pyenv)
	pvPath := filepath.Join(repoPath, ".python-version")
	if content, err := os.ReadFile(pvPath); err == nil {
		return strings.TrimSpace(string(content))
	}

	// Check pyproject.toml for python version
	pyprojectPath := filepath.Join(repoPath, "pyproject.toml")
	if content, err := os.ReadFile(pyprojectPath); err == nil {
		// Look for python = "^3.11" or requires-python = ">=3.8"
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if strings.Contains(line, "python") && strings.Contains(line, "=") {
				// Extract version (simple extraction)
				re := regexp.MustCompile(`[0-9]+\.[0-9]+`)
				if match := re.FindString(line); match != "" {
					return match
				}
			}
		}
	}

	// Check runtime.txt (Heroku style)
	runtimePath := filepath.Join(repoPath, "runtime.txt")
	if content, err := os.ReadFile(runtimePath); err == nil {
		line := strings.TrimSpace(string(content))
		if strings.HasPrefix(line, "python-") {
			return strings.TrimPrefix(line, "python-")
		}
	}

	return ""
}

// isPhpProject checks if the directory is a PHP project
func isPhpProject(repoPath string) bool {
	// Check for composer.json (Composer package manager)
	if _, err := os.Stat(filepath.Join(repoPath, "composer.json")); err == nil {
		return true
	}
	return false
}

// ComposerJSON represents the structure of composer.json
type ComposerJSON struct {
	Require    map[string]string `json:"require"`
	RequireDev map[string]string `json:"require-dev"`
	Config     struct {
		Platform struct {
			PHP string `json:"php"`
		} `json:"platform"`
	} `json:"config"`
}

// detectPhpFramework detects the PHP framework used
func detectPhpFramework(repoPath string) string {
	composerPath := filepath.Join(repoPath, "composer.json")
	file, err := os.Open(composerPath)
	if err != nil {
		return "PHP"
	}
	defer file.Close()

	var composer ComposerJSON
	if err := json.NewDecoder(file).Decode(&composer); err != nil {
		return "PHP"
	}

	// Merge dependencies for checking
	allDeps := make(map[string]bool)
	for dep := range composer.Require {
		allDeps[dep] = true
	}
	for dep := range composer.RequireDev {
		allDeps[dep] = true
	}

	// Framework detection (more specific first)
	if allDeps["laravel/framework"] {
		return "Laravel"
	}
	if allDeps["symfony/framework-bundle"] || allDeps["symfony/symfony"] {
		return "Symfony"
	}
	if allDeps["yiisoft/yii2"] {
		return "Yii2"
	}
	if allDeps["cakephp/cakephp"] {
		return "CakePHP"
	}
	if allDeps["codeigniter4/framework"] {
		return "CodeIgniter"
	}
	if allDeps["slim/slim"] {
		return "Slim"
	}
	if allDeps["laminas/laminas-mvc"] || allDeps["zendframework/zend-mvc"] {
		return "Laminas/Zend"
	}
	if allDeps["drupal/core"] {
		return "Drupal"
	}
	if allDeps["wordpress/core-dev"] || strings.Contains(composerPath, "wordpress") {
		return "WordPress"
	}
	if allDeps["magento/product-community-edition"] || allDeps["magento/magento2-base"] {
		return "Magento"
	}

	return "PHP"
}

// getPhpVersion reads the PHP version from composer.json
func getPhpVersion(repoPath string) string {
	composerPath := filepath.Join(repoPath, "composer.json")
	file, err := os.Open(composerPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	var composer ComposerJSON
	if err := json.NewDecoder(file).Decode(&composer); err != nil {
		return ""
	}

	// Check config.platform.php first
	if composer.Config.Platform.PHP != "" {
		return composer.Config.Platform.PHP
	}

	// Check require.php
	if phpVer, ok := composer.Require["php"]; ok {
		// Clean up version constraint (e.g., "^8.1" -> "8.1", ">=7.4" -> "7.4")
		re := regexp.MustCompile(`[0-9]+\.[0-9]+(\.[0-9]+)?`)
		if match := re.FindString(phpVer); match != "" {
			return match
		}
		return phpVer
	}

	return ""
}

// getPhpDependencies collects dependencies from composer.json
func getPhpDependencies(repoPath string) []string {
	composerPath := filepath.Join(repoPath, "composer.json")
	file, err := os.Open(composerPath)
	if err != nil {
		return nil
	}
	defer file.Close()

	var composer ComposerJSON
	if err := json.NewDecoder(file).Decode(&composer); err != nil {
		return nil
	}

	var deps []string
	count := 0
	for dep := range composer.Require {
		// Skip PHP itself and common extensions
		if dep == "php" || strings.HasPrefix(dep, "ext-") {
			continue
		}
		// Skip framework dependencies that are already tracked
		if dep == "laravel/framework" || dep == "symfony/framework-bundle" {
			continue
		}
		// Get short name (after vendor/)
		parts := strings.Split(dep, "/")
		if len(parts) == 2 {
			deps = append(deps, parts[1])
		} else {
			deps = append(deps, dep)
		}
		count++
		if count >= 10 {
			break
		}
	}
	return deps
}

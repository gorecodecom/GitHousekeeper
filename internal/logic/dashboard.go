package logic

import (
	"bufio"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// DashboardStats holds the aggregated data for the dashboard
type DashboardStats struct {
	TotalRepos       int             `json:"totalRepos"`
	AvgHealthScore   int             `json:"avgHealthScore"`
	TotalTodos       int             `json:"totalTodos"`
	TopDependencies  []NameCount     `json:"topDependencies"`
	RepoDetails      []RepoHealth    `json:"repoDetails"`
	SpringVersions   map[string]int  `json:"springVersions"` // e.g. "3.2.0" -> 5
}

type NameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type RepoHealth struct {
	Name            string `json:"name"`
	Path            string `json:"path"`
	HealthScore     int    `json:"healthScore"`
	TodoCount       int    `json:"todoCount"`
	SpringBootVer   string `json:"springBootVer"`
	JavaVersion     string `json:"javaVersion"`
	LastCommit      string `json:"lastCommit"`
	HasBuildErrors  bool   `json:"hasBuildErrors"`
}

// CollectDashboardStats scans the given root path for repositories and gathers statistics.
func CollectDashboardStats(rootPath string, excluded []string) DashboardStats {
	repos := FindGitRepos(rootPath, excluded)
	
	stats := DashboardStats{
		TotalRepos:      len(repos),
		SpringVersions:  make(map[string]int),
		TopDependencies: []NameCount{},
		RepoDetails:     []RepoHealth{},
	}

	if len(repos) == 0 {
		return stats
	}

	var wg sync.WaitGroup
	resultChan := make(chan RepoHealth, len(repos))
	depChan := make(chan []string, len(repos))

	for _, repo := range repos {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			health, deps := analyzeRepoHealth(path)
			resultChan <- health
			depChan <- deps
		}(repo)
	}

	go func() {
		wg.Wait()
		close(resultChan)
		close(depChan)
	}()

	totalScore := 0
	depCounts := make(map[string]int)

	for health := range resultChan {
		stats.RepoDetails = append(stats.RepoDetails, health)
		stats.TotalTodos += health.TodoCount
		totalScore += health.HealthScore
		if health.SpringBootVer != "" {
			stats.SpringVersions[health.SpringBootVer]++
		}
	}

	for deps := range depChan {
		for _, d := range deps {
			depCounts[d]++
		}
	}

	if stats.TotalRepos > 0 {
		stats.AvgHealthScore = totalScore / stats.TotalRepos
	}

	for name, count := range depCounts {
		stats.TopDependencies = append(stats.TopDependencies, NameCount{Name: name, Count: count})
	}

	return stats
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
				fullDep := dep.GroupId + ":" + dep.ArtifactId
				dependencies = append(dependencies, fullDep)
				
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
	var cmd *exec.Cmd
	if strings.Contains(strings.ToLower(os.Getenv("OS")), "windows") {
		cmd = exec.Command("cmd", "/C", "mvn", "help:effective-pom", "-N")
	} else {
		cmd = exec.Command("mvn", "help:effective-pom", "-N")
	}
	cmd.Dir = dir

	// Capture output
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
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

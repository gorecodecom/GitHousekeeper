package logic

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Replacement struct {
	Search  string
	Replace string
}

type ReportEntry struct {
	RepoPath          string
	Messages          []string
	Success           bool
	DeprecationOutput string
}

type RepoOptions struct {
	PomReplacements     []Replacement
	ProjectReplacements []Replacement
	TargetParentVersion string
	VersionBumpStrategy string
	RunCleanInstall     bool
	ExcludedFolders     []string
	TargetBranch        string // "housekeeping", "custom-name", or "" (for master)
	Log                 func(string)
}

func IsGitRepo(path string) bool {
	gitPath := filepath.Join(path, ".git")
	info, err := os.Stat(gitPath)
	return err == nil && info.IsDir()
}

func FindGitRepos(root string, excluded []string) []string {
	var repos []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			for _, ex := range excluded {
				if info.Name() == ex {
					return filepath.SkipDir
				}
			}

			if info.Name() == ".git" {
				repoPath := filepath.Dir(path)
				repos = append(repos, repoPath)
				return filepath.SkipDir
			}
		}
		return nil
	})

	if err != nil {
		fmt.Printf("Error searching: %v\n", err)
	}
	return repos
}

func ProcessRepo(path string, opts RepoOptions) ReportEntry {
	entry := ReportEntry{RepoPath: path, Success: true}
	// Use provided logger or fallback to stdout
	log := opts.Log
	if log == nil {
		log = func(msg string) {
			fmt.Println(msg)
		}
	}

	// Internal helper to capture messages for the report entry AND stream them
	captureLog := func(msg string) {
		entry.Messages = append(entry.Messages, msg)
		log(msg)
	}

	captureLog(fmt.Sprintf("Processing: %s", path))

	// 1. Always update master first
	captureLog("  Switching to master and updating...")
	err := runGitCommand(path, "checkout", "master")
	if err != nil {
		captureLog(fmt.Sprintf("  [ERROR] Checkout master failed: %v", err))
		entry.Success = false
		return entry
	}

	err = runGitCommand(path, "fetch", "-p")
	if err != nil {
		captureLog(fmt.Sprintf("  [WARNING] Fetch -p failed: %v", err))
	}

	err = runGitCommand(path, "pull")
	if err != nil {
		captureLog(fmt.Sprintf("  [ERROR] Pull master failed: %v", err))
		entry.Success = false
		return entry
	}
	captureLog("  Master successfully updated.")

	// 2. Branch Logic
	targetBranch := strings.TrimSpace(opts.TargetBranch)

	if targetBranch == "" {
		captureLog("  No target branch specified. Continuing on master.")
	} else {
		// Special logic for "housekeeping"
		if targetBranch == "housekeeping" {
			checkAndDeleteOldHousekeeping(path, captureLog)
		}

		if branchExists(path, targetBranch) {
			captureLog(fmt.Sprintf("  Switching to existing branch '%s'...", targetBranch))
			err := runGitCommand(path, "checkout", targetBranch)
			if err != nil {
				captureLog(fmt.Sprintf("  [ERROR] Checkout %s failed: %v", targetBranch, err))
				entry.Success = false
				return entry
			}

			// For custom branches (not housekeeping), try to pull updates if tracking remote
			if targetBranch != "housekeeping" {
				err := runGitCommand(path, "pull")
				if err == nil {
					captureLog("  Branch updated (Pull).")
				} else {
					// It's okay if pull fails (e.g. local only branch), just log info
					captureLog("  Pull not possible (maybe local only), continuing.")
				}
			}
		} else {
			captureLog(fmt.Sprintf("  Creating new branch '%s' from master...", targetBranch))
			err := runGitCommand(path, "checkout", "-b", targetBranch)
			if err != nil {
				captureLog(fmt.Sprintf("  [ERROR] Could not create branch '%s': %v", targetBranch, err))
				entry.Success = false
				return entry
			}
			captureLog(fmt.Sprintf("  Branch '%s' created.", targetBranch))
		}
	}

	tag := getLatestTag(path)
	captureLog(fmt.Sprintf("  Current Tag: %s", tag))

	processPomXml(path, tag, opts.PomReplacements, opts.TargetParentVersion, opts.VersionBumpStrategy, captureLog)
	processCiSettingsXml(path, captureLog)
	projectChangesMade := processProjectReplacements(path, opts.ProjectReplacements, opts.ExcludedFolders, captureLog)

	var buildOutput string

	if projectChangesMade || opts.RunCleanInstall {
		if opts.RunCleanInstall {
			captureLog("  Running Maven Clean Install (explicitly requested)...")
		} else {
			captureLog("  Changes were made. Running Maven Re-import...")
		}

		var cmd *exec.Cmd
		// Add -Dmaven.compiler.showDeprecation=true to capture deprecations in the same run
		if strings.Contains(strings.ToLower(os.Getenv("OS")), "windows") {
			cmd = exec.Command("cmd", "/C", "mvn", "clean", "install", "-DskipTests", "-Dmaven.compiler.showDeprecation=true")
		} else {
			cmd = exec.Command("mvn", "clean", "install", "-DskipTests", "-Dmaven.compiler.showDeprecation=true")
		}
		cmd.Dir = path

		outputBytes, err := cmd.CombinedOutput()
		buildOutput = string(outputBytes)

		if err != nil {
			captureLog(fmt.Sprintf("  [ERROR] Maven Build failed: %v\nOutput:\n%s", err, buildOutput))
			entry.Success = false
		} else {
			captureLog("  Maven Build successful.")
		}
	}

	if buildOutput != "" {
		// Parse deprecations from the build we just ran
		entry.DeprecationOutput = parseDeprecationsFromOutput(buildOutput, captureLog)
	} else {
		// No build ran yet. If we want to check deprecations, we must run a build now.
		// Since the user didn't ask for a build (runCleanInstall=false) and no changes were made,
		// we run 'clean compile' just for deprecations.
		entry.DeprecationOutput = checkDeprecations(path, captureLog)
	}

	return entry
}

func branchExists(path, branchName string) bool {
	err := runGitCommand(path, "show-ref", "--verify", "--quiet", "refs/heads/"+branchName)
	return err == nil
}

func checkAndDeleteOldHousekeeping(path string, log func(string)) {
	if !branchExists(path, "housekeeping") {
		return
	}

	cmd := exec.Command("git", "log", "-1", "--format=%cI", "housekeeping")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		log(fmt.Sprintf("  [WARNING] Could not read date of housekeeping: %v", err))
		return
	}
	dateStr := strings.TrimSpace(string(output))
	branchDate, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		log(fmt.Sprintf("  [WARNING] Could not parse date '%s': %v", dateStr, err))
		return
	}

	now := time.Now()
	currentMonthFirst := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	threshold := currentMonthFirst.AddDate(0, -1, 0)

	if branchDate.Before(threshold) {
		log(fmt.Sprintf("  [INFO] Branch housekeeping is old (%s), deleting it...", branchDate.Format("2006-01-02")))
		err := runGitCommand(path, "branch", "-D", "housekeeping")
		if err != nil {
			log(fmt.Sprintf("  [ERROR] Could not delete housekeeping: %v", err))
		} else {
			log("  Branch housekeeping deleted.")
		}
	}
}

func runGitCommand(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

func getLatestTag(path string) string {
	cmd := exec.Command("git", "tag", "--sort=-v:refname")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return "No Tags"
	}
	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 && lines[0] != "" {
		return strings.TrimSpace(lines[0])
	}
	return "No Tags"
}

func processPomXml(repoPath, tag string, replacements []Replacement, targetParentVersion string, versionBumpStrategy string, log func(string)) {
	pomPath := filepath.Join(repoPath, "pom.xml")
	contentBytes, err := os.ReadFile(pomPath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		log(fmt.Sprintf("  [ERROR] Could not read pom.xml: %v", err))
		return
	}
	content := string(contentBytes)
	originalContent := content

	cleanTag := strings.TrimPrefix(tag, "v")

	if cleanTag != "" && cleanTag != "No Tags" {
		excludePatterns := []string{
			`(?s)<parent>.*?</parent>`,
			`(?s)<dependencies>.*?</dependencies>`,
			`(?s)<dependencyManagement>.*?</dependencyManagement>`,
			`(?s)<build>.*?</build>`,
			`(?s)<profiles>.*?</profiles>`,
		}

		var excludedRanges [][]int
		for _, pat := range excludePatterns {
			re := regexp.MustCompile(pat)
			matches := re.FindAllStringIndex(content, -1)
			excludedRanges = append(excludedRanges, matches...)
		}

		reVersion := regexp.MustCompile(`<version>(.*?)</version>`)
		versionMatches := reVersion.FindAllStringSubmatchIndex(content, -1)

		var projectVersionMatch []int

		for _, match := range versionMatches {
			vStart := match[0]
			vEnd := match[1]

			isExcluded := false
			for _, rng := range excludedRanges {
				if vStart >= rng[0] && vEnd <= rng[1] {
					isExcluded = true
					break
				}
			}

			if !isExcluded {
				projectVersionMatch = match
				break
			}
		}

		if projectVersionMatch != nil {
			currentProjectVersion := content[projectVersionMatch[2]:projectVersionMatch[3]]

			if currentProjectVersion == cleanTag {
				parts := strings.Split(cleanTag, ".")
				var newVersion string

				if len(parts) >= 3 {
					major := parts[0]
					minor := parts[1]
					patch := parts[2]

					var majorInt, minorInt, patchInt int
					fmt.Sscanf(major, "%d", &majorInt)
					fmt.Sscanf(minor, "%d", &minorInt)
					fmt.Sscanf(patch, "%d", &patchInt)

					switch versionBumpStrategy {
					case "major":
						majorInt++
						minorInt = 0
						patchInt = 0
					case "minor":
						minorInt++
						patchInt = 0
					default: // "patch" or empty
						patchInt++
					}

					newVersion = fmt.Sprintf("%d.%d.%d", majorInt, minorInt, patchInt)

				} else if len(parts) == 2 {
					major := parts[0]
					minor := parts[1]

					var majorInt, minorInt int
					fmt.Sscanf(major, "%d", &majorInt)
					fmt.Sscanf(minor, "%d", &minorInt)

					switch versionBumpStrategy {
					case "major":
						majorInt++
						minorInt = 0
					case "minor":
						minorInt++
					default: // "patch"
						// 1.2 -> 1.2.1
						newVersion = fmt.Sprintf("%d.%d.1", majorInt, minorInt)
						// If we treat 2 parts as Major.Minor, patch bump adds a part.
						// If we treat as Major.Minor, minor bump is 1.3
						// Major bump is 2.0
					}

					if newVersion == "" {
						newVersion = fmt.Sprintf("%d.%d", majorInt, minorInt)
					}
				}

				if newVersion != "" {
					absStart := projectVersionMatch[2]
					absEnd := projectVersionMatch[3]

					content = content[:absStart] + newVersion + content[absEnd:]

					log(fmt.Sprintf("  [INFO] Version in pom.xml updated (%s): %s -> %s", versionBumpStrategy, currentProjectVersion, newVersion))
				}
			} else {
				log(fmt.Sprintf("  [INFO] Version in pom.xml (%s) does not match Tag (%s). No update.", currentProjectVersion, cleanTag))
			}
		} else {
			log("  [WARNING] No project version found in pom.xml (maybe only defined in Parent?).")
		}
	}

	for _, r := range replacements {
		if r.Search != "" {
			newContent, changed := performFuzzyReplacement(content, r.Search, r.Replace)
			if changed {
				content = newContent
				log(fmt.Sprintf("  [INFO] Custom replacement performed: '%s' -> '%s'", r.Search, r.Replace))
			} else {
				log(fmt.Sprintf("  [INFO] Search text '%s' not found, no replacement.", r.Search))
			}
		}
	}

	if targetParentVersion != "" {
		re := regexp.MustCompile(`(?s)<parent>.*?</parent>`)
		parentBlock := re.FindString(content)

		if parentBlock != "" {
			reVersion := regexp.MustCompile(`<version>(.*?)</version>`)
			match := reVersion.FindStringSubmatch(parentBlock)
			if len(match) > 1 {
				currentParentVersion := match[1]
				if currentParentVersion != targetParentVersion {
					newParentBlock := strings.Replace(parentBlock, "<version>"+currentParentVersion+"</version>", "<version>"+targetParentVersion+"</version>", 1)
					content = strings.Replace(content, parentBlock, newParentBlock, 1)
					log(fmt.Sprintf("  [INFO] Parent version updated: %s -> %s", currentParentVersion, targetParentVersion))
				} else {
					log("  [INFO] Parent version is already up to date.")
				}
			}
		}
	}

	singleRepo := `  <repositories>
    <repository>
      <id>gitlab-maven</id>
      <url>https://git.weka.de/api/v4/projects/592/packages/maven</url>
    </repository>
  </repositories>`

	doubleRepo := `  <repositories>
    <repository>
      <id>gitlab-maven</id>
      <url>https://git.weka.de/api/v4/projects/592/packages/maven</url>
    </repository>
    <repository>
      <id>gitlab-maven-common</id>
      <url>https://git.weka.de/api/v4/projects/611/packages/maven</url>
    </repository>
  </repositories>`

	if strings.Contains(content, singleRepo) {
		content = strings.Replace(content, singleRepo, doubleRepo, 1)
		log("  [INFO] Repositories block updated.")
	}

	if content != originalContent {
		err = os.WriteFile(pomPath, []byte(content), 0644)
		if err != nil {
			log(fmt.Sprintf("  [ERROR] Could not write pom.xml: %v", err))
			return
		}

		err = runGitCommand(repoPath, "add", "pom.xml")
		if err != nil {
			log(fmt.Sprintf("  [ERROR] git add pom.xml failed: %v", err))
			return
		}

		err = runGitCommand(repoPath, "commit", "-m", "Update pom.xml")
		if err != nil {
			log(fmt.Sprintf("  [ERROR] git commit failed: %v", err))
			return
		}
		log("  pom.xml updated and committed.")
	} else {
		log("  No changes to pom.xml.")
	}
}

func processCiSettingsXml(repoPath string, log func(string)) {
	ciPath := filepath.Join(repoPath, "ci-settings.xml")
	contentBytes, err := os.ReadFile(ciPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log(fmt.Sprintf("  [ERROR] Could not read ci-settings.xml: %v", err))
		}
		return
	}
	content := string(contentBytes)
	originalContent := content

	singleServer := `    <server>
      <id>gitlab-maven</id>
      <configuration>
        <httpHeaders>
          <property>
            <name>Job-Token</name>
            <value>${CI_JOB_TOKEN}</value>
          </property>
        </httpHeaders>
      </configuration>
    </server>
  </servers>`

	doubleServer := `    <server>
      <id>gitlab-maven</id>
      <configuration>
        <httpHeaders>
          <property>
            <name>Job-Token</name>
            <value>${CI_JOB_TOKEN}</value>
          </property>
        </httpHeaders>
      </configuration>
    </server>
    <server>
      <id>gitlab-maven-common</id>
      <configuration>
        <httpHeaders>
          <property>
            <name>Job-Token</name>
            <value>${CI_JOB_TOKEN}</value>
          </property>
        </httpHeaders>
      </configuration>
    </server>
  </servers>`

	if strings.Contains(content, singleServer) {
		content = strings.Replace(content, singleServer, doubleServer, 1)
		log("  [INFO] ci-settings.xml Server Block updated.")
	}

	if content != originalContent {
		err = os.WriteFile(ciPath, []byte(content), 0644)
		if err != nil {
			log(fmt.Sprintf("  [ERROR] Could not write ci-settings.xml: %v", err))
			return
		}

		err = runGitCommand(repoPath, "add", "ci-settings.xml")
		if err != nil {
			log(fmt.Sprintf("  [ERROR] git add ci-settings.xml failed: %v", err))
			return
		}

		err = runGitCommand(repoPath, "commit", "-m", "Update ci-settings.xml")
		if err != nil {
			log(fmt.Sprintf("  [ERROR] git commit failed: %v", err))
			return
		}
		log("  ci-settings.xml updated and committed.")
	} else {
		log("  No changes to ci-settings.xml.")
	}
}

func processProjectReplacements(root string, replacements []Replacement, excludedFolders []string, log func(string)) bool {
	if len(replacements) == 0 {
		return false
	}

	changesMade := false

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			for _, ex := range excludedFolders {
				if info.Name() == ex {
					return filepath.SkipDir
				}
			}
			if info.Name() == ".git" || info.Name() == "target" || info.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		contentBytes, err := os.ReadFile(path)
		if err != nil {
			log(fmt.Sprintf("    [WARNING] Could not read file %s: %v", path, err))
			return nil
		}

		for i := 0; i < len(contentBytes) && i < 1024; i++ {
			if contentBytes[i] == 0 {
				return nil
			}
		}

		content := string(contentBytes)
		fileChanged := false

		for _, r := range replacements {
			newContent, changed := performFuzzyReplacement(content, r.Search, r.Replace)
			if changed {
				content = newContent
				fileChanged = true
			}
		}

		if fileChanged {
			err = os.WriteFile(path, []byte(content), info.Mode())
			if err != nil {
				log(fmt.Sprintf("    [ERROR] Could not write file %s: %v", path, err))
			} else {
				log(fmt.Sprintf("    [INFO] File updated: %s", path))

				err = runGitCommand(root, "add", path)
				if err == nil {
					runGitCommand(root, "commit", "-m", fmt.Sprintf("Update %s via project-wide replacement", filepath.Base(path)))
				}

				changesMade = true
			}
		}

		return nil
	})

	if err != nil {
		log(fmt.Sprintf("  [ERROR] Error searching for replacements: %v", err))
	}

	return changesMade
}

func performFuzzyReplacement(content, search, replace string) (string, bool) {
	if search == "" {
		return content, false
	}

	// Sanitize inputs: replace non-breaking spaces with normal spaces
	search = strings.ReplaceAll(search, "\u00A0", " ")
	replace = strings.ReplaceAll(replace, "\u00A0", " ")

	// Fuzzy match: treat whitespace as flexible
	parts := strings.Fields(search)
	if len(parts) == 0 {
		return content, false
	}

	var escapedParts []string
	for _, p := range parts {
		escapedParts = append(escapedParts, regexp.QuoteMeta(p))
	}

	// Join parts with \s+ to allow any whitespace (spaces, tabs, newlines) between tokens
	pattern := strings.Join(escapedParts, `\s+`)

	re, err := regexp.Compile("(?s)" + pattern)
	if err != nil {
		return content, false
	}

	// We need to replace ALL occurrences, but respecting indentation for each.
	// Since we can't easily do this with ReplaceAllStringFunc (no index provided),
	// we have to iterate manually.

	currentContent := content
	changed := false

	// Loop until no more matches found
	// To avoid infinite loops if replacement contains the search pattern, we need to be careful.
	// However, usually replacement is different. But if it's fuzzy, it might match again.
	// A safer approach is to find all indices first, then replace from back to front.

	matches := re.FindAllStringIndex(currentContent, -1)
	if matches == nil {
		return content, false
	}

	// Iterate backwards to keep indices valid
	for i := len(matches) - 1; i >= 0; i-- {
		match := matches[i]
		startIdx := match[0]
		endIdx := match[1]

		// Determine indentation of the start line
		lineStartIdx := startIdx
		for lineStartIdx > 0 && currentContent[lineStartIdx-1] != '\n' {
			lineStartIdx--
		}
		indent := currentContent[lineStartIdx:startIdx]

		// Only use indent if it is purely whitespace
		if strings.TrimSpace(indent) != "" {
			indent = "" // Match didn't start after whitespace
		}

		// Adjust replacement
		currentReplace := replace
		currentReplace = strings.ReplaceAll(currentReplace, "\r\n", "\n")
		lines := strings.Split(currentReplace, "\n")

		if len(lines) > 1 && indent != "" {
			for j := 1; j < len(lines); j++ {
				lines[j] = indent + lines[j]
			}
			currentReplace = strings.Join(lines, "\n")
		}

		// Perform replacement
		currentContent = currentContent[:startIdx] + currentReplace + currentContent[endIdx:]
		changed = true
	}

	return currentContent, changed
}

func checkDeprecations(path string, log func(string)) string {
	log("  Checking for deprecations (separate run)...")

	var cmd *exec.Cmd
	if strings.Contains(strings.ToLower(os.Getenv("OS")), "windows") {
		cmd = exec.Command("cmd", "/C", "mvn", "clean", "compile", "-Dmaven.compiler.showDeprecation=true")
	} else {
		cmd = exec.Command("mvn", "clean", "compile", "-Dmaven.compiler.showDeprecation=true")
	}
	cmd.Dir = path

	// We ignore error here because we only care about the output logs
	output, _ := cmd.CombinedOutput()
	return parseDeprecationsFromOutput(string(output), log)
}

func parseDeprecationsFromOutput(output string, log func(string)) string {
	lines := strings.Split(output, "\n")
	var warnings []string
	count := 0

	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "deprecation") || strings.Contains(lower, "deprecated") || strings.Contains(lower, "warning") {
			// Clean up line
			line = strings.TrimSpace(line)
			if line != "" {
				warnings = append(warnings, line)
				count++
				if count >= 100 {
					break
				}
			}
		}
	}

	if len(warnings) > 0 {
		log(fmt.Sprintf("  %d deprecation warnings found.", len(warnings)))
		return strings.Join(warnings, "\n")
	}

	log("  No deprecation warnings found.")
	return ""
}

// Spring Boot Logic

type MavenMetadata struct {
	Versioning Versioning `xml:"versioning"`
}

type Versioning struct {
	Latest   string   `xml:"latest"`
	Release  string   `xml:"release"`
	Versions []string `xml:"versions>version"`
}

type SpringVersionInfo struct {
	Major          string
	Versions       []string
	MigrationGuide string
}

func GetSpringVersions() ([]SpringVersionInfo, error) {
	resp, err := http.Get("https://repo1.maven.org/maven2/org/springframework/boot/spring-boot-starter-parent/maven-metadata.xml")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var metadata MavenMetadata
	if err := xml.Unmarshal(body, &metadata); err != nil {
		return nil, err
	}

	// Group by Major version
	grouped := make(map[string][]string)
	for _, v := range metadata.Versioning.Versions {
		// Filter for stable versions (no M1, RC1, SNAPSHOT) if desired
		if strings.Contains(v, "SNAPSHOT") {
			continue
		}
		parts := strings.Split(v, ".")
		if len(parts) > 0 {
			major := parts[0]
			grouped[major] = append(grouped[major], v)
		}
	}

	var result []SpringVersionInfo

	// Define migration guides
	guides := map[string]string{
		"2": "https://github.com/spring-projects/spring-boot/wiki/Spring-Boot-2.0-Migration-Guide",
		"3": "https://github.com/spring-projects/spring-boot/wiki/Spring-Boot-3.0-Migration-Guide",
		"4": "https://github.com/spring-projects/spring-boot/wiki/Spring-Boot-4.0-Migration-Guide",
	}

	for major, versions := range grouped {
		// Reverse sort to show latest first
		// Since they are strings, we need to be careful with "2.10" vs "2.2"
		// But for now, simple string sort might be enough or we implement semantic version sort.
		// Maven metadata usually returns them sorted.
		// Let's just reverse the slice we got.
		for i, j := 0, len(versions)-1; i < j; i, j = i+1, j-1 {
			versions[i], versions[j] = versions[j], versions[i]
		}

		info := SpringVersionInfo{
			Major:          major,
			Versions:       versions,
			MigrationGuide: guides[major],
		}
		result = append(result, info)
	}

	// Sort result by Major version descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Major > result[j].Major
	})

	return result, nil
}

type ProjectSpringStatus struct {
	Path           string
	CurrentVersion string
	RepoName       string
}

type SpringScanResult struct {
	Projects []ProjectSpringStatus
	DebugLog []string
}

func ScanProjectsForSpring(root string, excluded []string) SpringScanResult {
	var result SpringScanResult
	result.Projects = make([]ProjectSpringStatus, 0)
	result.DebugLog = make([]string, 0)

	log := func(msg string) {
		result.DebugLog = append(result.DebugLog, msg)
		fmt.Println("[SPRING SCAN]", msg) // Also print to stdout
	}

	log(fmt.Sprintf("Starting scan in: %s", root))

	// Walk through directory and find ALL pom.xml files (not just in git repos)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log(fmt.Sprintf("Error accessing %s: %v", path, err))
			return err
		}

		if info.IsDir() {
			// Check exclusions
			for _, ex := range excluded {
				if info.Name() == ex {
					log(fmt.Sprintf("Skipping excluded folder: %s", info.Name()))
					return filepath.SkipDir
				}
			}
			// Always skip standard build/git folders
			if info.Name() == ".git" || info.Name() == "target" || info.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		// Found a pom.xml
		if strings.ToLower(info.Name()) == "pom.xml" {
			log(fmt.Sprintf("Checking POM: %s", path))
			contentBytes, err := os.ReadFile(path)
			if err != nil {
				log(fmt.Sprintf("  Could not read file: %v", err))
				return nil
			}
			content := string(contentBytes)

			// Find Spring Boot Parent version
			reParent := regexp.MustCompile(`(?s)<parent>.*?</parent>`)
			parentBlock := reParent.FindString(content)

			if parentBlock == "" {
				log("  No <parent> block found.")
				return nil
			}

			// Check if it's directly spring-boot-starter-parent
			if strings.Contains(parentBlock, "spring-boot-starter-parent") {
				reVersion := regexp.MustCompile(`<version>(.*?)</version>`)
				match := reVersion.FindStringSubmatch(parentBlock)
				if len(match) > 1 {
					v := match[1]
					log(fmt.Sprintf("  Found (direct): %s", v))
					result.Projects = append(result.Projects, ProjectSpringStatus{
						Path:           filepath.Dir(path),
						RepoName:       filepath.Base(filepath.Dir(path)),
						CurrentVersion: v,
					})
				} else {
					log("  spring-boot-starter-parent found, but no version extractable.")
				}
			} else {
				log("  <parent> is not spring-boot-starter-parent. Trying Effective-POM analysis...")
				// Fallback: Run Maven to get effective pom
				v, err := getSpringBootVersionFromMaven(filepath.Dir(path))
				if err == nil && v != "" {
					log(fmt.Sprintf("  Found (via Maven): %s", v))
					result.Projects = append(result.Projects, ProjectSpringStatus{
						Path:           filepath.Dir(path),
						RepoName:       filepath.Base(filepath.Dir(path)),
						CurrentVersion: v,
					})
				} else {
					log(fmt.Sprintf("  Maven analysis failed or no version found: %v", err))
				}
			}
		}
		return nil
	})

	if err != nil {
		log(fmt.Sprintf("Error during walk: %v", err))
	}

	log(fmt.Sprintf("Scan finished. Found: %d projects", len(result.Projects)))
	return result
}

func getSpringBootVersionFromMaven(dir string) (string, error) {
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
		return "", err
	}
	output := string(outputBytes)

	// Look for spring-boot dependency version
	// Try to find:  <groupId>org.springframework.boot</groupId>
	//               <artifactId>spring-boot</artifactId>
	//               <version>3.0.0</version>
	re := regexp.MustCompile(`(?s)<groupId>org\.springframework\.boot</groupId>\s*<artifactId>spring-boot(-starter)?</artifactId>\s*<version>(.*?)</version>`)
	match := re.FindStringSubmatch(output)
	if len(match) > 2 {
		return match[2], nil
	}

	// Fallback: Check for spring-boot-dependencies in dependencyManagement
	reDep := regexp.MustCompile(`(?s)<artifactId>spring-boot-dependencies</artifactId>\s*<version>(.*?)</version>`)
	matchDep := reDep.FindStringSubmatch(output)
	if len(matchDep) > 1 {
		return matchDep[1], nil
	}

	return "", fmt.Errorf("no Spring Boot version found in Effective POM")
}

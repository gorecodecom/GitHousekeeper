package logic

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Replacement struct {
	Search  string
	Replace string
}

type ReportEntry struct {
	RepoPath string
	Messages []string
	Success  bool
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
		fmt.Printf("Fehler beim Durchsuchen: %v\n", err)
	}
	return repos
}

func ProcessRepo(path string, pomReplacements []Replacement, projectReplacements []Replacement, targetParentVersion string, versionBumpStrategy string, excludedFolders []string) ReportEntry {
	entry := ReportEntry{RepoPath: path, Success: true}
	log := func(msg string) {
		entry.Messages = append(entry.Messages, msg)
		fmt.Println(msg) // Also print to stdout for CLI
	}

	log(fmt.Sprintf("Bearbeite: %s", path))

	isCurrent := checkAndDeleteOldHousekeeping(path, log)

	if isCurrent {
		log("  Branch 'housekeeping' existiert bereits und ist aktuell.")
		err := runGitCommand(path, "checkout", "housekeeping")
		if err != nil {
			log(fmt.Sprintf("  [FEHLER] Checkout housekeeping fehlgeschlagen: %v", err))
			entry.Success = false
			return entry
		}
		log("  Checkout housekeeping erfolgreich.")

		err = runGitCommand(path, "fetch", "-p")
		if err != nil {
			log(fmt.Sprintf("  [FEHLER] Fetch -p fehlgeschlagen: %v", err))
			entry.Success = false
			return entry
		}
		log("  Fetch -p erfolgreich.")
	} else {
		err := runGitCommand(path, "checkout", "master")
		if err != nil {
			log(fmt.Sprintf("  [FEHLER] Checkout master fehlgeschlagen: %v", err))
			entry.Success = false
			return entry
		}
		log("  Checkout master erfolgreich.")

		err = runGitCommand(path, "fetch", "--tags")
		if err != nil {
			log(fmt.Sprintf("  [FEHLER] Fetch --tags fehlgeschlagen: %v", err))
			entry.Success = false
			return entry
		}
		log("  Fetch --tags erfolgreich.")

		err = runGitCommand(path, "pull")
		if err != nil {
			log(fmt.Sprintf("  [FEHLER] Pull fehlgeschlagen: %v", err))
			entry.Success = false
			return entry
		}
		log("  Pull erfolgreich.")

		err = runGitCommand(path, "checkout", "-b", "housekeeping")
		if err != nil {
			log(fmt.Sprintf("  [INFO] Konnte Branch 'housekeeping' nicht neu anlegen (existiert evtl. schon?): %v", err))
		} else {
			log("  Branch 'housekeeping' angelegt.")
		}
	}

	tag := getLatestTag(path)
	log(fmt.Sprintf("  Aktueller Tag: %s", tag))

	processPomXml(path, tag, pomReplacements, targetParentVersion, versionBumpStrategy, log)
	processCiSettingsXml(path, log)
	projectChangesMade := processProjectReplacements(path, projectReplacements, excludedFolders, log)

	if projectChangesMade {
		log("  Änderungen wurden durchgeführt. Führe Maven Re-import aus...")
		var cmd *exec.Cmd
		if strings.Contains(strings.ToLower(os.Getenv("OS")), "windows") {
			cmd = exec.Command("cmd", "/C", "mvn", "clean", "install", "-DskipTests")
		} else {
			cmd = exec.Command("mvn", "clean", "install", "-DskipTests")
		}
		cmd.Dir = path
		
		output, err := cmd.CombinedOutput()
		if err != nil {
			log(fmt.Sprintf("  [FEHLER] Maven Re-import fehlgeschlagen: %v\nOutput:\n%s", err, string(output)))
			entry.Success = false
		} else {
			log("  Maven Re-import erfolgreich.")
		}
	}

	return entry
}

func checkAndDeleteOldHousekeeping(path string, log func(string)) bool {
	err := runGitCommand(path, "show-ref", "--verify", "--quiet", "refs/heads/housekeeping")
	if err != nil {
		return false
	}

	cmd := exec.Command("git", "log", "-1", "--format=%cI", "housekeeping")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		log(fmt.Sprintf("  [WARNUNG] Konnte Datum von housekeeping nicht lesen: %v", err))
		return false
	}
	dateStr := strings.TrimSpace(string(output))
	branchDate, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		log(fmt.Sprintf("  [WARNUNG] Konnte Datum '%s' nicht parsen: %v", dateStr, err))
		return false
	}

	now := time.Now()
	currentMonthFirst := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	threshold := currentMonthFirst.AddDate(0, -1, 0)

	if branchDate.Before(threshold) {
		log(fmt.Sprintf("  [INFO] Branch housekeeping ist alt (%s), lösche ihn...", branchDate.Format("2006-01-02")))
		err := runGitCommand(path, "branch", "-D", "housekeeping")
		if err != nil {
			log(fmt.Sprintf("  [FEHLER] Konnte housekeeping nicht löschen: %v", err))
		} else {
			log("  Branch housekeeping gelöscht.")
		}
		return false
	}
	return true
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
		return "Keine Tags"
	}
	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 && lines[0] != "" {
		return strings.TrimSpace(lines[0])
	}
	return "Keine Tags"
}

func processPomXml(repoPath, tag string, replacements []Replacement, targetParentVersion string, versionBumpStrategy string, log func(string)) {
	pomPath := filepath.Join(repoPath, "pom.xml")
	contentBytes, err := os.ReadFile(pomPath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		log(fmt.Sprintf("  [FEHLER] Konnte pom.xml nicht lesen: %v", err))
		return
	}
	content := string(contentBytes)
	originalContent := content
	
	cleanTag := strings.TrimPrefix(tag, "v")
	
	if cleanTag != "" && cleanTag != "Keine Tags" {
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
					
					log(fmt.Sprintf("  [INFO] Version in pom.xml aktualisiert (%s): %s -> %s", versionBumpStrategy, currentProjectVersion, newVersion))
				}
			} else {
				log(fmt.Sprintf("  [INFO] Version in pom.xml (%s) stimmt nicht mit Tag (%s) überein. Keine Aktualisierung.", currentProjectVersion, cleanTag))
			}
		} else {
			log("  [WARNUNG] Keine Projekt-Version in pom.xml gefunden (evtl. nur in Parent definiert?).")
		}
	}

	for _, r := range replacements {
		if r.Search != "" {
			newContent, changed := performFuzzyReplacement(content, r.Search, r.Replace)
			if changed {
				content = newContent
				log(fmt.Sprintf("  [INFO] Benutzerdefinierte Ersetzung durchgeführt: '%s' -> '%s'", r.Search, r.Replace))
			} else {
				log(fmt.Sprintf("  [INFO] Suchtext '%s' nicht gefunden, keine Ersetzung.", r.Search))
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
					log(fmt.Sprintf("  [INFO] Parent-Version aktualisiert: %s -> %s", currentParentVersion, targetParentVersion))
				} else {
					log("  [INFO] Parent-Version ist bereits aktuell.")
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
		log("  [INFO] Repositories Block aktualisiert.")
	}

	if content != originalContent {
		err = os.WriteFile(pomPath, []byte(content), 0644)
		if err != nil {
			log(fmt.Sprintf("  [FEHLER] Konnte pom.xml nicht schreiben: %v", err))
			return
		}
		
		err = runGitCommand(repoPath, "add", "pom.xml")
		if err != nil {
			log(fmt.Sprintf("  [FEHLER] git add pom.xml fehlgeschlagen: %v", err))
			return
		}
		
		err = runGitCommand(repoPath, "commit", "-m", "Update pom.xml")
		if err != nil {
			log(fmt.Sprintf("  [FEHLER] git commit fehlgeschlagen: %v", err))
			return
		}
		log("  pom.xml aktualisiert und committet.")
	} else {
		log("  Keine Änderungen an pom.xml.")
	}
}

func processCiSettingsXml(repoPath string, log func(string)) {
	ciPath := filepath.Join(repoPath, "ci-settings.xml")
	contentBytes, err := os.ReadFile(ciPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log(fmt.Sprintf("  [FEHLER] Konnte ci-settings.xml nicht lesen: %v", err))
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
		log("  [INFO] ci-settings.xml Server Block aktualisiert.")
	}

	if content != originalContent {
		err = os.WriteFile(ciPath, []byte(content), 0644)
		if err != nil {
			log(fmt.Sprintf("  [FEHLER] Konnte ci-settings.xml nicht schreiben: %v", err))
			return
		}

		err = runGitCommand(repoPath, "add", "ci-settings.xml")
		if err != nil {
			log(fmt.Sprintf("  [FEHLER] git add ci-settings.xml fehlgeschlagen: %v", err))
			return
		}

		err = runGitCommand(repoPath, "commit", "-m", "Update ci-settings.xml")
		if err != nil {
			log(fmt.Sprintf("  [FEHLER] git commit fehlgeschlagen: %v", err))
			return
		}
		log("  ci-settings.xml aktualisiert und committet.")
	} else {
		log("  Keine Änderungen an ci-settings.xml.")
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
			log(fmt.Sprintf("    [WARNUNG] Konnte Datei %s nicht lesen: %v", path, err))
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
				log(fmt.Sprintf("    [FEHLER] Konnte Datei %s nicht schreiben: %v", path, err))
			} else {
				log(fmt.Sprintf("    [INFO] Datei aktualisiert: %s", path))
				
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
		log(fmt.Sprintf("  [FEHLER] Fehler beim Durchsuchen für Ersetzungen: %v", err))
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

	// Try exact match first
	if strings.Contains(content, search) {
		return strings.ReplaceAll(content, search, replace), true
	}

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

	if re.MatchString(content) {
		return re.ReplaceAllString(content, replace), true
	}

	return content, false
}

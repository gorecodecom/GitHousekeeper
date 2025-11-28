package main

import (
	"bufio"
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

func main() {
	reader := bufio.NewReader(os.Stdin)

	// 1. Ask for path
	fmt.Print("Bitte gib den Pfad ein: ")
	pathInput, _ := reader.ReadString('\n')
	pathInput = strings.TrimSpace(pathInput)

	// Validate path
	info, err := os.Stat(pathInput)
	if err != nil {
		fmt.Printf("Fehler beim Zugriff auf Pfad: %v\n", err)
		return
	}
	if !info.IsDir() {
		fmt.Println("Der angegebene Pfad ist kein Verzeichnis.")
		return
	}

	// 2. Ask for excluded folders
	fmt.Print("Welche Ordner sollen ausgeschlossen werden? (Komma-getrennt, leer lassen für keine): ")
	excludeInput, _ := reader.ReadString('\n')
	excludeInput = strings.TrimSpace(excludeInput)
	var excludedFolders []string
	if excludeInput != "" {
		parts := strings.Split(excludeInput, ",")
		for _, p := range parts {
			excludedFolders = append(excludedFolders, strings.TrimSpace(p))
		}
	}

	// 3. Ask for custom POM replacements
	var replacements []Replacement
	
	fmt.Print("Soll eine benutzerdefinierte Ersetzung in der pom.xml durchgeführt werden? (j/n): ")
	customPomInput, _ := reader.ReadString('\n')
	customPomInput = strings.TrimSpace(strings.ToLower(customPomInput))

	if customPomInput == "j" || customPomInput == "y" {
		for {
			fmt.Print("Suchtext: ")
			pomSearch, _ := reader.ReadString('\n')
			pomSearch = strings.TrimSpace(pomSearch)
			
			fmt.Print("Ersetzungstext: ")
			pomReplace, _ := reader.ReadString('\n')
			pomReplace = strings.TrimSpace(pomReplace)

			if pomSearch != "" {
				replacements = append(replacements, Replacement{Search: pomSearch, Replace: pomReplace})
			}
			
			fmt.Print("Möchtest du noch eine weitere Ersetzung hinzufügen? (j/n): ")
			moreInput, _ := reader.ReadString('\n')
			moreInput = strings.TrimSpace(strings.ToLower(moreInput))
			if moreInput != "j" && moreInput != "y" {
				break
			}
		}
	}

	// 4. Ask for Parent Version
	fmt.Print("Welche Parent-Version soll genutzt werden? (Leer lassen für keine Änderung): ")
	parentVersionInput, _ := reader.ReadString('\n')
	parentVersionInput = strings.TrimSpace(parentVersionInput)



	// 5. Ask for mode
	fmt.Print("Ist der Pfad selbst ein Git-Projekt (j) oder sollen Unterordner durchsucht werden (n)? [j/n]: ")
	modeInput, _ := reader.ReadString('\n')
	modeInput = strings.TrimSpace(strings.ToLower(modeInput))

	var repos []string

	if modeInput == "j" || modeInput == "y" {
		if isGitRepo(pathInput) {
			repos = append(repos, pathInput)
		} else {
			fmt.Println("Der Pfad ist kein Git-Repository.")
			return
		}
	} else {
		fmt.Println("Suche nach Git-Projekten...")
		repos = findGitRepos(pathInput, excludedFolders)
	}

	if len(repos) == 0 {
		fmt.Println("Keine Git-Projekte gefunden.")
		return
	}

	fmt.Printf("%d Git-Projekte gefunden. Starte Bearbeitung...\n", len(repos))

	// Process repos
	for _, repo := range repos {
		fmt.Printf("\nBearbeite: %s\n", repo)
		processRepo(repo, replacements, parentVersionInput)
	}
	
	fmt.Println("\nFertig.")
}

func isGitRepo(path string) bool {
	gitPath := filepath.Join(path, ".git")
	info, err := os.Stat(gitPath)
	return err == nil && info.IsDir()
}

func findGitRepos(root string, excluded []string) []string {
	var repos []string
	// Walk handles recursive search. 
	
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if info.IsDir() {
			// Check if this directory is in the excluded list
			for _, ex := range excluded {
				if info.Name() == ex {
					return filepath.SkipDir
				}
			}
			
			if info.Name() == ".git" {
				repoPath := filepath.Dir(path)
				repos = append(repos, repoPath)
				return filepath.SkipDir // Don't search inside .git
			}
		}
		return nil
	})
	
	if err != nil {
		fmt.Printf("Fehler beim Durchsuchen: %v\n", err)
	}
	return repos
}

func processRepo(path string, replacements []Replacement, targetParentVersion string) {
	// Check and delete old housekeeping branch if needed
	// Returns true if branch exists and is current (kept)
	isCurrent := checkAndDeleteOldHousekeeping(path)

	if isCurrent {
		fmt.Println("  Branch 'housekeeping' existiert bereits und ist aktuell.")
		err := runGitCommand(path, "checkout", "housekeeping")
		if err != nil {
			fmt.Printf("  [FEHLER] Checkout housekeeping fehlgeschlagen: %v\n", err)
			return
		}
		fmt.Println("  Checkout housekeeping erfolgreich.")

		err = runGitCommand(path, "fetch", "-p")
		if err != nil {
			fmt.Printf("  [FEHLER] Fetch -p fehlgeschlagen: %v\n", err)
			return
		}
		fmt.Println("  Fetch -p erfolgreich.")
		
		// Continue to POM processing
	} else {
		// 3. Checkout master
		err := runGitCommand(path, "checkout", "master")
		if err != nil {
			fmt.Printf("  [FEHLER] Checkout master fehlgeschlagen: %v\n", err)
			return
		}
		fmt.Println("  Checkout master erfolgreich.")

		// 4. Git fetch -p and pull
	err = runGitCommand(path, "fetch", "--tags")
	if err != nil {
		fmt.Printf("  [FEHLER] Fetch --tags fehlgeschlagen: %v\n", err)
		return
	}
	fmt.Println("  Fetch --tags erfolgreich.")

	err = runGitCommand(path, "pull")
	if err != nil {
		fmt.Printf("  [FEHLER] Pull fehlgeschlagen: %v\n", err)
		return
	}
	fmt.Println("  Pull erfolgreich.")

	// 5. Create branch housekeeping
	err = runGitCommand(path, "checkout", "-b", "housekeeping")
	if err != nil {
		// Maybe it already exists?
		fmt.Printf("  [INFO] Konnte Branch 'housekeeping' nicht neu anlegen (existiert evtl. schon?): %v\n", err)
	} else {
		fmt.Println("  Branch 'housekeeping' angelegt.")
	}
	}

	// 6. Display latest tag
	tag := getLatestTag(path)
	fmt.Printf("  Aktueller Tag: %s\n", tag)

	// 7. Process POM
	processPomXml(path, tag, replacements, targetParentVersion)

	// 8. Process CI Settings
	processCiSettingsXml(path)
}





func processPomXml(repoPath, tag string, replacements []Replacement, targetParentVersion string) {
	pomPath := filepath.Join(repoPath, "pom.xml")
	contentBytes, err := os.ReadFile(pomPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No pom.xml, skip
			return
		}
		fmt.Printf("  [FEHLER] Konnte pom.xml nicht lesen: %v\n", err)
		return
	}
	content := string(contentBytes)
	originalContent := content
	
	// 1. Version Update
	cleanTag := strings.TrimPrefix(tag, "v")
	
	if cleanTag != "" && cleanTag != "Keine Tags" {
		// Find project version by excluding known blocks
		
		// Define blocks to exclude
		excludePatterns := []string{
			`(?s)<parent>.*?</parent>`,
			`(?s)<dependencies>.*?</dependencies>`,
			`(?s)<dependencyManagement>.*?</dependencyManagement>`,
			`(?s)<build>.*?</build>`,
			`(?s)<profiles>.*?</profiles>`,
		}
		
		// Find all excluded ranges
		var excludedRanges [][]int
		for _, pat := range excludePatterns {
			re := regexp.MustCompile(pat)
			matches := re.FindAllStringIndex(content, -1)
			excludedRanges = append(excludedRanges, matches...)
		}
		
		// Find all version tags
		reVersion := regexp.MustCompile(`<version>(.*?)</version>`)
		versionMatches := reVersion.FindAllStringSubmatchIndex(content, -1)
		
		var projectVersionMatch []int
		
		for _, match := range versionMatches {
			// match[0], match[1] are start/end of the whole tag
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
				break // Found the first non-excluded version
			}
		}
		
		if projectVersionMatch != nil {
			// projectVersionMatch[2], projectVersionMatch[3] are start/end of the version string
			currentProjectVersion := content[projectVersionMatch[2]:projectVersionMatch[3]]
			
			// Check if equal to tag
			if currentProjectVersion == cleanTag {
				// Calculate new version
				parts := strings.Split(cleanTag, ".")
				if len(parts) >= 3 {
					major := parts[0]
					minor := parts[1]
					patch := parts[2]
					
					var patchInt int
					fmt.Sscanf(patch, "%d", &patchInt)
					patchInt++
					
					newVersion := fmt.Sprintf("%s.%s.%d", major, minor, patchInt)
					
					// Replace ONLY this occurrence
					absStart := projectVersionMatch[2]
					absEnd := projectVersionMatch[3]
					
					content = content[:absStart] + newVersion + content[absEnd:]
					
					fmt.Printf("  [INFO] Version in pom.xml aktualisiert: %s -> %s\n", currentProjectVersion, newVersion)
				} else if len(parts) == 2 {
					// Handle case like 3.6 -> 3.6.1
					major := parts[0]
					minor := parts[1]
					newVersion := fmt.Sprintf("%s.%s.1", major, minor)
					
					// Replace ONLY this occurrence
					absStart := projectVersionMatch[2]
					absEnd := projectVersionMatch[3]
					
					content = content[:absStart] + newVersion + content[absEnd:]
					
					fmt.Printf("  [INFO] Version in pom.xml aktualisiert: %s -> %s\n", currentProjectVersion, newVersion)
				}
			} else {
				fmt.Printf("  [INFO] Version in pom.xml (%s) stimmt nicht mit Tag (%s) überein. Keine Aktualisierung.\n", currentProjectVersion, cleanTag)
			}
		} else {
			fmt.Println("  [WARNUNG] Keine Projekt-Version in pom.xml gefunden (evtl. nur in Parent definiert?).")
		}
	}

	// 2. Custom Replacements
	for _, r := range replacements {
		if r.Search != "" {
			if strings.Contains(content, r.Search) {
				content = strings.ReplaceAll(content, r.Search, r.Replace)
				fmt.Printf("  [INFO] Benutzerdefinierte Ersetzung durchgeführt: '%s' -> '%s'\n", r.Search, r.Replace)
			} else {
				fmt.Printf("  [INFO] Suchtext '%s' nicht gefunden, keine Ersetzung.\n", r.Search)
			}
		}
	}


	// 3. Parent Version Update
	if targetParentVersion != "" {
		// Regex to find parent block
		// (?s) enables dot matching newlines
		re := regexp.MustCompile(`(?s)<parent>.*?</parent>`)
		parentBlock := re.FindString(content)
		
		if parentBlock != "" {
			// Find version inside parent block
			reVersion := regexp.MustCompile(`<version>(.*?)</version>`)
			match := reVersion.FindStringSubmatch(parentBlock)
			if len(match) > 1 {
				currentParentVersion := match[1]
				if currentParentVersion != targetParentVersion {
					// Replace version in parent block
					newParentBlock := strings.Replace(parentBlock, "<version>"+currentParentVersion+"</version>", "<version>"+targetParentVersion+"</version>", 1)
					// Replace parent block in content
					content = strings.Replace(content, parentBlock, newParentBlock, 1)
					fmt.Printf("  [INFO] Parent-Version aktualisiert: %s -> %s\n", currentParentVersion, targetParentVersion)
				} else {
					fmt.Println("  [INFO] Parent-Version ist bereits aktuell.")
				}
			}
		}
	}

	// 4. Repositories Update
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

	// Normalize line endings just in case, or try exact match
	// Let's try exact match first.
	if strings.Contains(content, singleRepo) {
		content = strings.Replace(content, singleRepo, doubleRepo, 1)
		fmt.Println("  [INFO] Repositories Block aktualisiert.")
	}

	if content != originalContent {
		err = os.WriteFile(pomPath, []byte(content), 0644)
		if err != nil {
			fmt.Printf("  [FEHLER] Konnte pom.xml nicht schreiben: %v\n", err)
			return
		}
		
		// Commit changes
		err = runGitCommand(repoPath, "add", "pom.xml")
		if err != nil {
			fmt.Printf("  [FEHLER] git add pom.xml fehlgeschlagen: %v\n", err)
			return
		}
		
		err = runGitCommand(repoPath, "commit", "-m", "Update pom.xml")
		if err != nil {
			fmt.Printf("  [FEHLER] git commit fehlgeschlagen: %v\n", err)
			return
		}
		fmt.Println("  pom.xml aktualisiert und committet.")
	} else {
		fmt.Println("  Keine Änderungen an pom.xml.")
	}
}


func getLatestTag(path string) string {
	// Use git tag --sort=-v:refname to get the highest version tag
	// This works for tags not reachable from HEAD too, as long as they are fetched.
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

func checkAndDeleteOldHousekeeping(path string) bool {
	// Check if branch exists
	err := runGitCommand(path, "show-ref", "--verify", "--quiet", "refs/heads/housekeeping")
	if err != nil {
		// Branch does not exist
		return false
	}

	// Get branch date
	cmd := exec.Command("git", "log", "-1", "--format=%cI", "housekeeping")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("  [WARNUNG] Konnte Datum von housekeeping nicht lesen: %v\n", err)
		return false // Treat as not current/safe to ignore? Or maybe better to return false so we try to recreate/overwrite?
		// If we return false, main logic tries to checkout master and create housekeeping. 
		// If it exists, create will fail. 
		// But here we are deciding if we should delete it.
		// If we can't read date, let's assume it's not what we want and maybe we should have deleted it?
		// But for safety, let's just return false (not current) and let the create fail if it's there.
	}
	dateStr := strings.TrimSpace(string(output))
	branchDate, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		fmt.Printf("  [WARNUNG] Konnte Datum '%s' nicht parsen: %v\n", dateStr, err)
		return false
	}

	// Calculate threshold: 1st of previous month
	now := time.Now()
	// First day of current month
	currentMonthFirst := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	// First day of previous month
	threshold := currentMonthFirst.AddDate(0, -1, 0)

	if branchDate.Before(threshold) {
		fmt.Printf("  [INFO] Branch housekeeping ist alt (%s), lösche ihn...\n", branchDate.Format("2006-01-02"))
		err := runGitCommand(path, "branch", "-D", "housekeeping")
		if err != nil {
			fmt.Printf("  [FEHLER] Konnte housekeeping nicht löschen: %v\n", err)
		} else {
			fmt.Println("  Branch housekeeping gelöscht.")
		}
		return false
	} else {
		// It is current
		return true
	}
}

func runGitCommand(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	// Capture output to show if needed, or just let it go to stdout/stderr if we want user to see it.
	// For a tool like this, seeing git output is often helpful.
	// But to keep it clean, maybe only on error?
	// Let's capture combined output and return error with it if it fails.
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

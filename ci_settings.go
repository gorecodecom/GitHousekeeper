package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func processCiSettingsXml(repoPath string) {
	ciPath := filepath.Join(repoPath, "ci-settings.xml")
	contentBytes, err := os.ReadFile(ciPath)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Printf("  [FEHLER] Konnte ci-settings.xml nicht lesen: %v\n", err)
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
		fmt.Println("  [INFO] ci-settings.xml Server Block aktualisiert.")
	}

	if content != originalContent {
		err = os.WriteFile(ciPath, []byte(content), 0644)
		if err != nil {
			fmt.Printf("  [FEHLER] Konnte ci-settings.xml nicht schreiben: %v\n", err)
			return
		}

		err = runGitCommand(repoPath, "add", "ci-settings.xml")
		if err != nil {
			fmt.Printf("  [FEHLER] git add ci-settings.xml fehlgeschlagen: %v\n", err)
			return
		}

		err = runGitCommand(repoPath, "commit", "-m", "Update ci-settings.xml")
		if err != nil {
			fmt.Printf("  [FEHLER] git commit fehlgeschlagen: %v\n", err)
			return
		}
		fmt.Println("  ci-settings.xml aktualisiert und committet.")
	} else {
		fmt.Println("  Keine Änderungen an ci-settings.xml.")
	}
}

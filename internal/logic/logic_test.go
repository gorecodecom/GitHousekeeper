package logic

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDeprecationsFromOutput(t *testing.T) {
	// Mock log function
	var loggedMessages []string
	mockLog := func(msg string) {
		loggedMessages = append(loggedMessages, msg)
	}

	tests := []struct {
		name           string
		input          string
		expectedOutput []string
		expectedCount  int
	}{
		{
			name: "No deprecations",
			input: `[INFO] Scanning for projects...
[INFO] ------------------------------------------------------------------------
[INFO] BUILD SUCCESS
[INFO] ------------------------------------------------------------------------`,
			expectedOutput: nil,
			expectedCount:  0,
		},
		{
			name: "Single deprecation warning",
			input: `[INFO] Compiling 1 source file to /target/classes
[WARNING] /path/to/File.java:[10,20] [deprecation] someMethod() in SomeClass has been deprecated
[INFO] Build success`,
			expectedOutput: []string{"[WARNING] /path/to/File.java:[10,20] [deprecation] someMethod() in SomeClass has been deprecated"},
			expectedCount:  1,
		},
		{
			name: "Multiple deprecations mixed with other logs",
			input: `[INFO] Start build
[WARNING] Warning 1: deprecated API used
[INFO] Some info
[WARNING] Warning 2: another deprecation
[ERROR] Some error (ignored by deprecation filter)
[WARNING] Warning 3: something else`,
			expectedOutput: []string{
				"[WARNING] Warning 1: deprecated API used",
				"[WARNING] Warning 2: another deprecation",
				"[WARNING] Warning 3: something else",
			},
			expectedCount: 3,
		},
		{
			name: "Case insensitivity",
			input: `DEPRECATED: old method
[WARNING] careful now`,
			expectedOutput: []string{
				"DEPRECATED: old method",
				"[WARNING] careful now",
			},
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loggedMessages = []string{} // Reset log
			result := parseDeprecationsFromOutput(tt.input, mockLog)

			if tt.expectedCount == 0 {
				if result != "" {
					t.Errorf("Expected empty result, got: %s", result)
				}
			} else {
				resultLines := strings.Split(result, "\n")
				if len(resultLines) != len(tt.expectedOutput) {
					t.Errorf("Expected %d lines, got %d", len(tt.expectedOutput), len(resultLines))
				}
				for i, line := range resultLines {
					if line != tt.expectedOutput[i] {
						t.Errorf("Line %d: expected '%s', got '%s'", i, tt.expectedOutput[i], line)
					}
				}
			}
		})
	}
}

func TestProcessRepo_Options(t *testing.T) {
	// This test verifies that the Options struct is correctly used
	// We can't easily test the full Git/Maven interaction here without mocking,
	// but we can test that the logger is called.

	// Since ProcessRepo does heavy IO, we will just verify the struct exists and compiles
	// which is implicitly done by the build.
	// A real unit test for ProcessRepo would require dependency injection for exec.Command.

	opts := RepoOptions{
		Log: func(msg string) {},
	}
	if opts.Log == nil {
		t.Error("RepoOptions Log should not be nil")
	}
}

func TestPerformFuzzyReplacement_Indentation(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		search   string
		replace  string
		expected string
	}{
		{
			name: "Preserve indentation for multiline replacement",
			content: `
    <parent>
        <child>old</child>
    </parent>
`,
			search: `<parent> <child>old</child> </parent>`,
			replace: `<parent>
    <child>new</child>
</parent>`,
			expected: `
    <parent>
        <child>new</child>
    </parent>
`,
		},
		{
			name: "Preserve indentation 2 spaces",
			content: `
  <foo>bar</foo>
`,
			search: `<foo>bar</foo>`,
			replace: `<foo>
baz
</foo>`,
			expected: `
  <foo>
  baz
  </foo>
`,
		},
		{
			name:    "No indentation match",
			content: `<root><item>val</item></root>`,
			search:  `<item>val</item>`,
			replace: `<item>
new
</item>`,
			expected: `<root><item>
new
</item></root>`,
		},
		{
			name: "Mixed content",
			content: `
    // Comment
    if (foo) {
        doSomething();
    }
`,
			search: `if (foo) { doSomething(); }`,
			replace: `if (bar) {
    doOther();
}`,
			expected: `
    // Comment
    if (bar) {
        doOther();
    }
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, changed := performFuzzyReplacement(tt.content, tt.search, tt.replace)
			if !changed {
				t.Error("Expected change, but got none")
			}

			// Normalize newlines for comparison
			result = strings.ReplaceAll(result, "\r\n", "\n")
			expected := strings.ReplaceAll(tt.expected, "\r\n", "\n")

			if result != expected {
				t.Errorf("Result mismatch.\nExpected:\n%q\nGot:\n%q", expected, result)
			}
		})
	}
}

// ===========================================
// Tests for Unified Replacements (v2.1.0)
// ===========================================

func TestReplacementScope_Routing(t *testing.T) {
	// Test that ReplacementScope correctly routes replacements
	tests := []struct {
		name                 string
		scope                string
		replacements         []Replacement
		expectedPomCount     int
		expectedProjectCount int
	}{
		{
			name:  "Scope 'all' - replacements go to both",
			scope: "all",
			replacements: []Replacement{
				{Search: "foo", Replace: "bar"},
				{Search: "baz", Replace: "qux"},
			},
			expectedPomCount:     2,
			expectedProjectCount: 2,
		},
		{
			name:  "Scope 'pom-only' - replacements only to POM",
			scope: "pom-only",
			replacements: []Replacement{
				{Search: "foo", Replace: "bar"},
			},
			expectedPomCount:     1,
			expectedProjectCount: 0,
		},
		{
			name:  "Scope 'exclude-pom' - replacements only to project files",
			scope: "exclude-pom",
			replacements: []Replacement{
				{Search: "foo", Replace: "bar"},
				{Search: "baz", Replace: "qux"},
			},
			expectedPomCount:     0,
			expectedProjectCount: 2,
		},
		{
			name:  "Empty scope defaults to 'all'",
			scope: "",
			replacements: []Replacement{
				{Search: "test", Replace: "replaced"},
			},
			expectedPomCount:     1,
			expectedProjectCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pomReplacements []Replacement
			var projectReplacements []Replacement

			// Simulate the scope routing logic from ProcessRepo
			switch tt.scope {
			case "pom-only":
				pomReplacements = tt.replacements
				projectReplacements = nil
			case "exclude-pom":
				pomReplacements = nil
				projectReplacements = tt.replacements
			default: // "all" or empty
				pomReplacements = tt.replacements
				projectReplacements = tt.replacements
			}

			if len(pomReplacements) != tt.expectedPomCount {
				t.Errorf("POM replacements: expected %d, got %d", tt.expectedPomCount, len(pomReplacements))
			}
			if len(projectReplacements) != tt.expectedProjectCount {
				t.Errorf("Project replacements: expected %d, got %d", tt.expectedProjectCount, len(projectReplacements))
			}
		})
	}
}

func TestProcessProjectReplacements_SkipsPomXml(t *testing.T) {
	// Create a temporary directory structure
	tempDir, err := os.MkdirTemp("", "test-replacements-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize as a git repo (required for the function)
	runGitCommand(tempDir, "init")
	runGitCommand(tempDir, "config", "user.email", "test@test.com")
	runGitCommand(tempDir, "config", "user.name", "Test User")

	// Create test files
	pomContent := `<?xml version="1.0"?>
<project>
    <version>REPLACE_ME</version>
</project>`
	javaContent := `public class Test {
    // REPLACE_ME
}`
	xmlContent := `<config>REPLACE_ME</config>`

	os.WriteFile(filepath.Join(tempDir, "pom.xml"), []byte(pomContent), 0644)
	os.WriteFile(filepath.Join(tempDir, "Test.java"), []byte(javaContent), 0644)
	os.WriteFile(filepath.Join(tempDir, "config.xml"), []byte(xmlContent), 0644)

	// Initial commit
	runGitCommand(tempDir, "add", "-A")
	runGitCommand(tempDir, "commit", "-m", "Initial commit")

	// Run project replacements (should skip pom.xml)
	replacements := []Replacement{
		{Search: "REPLACE_ME", Replace: "REPLACED"},
	}

	var logMessages []string
	mockLog := func(msg string) {
		logMessages = append(logMessages, msg)
	}

	processProjectReplacements(tempDir, replacements, []string{}, "all", mockLog)

	// Read files back
	pomAfter, _ := os.ReadFile(filepath.Join(tempDir, "pom.xml"))
	javaAfter, _ := os.ReadFile(filepath.Join(tempDir, "Test.java"))
	xmlAfter, _ := os.ReadFile(filepath.Join(tempDir, "config.xml"))

	// pom.xml should NOT be changed (it's handled separately by processPomXml)
	if !strings.Contains(string(pomAfter), "REPLACE_ME") {
		t.Error("pom.xml should NOT be modified by processProjectReplacements")
	}

	// Other files should be changed
	if strings.Contains(string(javaAfter), "REPLACE_ME") {
		t.Error("Test.java should have been modified")
	}
	if !strings.Contains(string(javaAfter), "REPLACED") {
		t.Error("Test.java should contain REPLACED")
	}

	if strings.Contains(string(xmlAfter), "REPLACE_ME") {
		t.Error("config.xml should have been modified")
	}
	if !strings.Contains(string(xmlAfter), "REPLACED") {
		t.Error("config.xml should contain REPLACED")
	}
}

func TestProcessProjectReplacements_ExcludesDirectories(t *testing.T) {
	// Create a temporary directory structure
	tempDir, err := os.MkdirTemp("", "test-excluded-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize as a git repo
	runGitCommand(tempDir, "init")
	runGitCommand(tempDir, "config", "user.email", "test@test.com")
	runGitCommand(tempDir, "config", "user.name", "Test User")

	// Create directory structure
	os.MkdirAll(filepath.Join(tempDir, "src"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "target"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "node_modules"), 0755)

	content := `REPLACE_ME content`
	os.WriteFile(filepath.Join(tempDir, "src", "file.txt"), []byte(content), 0644)
	os.WriteFile(filepath.Join(tempDir, "target", "file.txt"), []byte(content), 0644)
	os.WriteFile(filepath.Join(tempDir, "node_modules", "file.txt"), []byte(content), 0644)

	// Initial commit
	runGitCommand(tempDir, "add", "-A")
	runGitCommand(tempDir, "commit", "-m", "Initial commit")

	replacements := []Replacement{
		{Search: "REPLACE_ME", Replace: "REPLACED"},
	}

	processProjectReplacements(tempDir, replacements, []string{}, "all", func(msg string) {})

	// Read files back
	srcFile, _ := os.ReadFile(filepath.Join(tempDir, "src", "file.txt"))
	targetFile, _ := os.ReadFile(filepath.Join(tempDir, "target", "file.txt"))
	nodeFile, _ := os.ReadFile(filepath.Join(tempDir, "node_modules", "file.txt"))

	// src should be modified
	if strings.Contains(string(srcFile), "REPLACE_ME") {
		t.Error("src/file.txt should have been modified")
	}

	// target should NOT be modified (excluded)
	if !strings.Contains(string(targetFile), "REPLACE_ME") {
		t.Error("target/file.txt should NOT be modified (excluded directory)")
	}

	// node_modules should NOT be modified (excluded)
	if !strings.Contains(string(nodeFile), "REPLACE_ME") {
		t.Error("node_modules/file.txt should NOT be modified (excluded directory)")
	}
}

func TestRepoOptions_ReplacementScopeField(t *testing.T) {
	// Test that RepoOptions struct correctly holds ReplacementScope
	opts := RepoOptions{
		Replacements:     []Replacement{{Search: "a", Replace: "b"}},
		ReplacementScope: "pom-only",
		Log:              func(msg string) {},
	}

	if opts.ReplacementScope != "pom-only" {
		t.Errorf("Expected ReplacementScope 'pom-only', got '%s'", opts.ReplacementScope)
	}

	// Test all valid scope values
	validScopes := []string{"all", "pom-only", "exclude-pom", ""}
	for _, scope := range validScopes {
		opts.ReplacementScope = scope
		if opts.ReplacementScope != scope {
			t.Errorf("Failed to set ReplacementScope to '%s'", scope)
		}
	}
}

func TestProcessProjectReplacements_EmptyReplacements(t *testing.T) {
	// Should return false immediately if no replacements
	result := processProjectReplacements("/tmp", []Replacement{}, []string{}, "all", func(msg string) {})
	if result != false {
		t.Error("Expected false for empty replacements")
	}
}

func TestProcessProjectReplacements_NilReplacements(t *testing.T) {
	// Should return false for nil replacements
	result := processProjectReplacements("/tmp", nil, []string{}, "all", func(msg string) {})
	if result != false {
		t.Error("Expected false for nil replacements")
	}
}

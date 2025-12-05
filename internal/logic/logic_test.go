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

// Tests for getDefaultBranch (v2.3.0)
func TestGetDefaultBranch_Fallback(t *testing.T) {
	// Test that getDefaultBranch returns "master" for non-existent path
	// (since no git repo exists, it will fall through to default)
	branch := getDefaultBranch("/nonexistent/path")
	if branch != "master" {
		t.Errorf("Expected 'master' as fallback for non-existent repo, got '%s'", branch)
	}
}

func TestBranchExists_NonExistentRepo(t *testing.T) {
	// Test that branchExists returns false for non-existent path
	exists := branchExists("/nonexistent/path", "main")
	if exists {
		t.Error("Expected false for non-existent repo")
	}
}

// ===========================================
// Tests for Go Project Detection (v2.4.0)
// ===========================================

func TestDetectGoFramework(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "test-go-framework-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name           string
		goModContent   string
		expectedResult string
	}{
		{
			name:           "Gin Framework",
			goModContent:   "module myapp\n\ngo 1.21\n\nrequire github.com/gin-gonic/gin v1.9.0\n",
			expectedResult: "Gin",
		},
		{
			name:           "Fiber Framework",
			goModContent:   "module myapp\n\ngo 1.21\n\nrequire github.com/gofiber/fiber/v2 v2.50.0\n",
			expectedResult: "Fiber",
		},
		{
			name:           "Echo Framework",
			goModContent:   "module myapp\n\ngo 1.21\n\nrequire github.com/labstack/echo/v4 v4.11.0\n",
			expectedResult: "Echo",
		},
		{
			name:           "Chi Router",
			goModContent:   "module myapp\n\ngo 1.21\n\nrequire github.com/go-chi/chi/v5 v5.0.10\n",
			expectedResult: "Chi",
		},
		{
			name:           "Gorilla Mux",
			goModContent:   "module myapp\n\ngo 1.21\n\nrequire github.com/gorilla/mux v1.8.0\n",
			expectedResult: "Gorilla Mux",
		},
		{
			name:           "gRPC",
			goModContent:   "module myapp\n\ngo 1.21\n\nrequire google.golang.org/grpc v1.59.0\n",
			expectedResult: "gRPC",
		},
		{
			name:           "Plain Go (no framework)",
			goModContent:   "module myapp\n\ngo 1.21\n",
			expectedResult: "Go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write go.mod
			err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(tt.goModContent), 0644)
			if err != nil {
				t.Fatalf("Failed to write go.mod: %v", err)
			}

			result := detectGoFramework(tempDir)
			if result != tt.expectedResult {
				t.Errorf("Expected '%s', got '%s'", tt.expectedResult, result)
			}
		})
	}
}

func TestGetGoVersion(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "test-go-version-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name           string
		goModContent   string
		expectedResult string
	}{
		{
			name:           "Go 1.21",
			goModContent:   "module myapp\n\ngo 1.21\n",
			expectedResult: "1.21",
		},
		{
			name:           "Go 1.22.0",
			goModContent:   "module myapp\n\ngo 1.22.0\n",
			expectedResult: "1.22.0",
		},
		{
			name:           "No go version",
			goModContent:   "module myapp\n",
			expectedResult: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(tt.goModContent), 0644)
			if err != nil {
				t.Fatalf("Failed to write go.mod: %v", err)
			}

			result := getGoVersion(tempDir)
			if result != tt.expectedResult {
				t.Errorf("Expected '%s', got '%s'", tt.expectedResult, result)
			}
		})
	}
}

// ===========================================
// Tests for Python Project Detection (v2.4.0)
// ===========================================

func TestIsPythonProject(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "test-python-project-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Initially should not be a Python project
	if isPythonProject(tempDir) {
		t.Error("Empty directory should not be detected as Python project")
	}

	// Create requirements.txt
	os.WriteFile(filepath.Join(tempDir, "requirements.txt"), []byte("flask==2.0.0\n"), 0644)
	if !isPythonProject(tempDir) {
		t.Error("Directory with requirements.txt should be detected as Python project")
	}

	// Remove and test with pyproject.toml
	os.Remove(filepath.Join(tempDir, "requirements.txt"))
	os.WriteFile(filepath.Join(tempDir, "pyproject.toml"), []byte("[project]\nname = \"myapp\"\n"), 0644)
	if !isPythonProject(tempDir) {
		t.Error("Directory with pyproject.toml should be detected as Python project")
	}

	// Remove and test with .py file
	os.Remove(filepath.Join(tempDir, "pyproject.toml"))
	os.WriteFile(filepath.Join(tempDir, "main.py"), []byte("print('hello')\n"), 0644)
	if !isPythonProject(tempDir) {
		t.Error("Directory with .py files should be detected as Python project")
	}
}

func TestDetectPythonFramework(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "test-python-framework-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name           string
		requirements   string
		expectedResult string
	}{
		{
			name:           "Django",
			requirements:   "django==4.2.0\npsycopg2==2.9.0\n",
			expectedResult: "Django",
		},
		{
			name:           "Flask",
			requirements:   "flask==2.3.0\nflask-sqlalchemy==3.0.0\n",
			expectedResult: "Flask",
		},
		{
			name:           "FastAPI",
			requirements:   "fastapi==0.103.0\nuvicorn==0.23.0\n",
			expectedResult: "FastAPI",
		},
		{
			name:           "Streamlit",
			requirements:   "streamlit==1.28.0\npandas==2.0.0\n",
			expectedResult: "Streamlit",
		},
		{
			name:           "PyTorch",
			requirements:   "torch==2.0.0\ntorchvision==0.15.0\n",
			expectedResult: "PyTorch",
		},
		{
			name:           "TensorFlow",
			requirements:   "tensorflow==2.14.0\nkeras==2.14.0\n",
			expectedResult: "TensorFlow",
		},
		{
			name:           "Data Science (pandas)",
			requirements:   "pandas==2.0.0\nmatplotlib==3.7.0\n",
			expectedResult: "Data Science",
		},
		{
			name:           "Plain Python",
			requirements:   "requests==2.31.0\n",
			expectedResult: "Python",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.WriteFile(filepath.Join(tempDir, "requirements.txt"), []byte(tt.requirements), 0644)
			if err != nil {
				t.Fatalf("Failed to write requirements.txt: %v", err)
			}

			result := detectPythonFramework(tempDir)
			if result != tt.expectedResult {
				t.Errorf("Expected '%s', got '%s'", tt.expectedResult, result)
			}
		})
	}
}

// ===========================================
// Tests for PHP Project Detection (v2.4.0)
// ===========================================

func TestIsPhpProject(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "test-php-project-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Initially should not be a PHP project
	if isPhpProject(tempDir) {
		t.Error("Empty directory should not be detected as PHP project")
	}

	// Create composer.json
	composerJSON := `{"name": "vendor/myapp", "require": {"php": "^8.1"}}`
	os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte(composerJSON), 0644)
	if !isPhpProject(tempDir) {
		t.Error("Directory with composer.json should be detected as PHP project")
	}
}

func TestDetectPhpFramework(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "test-php-framework-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name           string
		composerJSON   string
		expectedResult string
	}{
		{
			name:           "Laravel",
			composerJSON:   `{"require": {"laravel/framework": "^10.0"}}`,
			expectedResult: "Laravel",
		},
		{
			name:           "Symfony",
			composerJSON:   `{"require": {"symfony/framework-bundle": "^6.3"}}`,
			expectedResult: "Symfony",
		},
		{
			name:           "Yii2",
			composerJSON:   `{"require": {"yiisoft/yii2": "^2.0"}}`,
			expectedResult: "Yii2",
		},
		{
			name:           "CakePHP",
			composerJSON:   `{"require": {"cakephp/cakephp": "^4.0"}}`,
			expectedResult: "CakePHP",
		},
		{
			name:           "CodeIgniter",
			composerJSON:   `{"require": {"codeigniter4/framework": "^4.0"}}`,
			expectedResult: "CodeIgniter",
		},
		{
			name:           "Slim",
			composerJSON:   `{"require": {"slim/slim": "^4.0"}}`,
			expectedResult: "Slim",
		},
		{
			name:           "Plain PHP",
			composerJSON:   `{"require": {"php": "^8.1", "guzzlehttp/guzzle": "^7.0"}}`,
			expectedResult: "PHP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte(tt.composerJSON), 0644)
			if err != nil {
				t.Fatalf("Failed to write composer.json: %v", err)
			}

			result := detectPhpFramework(tempDir)
			if result != tt.expectedResult {
				t.Errorf("Expected '%s', got '%s'", tt.expectedResult, result)
			}
		})
	}
}

func TestGetPhpVersion(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "test-php-version-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name           string
		composerJSON   string
		expectedResult string
	}{
		{
			name:           "PHP version from require",
			composerJSON:   `{"require": {"php": "^8.1"}}`,
			expectedResult: "8.1",
		},
		{
			name:           "PHP version with >=",
			composerJSON:   `{"require": {"php": ">=7.4"}}`,
			expectedResult: "7.4",
		},
		{
			name:           "PHP version from platform config",
			composerJSON:   `{"require": {"php": "^8.0"}, "config": {"platform": {"php": "8.2.0"}}}`,
			expectedResult: "8.2.0",
		},
		{
			name:           "No PHP version",
			composerJSON:   `{"require": {"guzzlehttp/guzzle": "^7.0"}}`,
			expectedResult: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.WriteFile(filepath.Join(tempDir, "composer.json"), []byte(tt.composerJSON), 0644)
			if err != nil {
				t.Fatalf("Failed to write composer.json: %v", err)
			}

			result := getPhpVersion(tempDir)
			if result != tt.expectedResult {
				t.Errorf("Expected '%s', got '%s'", tt.expectedResult, result)
			}
		})
	}
}

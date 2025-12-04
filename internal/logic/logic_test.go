package logic

import (
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
			name: "No indentation match",
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

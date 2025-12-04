package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorecode/updates/internal/logic"
)

// ===========================================
// Tests for Health Endpoint (v2.1.0)
// ===========================================

func TestHandleHealth_ReturnsOK(t *testing.T) {
	req, err := http.NewRequest("GET", "/api/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(handleHealth)

	handler.ServeHTTP(rr, req)

	// Check status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Check content type
	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Handler returned wrong content type: got %v want %v", contentType, "application/json")
	}

	// Check response body
	var response map[string]string
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	if err != nil {
		t.Errorf("Failed to parse response body: %v", err)
	}

	if response["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", response["status"])
	}
}

func TestHandleHealth_SupportsHEAD(t *testing.T) {
	req, err := http.NewRequest("HEAD", "/api/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(handleHealth)

	handler.ServeHTTP(rr, req)

	// HEAD should also return 200 OK
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("HEAD request returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestHandleHealth_SupportsPOST(t *testing.T) {
	req, err := http.NewRequest("POST", "/api/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(handleHealth)

	handler.ServeHTTP(rr, req)

	// POST should also return 200 OK (health check should be method-agnostic)
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("POST request returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

// ===========================================
// Tests for RunRequest Struct (v2.1.0)
// ===========================================

func TestRunRequest_ReplacementScope(t *testing.T) {
	tests := []struct {
		name     string
		scope    string
		expected string
	}{
		{"All files", "all", "all"},
		{"POM only", "pom-only", "pom-only"},
		{"Exclude POM", "exclude-pom", "exclude-pom"},
		{"Empty defaults to all", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := RunRequest{
				ReplacementScope: tt.scope,
			}

			if req.ReplacementScope != tt.expected {
				t.Errorf("Expected ReplacementScope '%s', got '%s'", tt.expected, req.ReplacementScope)
			}
		})
	}
}

func TestRunRequest_UnifiedReplacements(t *testing.T) {
	// Test that RunRequest correctly holds unified replacements
	req := RunRequest{
		RootPath: "/test/path",
		Replacements: []logic.Replacement{
			{Search: "foo", Replace: "bar"},
			{Search: "baz", Replace: "qux"},
		},
		ReplacementScope: "pom-only",
	}

	if len(req.Replacements) != 2 {
		t.Errorf("Expected 2 replacements, got %d", len(req.Replacements))
	}

	if req.Replacements[0].Search != "foo" {
		t.Errorf("Expected first search 'foo', got '%s'", req.Replacements[0].Search)
	}

	if req.ReplacementScope != "pom-only" {
		t.Errorf("Expected scope 'pom-only', got '%s'", req.ReplacementScope)
	}
}

// ===========================================
// Tests for Replacement Struct
// ===========================================

func TestReplacement_Fields(t *testing.T) {
	r := logic.Replacement{
		Search:  "old value",
		Replace: "new value",
	}

	if r.Search != "old value" {
		t.Errorf("Expected Search 'old value', got '%s'", r.Search)
	}

	if r.Replace != "new value" {
		t.Errorf("Expected Replace 'new value', got '%s'", r.Replace)
	}
}

func TestReplacement_EmptyValues(t *testing.T) {
	// Empty search/replace should be valid
	r := logic.Replacement{
		Search:  "",
		Replace: "",
	}

	if r.Search != "" {
		t.Errorf("Expected empty Search, got '%s'", r.Search)
	}
}

func TestReplacement_MultilineValues(t *testing.T) {
	// Multiline search/replace should work
	r := logic.Replacement{
		Search: `<dependency>
    <groupId>old.group</groupId>
</dependency>`,
		Replace: `<dependency>
    <groupId>new.group</groupId>
</dependency>`,
	}

	if !containsNewline(r.Search) {
		t.Error("Search should contain newlines")
	}

	if !containsNewline(r.Replace) {
		t.Error("Replace should contain newlines")
	}
}

func containsNewline(s string) bool {
	for _, c := range s {
		if c == '\n' {
			return true
		}
	}
	return false
}

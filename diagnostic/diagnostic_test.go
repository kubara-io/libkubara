package diagnostic_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kubara-io/libkubara/diagnostic"
)

func TestDiagnosticErrorFormatting(t *testing.T) {
	tests := []struct {
		name     string
		diag     diagnostic.Diagnostic
		expected string
	}{
		{
			name: "message only",
			diag: diagnostic.Diagnostic{
				Message: "something went wrong",
			},
			expected: "something went wrong",
		},
		{
			name: "cause fallback when message is empty",
			diag: diagnostic.Diagnostic{
				Cause: errors.New("underlying cause"),
			},
			expected: "underlying cause",
		},
		{
			name: "location with source and line and column and path",
			diag: diagnostic.Diagnostic{
				Source:  "config.yaml",
				Line:    12,
				Column:  4,
				Path:    "spec.replicas",
				Message: "invalid value",
			},
			expected: "config.yaml:12:4: spec.replicas: invalid value",
		},
		{
			name: "location line only with path",
			diag: diagnostic.Diagnostic{
				Line:    5,
				Path:    "metadata.name",
				Message: "name is required",
			},
			expected: "5: metadata.name: name is required",
		},
		{
			name: "path only",
			diag: diagnostic.Diagnostic{
				Path:    "spec.enabled",
				Message: "must be boolean",
			},
			expected: "spec.enabled: must be boolean",
		},
		{
			name: "source only",
			diag: diagnostic.Diagnostic{
				Source:  "test.yaml",
				Message: "parse error",
			},
			expected: "test.yaml: parse error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.diag.Error()
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestDiagnosticUnwrap(t *testing.T) {
	underlying := errors.New("io error")
	diag := diagnostic.Diagnostic{Cause: underlying}
	if !errors.Is(diag, underlying) {
		t.Fatalf("expected diag to unwrap to underlying error")
	}
}

func TestDiagnosticList(t *testing.T) {
	var list diagnostic.List
	if list.HasErrors() {
		t.Errorf("empty list should not have errors")
	}
	if err := list.Err(); err != nil {
		t.Errorf("empty list Err() should be nil, got %v", err)
	}
	if list.Error() != "" {
		t.Errorf("empty list Error() should be empty string, got %q", list.Error())
	}

	// Warning only
	list = append(list, diagnostic.Diagnostic{
		Severity: diagnostic.SeverityWarning,
		Message:  "deprecated field",
	})
	if list.HasErrors() {
		t.Errorf("list with only warnings should not report HasErrors() = true")
	}
	if err := list.Err(); err != nil {
		t.Errorf("list with only warnings should have nil Err(), got %v", err)
	}

	// Add an error
	list = append(list, diagnostic.Diagnostic{
		Severity: diagnostic.SeverityError,
		Message:  "fatal syntax error",
	})
	if !list.HasErrors() {
		t.Errorf("expected HasErrors() to be true")
	}
	err := list.Err()
	if err == nil {
		t.Fatal("expected non-nil Err()")
	}
	if !strings.Contains(err.Error(), "fatal syntax error") {
		t.Errorf("expected Err() to contain 'fatal syntax error', got %v", err)
	}

	joinedStr := list.Error()
	if !strings.Contains(joinedStr, "deprecated field") || !strings.Contains(joinedStr, "fatal syntax error") {
		t.Errorf("unexpected list.Error(): %q", joinedStr)
	}
}

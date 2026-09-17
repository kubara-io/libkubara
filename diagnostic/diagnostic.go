// Package diagnostic provides structured diagnostic types for error and warning
// reporting across manifest decoding, CRD compilation, and resource validation.
package diagnostic

import (
	"errors"
	"fmt"
	"strings"
)

// Severity indicates whether a diagnostic represents an error or a non-fatal warning.
type Severity string

const (
	// SeverityError indicates a validation or parsing failure that prevents progress.
	SeverityError Severity = "error"
	// SeverityWarning indicates a non-fatal issue, such as a deprecated schema field.
	SeverityWarning Severity = "warning"
)

// Diagnostic represents an issue encountered during manifest decoding or validation,
// containing severity, location metadata, and error details.
type Diagnostic struct {
	// Severity indicates if the diagnostic is an error or warning.
	Severity Severity
	// Source is the file path or reader name where the diagnostic occurred.
	Source string
	// Document is the 0-indexed document number within a multi-document stream.
	Document int
	// Path is the JSON/field path within the object (e.g. "spec.replicas").
	Path string
	// Line is the 1-indexed line number in the source file, if known.
	Line int
	// Column is the 1-indexed column number in the source file, if known.
	Column int
	// Code is a machine-readable classification code for the diagnostic.
	Code string
	// Message is the human-readable explanation of the issue.
	Message string
	// Cause is the underlying error that triggered this diagnostic, if any.
	Cause error
}

// Error formats the diagnostic into a human-readable string including available location information.
func (d Diagnostic) Error() string {
	var location string
	if d.Source != "" {
		location = d.Source
	}
	if d.Line > 0 {
		position := fmt.Sprintf("%d", d.Line)
		if d.Column > 0 {
			position += fmt.Sprintf(":%d", d.Column)
		}
		if location == "" {
			location = position
		} else {
			location += ":" + position
		}
	}
	if d.Path != "" {
		if location == "" {
			location = d.Path
		} else {
			location += ": " + d.Path
		}
	}

	message := d.Message
	if message == "" && d.Cause != nil {
		message = d.Cause.Error()
	}
	if location == "" {
		return message
	}
	return location + ": " + message
}

// Unwrap returns the underlying cause of the diagnostic, allowing errors.Is and errors.As traversal.
func (d Diagnostic) Unwrap() error { return d.Cause }

// List is a collection of Diagnostics.
type List []Diagnostic

// HasErrors returns true if any diagnostic in the list has SeverityError.
func (l List) HasErrors() bool {
	for _, item := range l {
		if item.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Err returns a joined error combining all diagnostics that have SeverityError.
// If the list contains no errors, it returns nil.
func (l List) Err() error {
	errs := make([]error, 0, len(l))
	for _, item := range l {
		if item.Severity == SeverityError {
			errs = append(errs, item)
		}
	}
	return errors.Join(errs...)
}

// Error formats all diagnostics in the list separated by newlines.
func (l List) Error() string {
	parts := make([]string, 0, len(l))
	for _, item := range l {
		parts = append(parts, item.Error())
	}
	return strings.Join(parts, "\n")
}

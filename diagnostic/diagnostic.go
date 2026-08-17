package diagnostic

import (
	"errors"
	"fmt"
	"strings"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

type Diagnostic struct {
	Severity Severity
	Source   string
	Document int
	Path     string
	Line     int
	Column   int
	Code     string
	Message  string
	Cause    error
}

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

func (d Diagnostic) Unwrap() error { return d.Cause }

type List []Diagnostic

func (l List) HasErrors() bool {
	for _, item := range l {
		if item.Severity == SeverityError {
			return true
		}
	}
	return false
}

func (l List) Err() error {
	errs := make([]error, 0, len(l))
	for _, item := range l {
		if item.Severity == SeverityError {
			errs = append(errs, item)
		}
	}
	return errors.Join(errs...)
}

func (l List) Error() string {
	parts := make([]string, 0, len(l))
	for _, item := range l {
		parts = append(parts, item.Error())
	}
	return strings.Join(parts, "\n")
}

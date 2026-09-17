package diagnostic_test

import (
	"fmt"

	"github.com/kubara-io/libkubara/diagnostic"
)

func ExampleDiagnostic_Error() {
	diag := diagnostic.Diagnostic{
		Severity: diagnostic.SeverityError,
		Source:   "deployment.yaml",
		Line:     18,
		Column:   7,
		Path:     "spec.replicas",
		Message:  "must be greater than or equal to 1",
	}

	fmt.Println(diag.Error())

	// Output:
	// deployment.yaml:18:7: spec.replicas: must be greater than or equal to 1
}

func ExampleList_Err() {
	var diagnostics diagnostic.List

	// Warnings do not cause Err() to fail
	diagnostics = append(diagnostics, diagnostic.Diagnostic{
		Severity: diagnostic.SeverityWarning,
		Path:     "spec.deprecatedField",
		Message:  "field is deprecated in v1",
	})
	fmt.Printf("Has errors with warning: %v, Err: %v\n", diagnostics.HasErrors(), diagnostics.Err())

	// Adding an error makes HasErrors true and Err non-nil
	diagnostics = append(diagnostics, diagnostic.Diagnostic{
		Severity: diagnostic.SeverityError,
		Path:     "spec.replicas",
		Message:  "required value",
	})
	fmt.Printf("Has errors with error: %v, Err != nil: %v\n", diagnostics.HasErrors(), diagnostics.Err() != nil)

	// Output:
	// Has errors with warning: false, Err: <nil>
	// Has errors with error: true, Err != nil: true
}

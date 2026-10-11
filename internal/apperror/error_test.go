package apperror_test

import (
	"testing"

	"locus-scope/internal/apperror"
)

func TestWrapMergesDetailsWhenExistingDetailsAreNil(t *testing.T) {
	existing := apperror.New(apperror.InvalidData, "scope.invalid", "invalid declaration", nil)
	wrapped := apperror.Wrap(apperror.InvalidData, "scope.mutation_invalid", "mutation failed", existing, map[string]string{"operation": "scope.mutate"})

	application := apperror.Normalize(wrapped, "test")
	if application.Code != apperror.InvalidData || application.Reason != "scope.invalid" {
		t.Fatalf("application error = %#v", application)
	}
	if application.Details["operation"] != "scope.mutate" {
		t.Fatalf("details = %#v", application.Details)
	}
}

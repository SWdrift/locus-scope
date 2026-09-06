package apperror

import (
	"errors"
	"fmt"
)

type Code string

const (
	InvalidArgument    Code = "invalid_argument"
	InvalidData        Code = "invalid_data"
	NotFound           Code = "not_found"
	FailedPrecondition Code = "failed_precondition"
	Conflict           Code = "conflict"
	IntegrityFailed    Code = "integrity_failed"
	Unauthenticated    Code = "unauthenticated"
	PermissionDenied   Code = "permission_denied"
	Unavailable        Code = "unavailable"
	Internal           Code = "internal"
)

type Error struct {
	Code    Code              `json:"code"`
	Reason  string            `json:"reason"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
	Cause   error             `json:"-"`
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Cause }

func New(code Code, reason, message string, details map[string]string) error {
	return &Error{Code: code, Reason: reason, Message: message, Details: cloneDetails(details)}
}

func Wrap(code Code, reason, message string, cause error, details map[string]string) error {
	if cause == nil {
		return New(code, reason, message, details)
	}
	var existing *Error
	if errors.As(cause, &existing) {
		merged := cloneDetails(existing.Details)
		for key, value := range details {
			if _, exists := merged[key]; !exists {
				merged[key] = value
			}
		}
		return &Error{Code: existing.Code, Reason: existing.Reason, Message: message, Details: merged, Cause: cause}
	}
	return &Error{Code: code, Reason: reason, Message: message, Details: cloneDetails(details), Cause: cause}
}

func Normalize(err error, operation string) *Error {
	var application *Error
	if errors.As(err, &application) {
		return application
	}
	return &Error{Code: Internal, Reason: "scope.internal", Message: err.Error(), Details: map[string]string{"operation": operation}, Cause: err}
}

func CodeOf(err error) Code {
	var application *Error
	if errors.As(err, &application) {
		return application.Code
	}
	return Internal
}
func ExitCode(err error) int {
	if CodeOf(err) == InvalidArgument {
		return 2
	}
	return 1
}
func cloneDetails(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
func Invalid(reason string, err error) error {
	return Wrap(InvalidArgument, reason, err.Error(), err, nil)
}
func Message(code Code, reason string, format string, values ...any) error {
	return New(code, reason, fmt.Sprintf(format, values...), nil)
}

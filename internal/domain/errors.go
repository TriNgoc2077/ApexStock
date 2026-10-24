package domain

import (
	"fmt"
	"net/http"
)

// Error is a public business error that can be rendered using the API envelope.
type Error struct {
	Code    string
	Message string
	Status  int
	Details map[string]any
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func ValidationError(field, message string) *Error {
	return &Error{Code: "validation_error", Message: message, Status: http.StatusBadRequest, Details: map[string]any{"field": field}}
}

func NotFoundError(code, message string) *Error {
	return &Error{Code: code, Message: message, Status: http.StatusNotFound}
}

func ConflictError(code, message string, details map[string]any) *Error {
	return &Error{Code: code, Message: message, Status: http.StatusConflict, Details: details}
}

func ServiceUnavailableError() *Error {
	return &Error{Code: "service_unavailable", Message: "Hệ thống tạm thời gián đoạn, vui lòng thử lại", Status: http.StatusServiceUnavailable}
}

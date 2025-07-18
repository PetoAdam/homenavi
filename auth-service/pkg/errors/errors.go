package errors

import (
	"encoding/json"
	"net/http"
)

type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}

func NewAppError(code int, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

func BadRequest(message string) *AppError {
	return NewAppError(http.StatusBadRequest, message, nil)
}

func Unauthorized(message string) *AppError {
	return NewAppError(http.StatusUnauthorized, message, nil)
}

func Forbidden(message string) *AppError {
	return NewAppError(http.StatusForbidden, message, nil)
}

func NotFound(message string) *AppError {
	return NewAppError(http.StatusNotFound, message, nil)
}

func InternalServerError(message string, err error) *AppError {
	return NewAppError(http.StatusInternalServerError, message, err)
}

func WriteError(w http.ResponseWriter, err *AppError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.Code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   err.Message,
		"code":    err.Code,
	})
}

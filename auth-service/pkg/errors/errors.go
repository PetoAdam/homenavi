package errors

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
	Fields  map[string]interface{} `json:"-"`
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
		Fields:  make(map[string]interface{}),
	}
}

// WithField adds a single additional field to be serialized with the error response.
func (e *AppError) WithField(key string, value interface{}) *AppError {
	if e.Fields == nil { e.Fields = make(map[string]interface{}) }
	e.Fields[key] = value
	return e
}

// WithFields bulk-adds fields to the error.
func (e *AppError) WithFields(fields map[string]interface{}) *AppError {
	if e.Fields == nil { e.Fields = make(map[string]interface{}) }
	for k,v := range fields { e.Fields[k] = v }
	return e
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
	if err.Code == http.StatusLocked {
		if v, ok := err.Fields["lockout_remaining"]; ok {
			switch t := v.(type) {
			case int: if t > 0 { w.Header().Set("Retry-After", strconv.Itoa(t)) }
			case int64: if t > 0 { w.Header().Set("Retry-After", strconv.FormatInt(t,10)) }
			case float64: if t > 0 { w.Header().Set("Retry-After", strconv.Itoa(int(t))) }
			}
		}
	}
	w.WriteHeader(err.Code)
	payload := map[string]interface{}{
		"error": err.Message,
		"code":  err.Code,
	}
	for k,v := range err.Fields { // include any supplemental fields
		// avoid overwriting core keys
		if k == "error" || k == "code" { continue }
		payload[k] = v
	}
	json.NewEncoder(w).Encode(payload)
}

package cigExchange

import (
	"fmt"
	"net/http"
)

// NotFoundHandler returns an error when requested resourse / route is missing
var NotFoundHandler = func(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		Respond(w, Message(false, "This resources was not found on our server"))
		next.ServeHTTP(w, r)
	})
}

// top level API Error types
const (
	ErrorTypeBadRequest      = "Bad request"
	ErrorTypeUnauthorized    = "Unauthorized"
	ErrorTypeUnprocessable   = "Unprocessable entity"
	ErrorTypeDatabaseFailure = "Database error"
	ErrorTypeRedisFailure    = "Redis error"
	ErrorTypeTwilioFailure   = "Twilio error"
)

// nested API Error reasons
const (
	NestedErrorUserAlreadyExists = "User already exists"
	NestedErrorUserDoesntExist   = "User doesn't exist"
	NestedErrorFieldMissing      = "Missing field"
	NestedErrorFieldInvalid      = "Invalid field"
	NestedErrorGormFailure       = "GORM failure"
	NestedErrorRedisFailure      = "Redis failure"
	NestedErrorTwilioFailure     = "Twilio failure"
	NestedErrorJSONFailure       = "JSON decoding failure"
)

// APIError is a custom error type that gets reported to the client
// conforms to https://github.com/gocardless/http-api-design
type APIError struct {
	Type    string            `json:"type"`
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Errors  []*NestedAPIError `json:"errors,omitempty"`
}

// NestedAPIError represents a detailed error description
type NestedAPIError struct {
	Field         string `json:"field,omitempty"`
	Reason        string `json:"reason"`
	Message       string `json:"message"`
	OriginalError error  `json:"-"`
}

// NewNestedError inserts a new nested error
func (e *APIError) NewNestedError() *NestedAPIError {

	nestedError := &NestedAPIError{}
	e.Errors = append(e.Errors, nestedError)
	return nestedError
}

// SetErrorType sets the top level error type and corresponding error code
func (e *APIError) SetErrorType(errType string) {

	e.Type = errType
	e.Message = errType

	// choose the corresponding error code
	switch e.Type {
	case ErrorTypeBadRequest:
		e.Code = 400
	case ErrorTypeUnauthorized:
		e.Code = 401
	case ErrorTypeUnprocessable:
		e.Code = 422
	case ErrorTypeDatabaseFailure:
	case ErrorTypeRedisFailure:
		e.Code = 503
	default:
		// 500 is the default for any uncategorized errors
		e.Code = 500
	}
}

// ShouldSilenceError returns true if the error is not intended to be shown to end user for security reasons
// used in AuthApi to prevent existing emails / phone numbers discovery
func (e *APIError) ShouldSilenceError() bool {

	// only silence errors that have valid nested errors
	if len(e.Errors) == 0 {
		return false
	}

	// silense "Unauthorized : User Already Exists" error
	if e.Type == ErrorTypeUnauthorized && e.Errors[0].Reason == NestedErrorUserAlreadyExists {
		return true
	}

	// silense "Unauthorized : User Doesn't Exist" error
	if e.Type == ErrorTypeUnauthorized && e.Errors[0].Reason == NestedErrorUserDoesntExist {
		return true
	}

	return false
}

// ToString creates a string representation of the error
func (e *APIError) ToString() string {
	res := fmt.Sprintf("[%d] %s", e.Code, e.Type)
	for _, nested := range e.Errors {
		res += fmt.Sprintf("\n%s : %s", nested.Reason, nested.Message)
		if len(nested.Field) > 0 {
			res += " [" + nested.Field + "]"
		}
		if nested.OriginalError != nil {
			res += " " + nested.OriginalError.Error()
		}
	}

	return res
}

// Helper functions for creating specific errors

// NewGormError creates APIError with ErrorTypeDatabaseFailure
// and nested error with NestedErrorGormFailure reason
func NewGormError(message string, err error) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeDatabaseFailure)

	nesetedError := apiErr.NewNestedError()
	nesetedError.Reason = NestedErrorGormFailure
	nesetedError.Message = message
	nesetedError.OriginalError = err

	return apiErr
}

// NewRedisError creates APIError with ErrorTypeRedisFailure
// and nested error with NestedErrorRedisFailure reason
func NewRedisError(message string, err error) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeRedisFailure)

	nesetedError := apiErr.NewNestedError()
	nesetedError.Reason = NestedErrorRedisFailure
	nesetedError.Message = message
	nesetedError.OriginalError = err

	return apiErr
}

// NewTwilioError creates APIError with ErrorTypeTwilioFailure
// and nested error with NestedErrorTwilioFailure reason
func NewTwilioError(message string, err error) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeTwilioFailure)

	nesetedError := apiErr.NewNestedError()
	nesetedError.Reason = NestedErrorTwilioFailure
	nesetedError.Message = message
	nesetedError.OriginalError = err

	return apiErr
}

// NewUserDoesntExistError creates APIError with ErrorTypeUnauthorized
// and nested error with NestedErrorUserDoesntExist reason
// This error is silenced by default (not shown to the client by authAPI)
func NewUserDoesntExistError(message string) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeUnauthorized)

	nesetedError := apiErr.NewNestedError()
	nesetedError.Reason = NestedErrorUserDoesntExist
	nesetedError.Message = message

	return apiErr
}

// NewRequiredFieldError creates APIError with ErrorTypeUnprocessable
// and nested error(s) with NestedErrorFieldMissing reason and filled field name
func NewRequiredFieldError(fields []string) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeUnprocessable)

	for _, fieldName := range fields {
		nesetedError := apiErr.NewNestedError()
		nesetedError.Reason = NestedErrorFieldMissing
		nesetedError.Field = fieldName
		nesetedError.Message = "Required field missing"
	}

	return apiErr
}

// NewInvalidFieldError creates APIError with ErrorTypeUnprocessable
// and nested error with NestedErrorFieldInvalid reason with filled message and field name
func NewInvalidFieldError(fieldName, message string) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeUnprocessable)

	nesetedError := apiErr.NewNestedError()
	nesetedError.Reason = NestedErrorFieldInvalid
	nesetedError.Field = fieldName
	nesetedError.Message = message

	return apiErr
}

// NewJSONDecodingError creates APIError with NewBadRequestError
// and nested error with NestedErrorJSONFailure reason
func NewJSONDecodingError(err error) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeBadRequest)

	nesetedError := apiErr.NewNestedError()
	nesetedError.Reason = NestedErrorJSONFailure
	nesetedError.Message = "Request body decoding failed"
	nesetedError.OriginalError = err

	return apiErr
}

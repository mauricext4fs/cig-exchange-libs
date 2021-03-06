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
	ErrorTypeBadRequest          = "Bad request"
	ErrorTypeUnauthorized        = "Unauthorized"
	ErrorTypeForbidden           = "Forbidden"
	ErrorTypeInternalServer      = "Internal server error"
	ErrorTypeUnprocessableEntity = "Unprocessable Entity"
)

// nested API Error reasons
const (
	ReasonUserAlreadyExists           = "User already exists"
	ReasonInvitationAlreadyExists     = "User is already invited"
	ReasonInvitationAlreadyAccepted   = "Invitation is already accepted"
	ReasonUserDoesntExist             = "User doesn't exist"
	ReasonOrganisationDoesntExist     = "Organisation doesn't exist"
	ReasonOrganisationUserDoesntExist = "Organisation User doesn't exist"
	ReasonNotAllowed                  = "Not allowed / wrong permissions"
	ReasonFieldMissing                = "Required field missing"
	ReasonFieldInvalid                = "Invalid field"
	ReasonJSONFailure                 = "JSON decoding failure"
	ReasonDatabaseFailure             = "Database error"
	ReasonUserActivityFailure         = "Record user activity error"
	ReasonReadFailure                 = "Read request body error"
	ReasonRedisFailure                = "Redis error"
	ReasonTwilioFailure               = "Twilio error"
	ReasonMandrillFailure             = "Mandrill error"
	ReasonTokenGenerationFailure      = "JWT generation error"
	ReasonRoutingFailure              = "Routing error"
)

// nested API Error messages
const (
	MessageResponseJSONEncoding = "Response JSON encoding failed"
	MessageRequestJSONDecoding  = "Request JSON parsing failed"
	MessageJSONEncoding         = "JSON encoding failed"
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
func (e *APIError) NewNestedError(reason, message string) *NestedAPIError {

	nestedError := &NestedAPIError{
		Reason:  reason,
		Message: message,
	}
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
	case ErrorTypeUnprocessableEntity:
		e.Code = 422
	case ErrorTypeInternalServer:
		e.Code = 500
	default:
		// 500 is the default for any uncategorized errors
		e.Code = 500
		e.Message = "Unknown server error"
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
	if e.Type == ErrorTypeUnauthorized && e.Errors[0].Reason == ReasonUserAlreadyExists {
		return true
	}

	// silense "Unauthorized : User Doesn't Exist" error
	if e.Type == ErrorTypeUnauthorized && e.Errors[0].Reason == ReasonUserDoesntExist {
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

// NewInternalServerError creates APIError with ErrorTypeInternalServer
func NewInternalServerError(reason, message string) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeInternalServer)

	apiErr.NewNestedError(reason, message)

	return apiErr
}

// NewDatabaseError creates APIError with ErrorTypeInternalServer
// and nested error with ReasonDatabaseFailure reason
func NewDatabaseError(message string, err error) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeInternalServer)

	nesetedError := apiErr.NewNestedError(ReasonDatabaseFailure, message)
	nesetedError.OriginalError = err

	return apiErr
}

// NewReadError creates APIError with ErrorTypeBadRequest
// and nested error with ReasonReadFailure reason
func NewReadError(message string, err error) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeBadRequest)

	nesetedError := apiErr.NewNestedError(ReasonReadFailure, message)
	nesetedError.OriginalError = err

	return apiErr
}

// NewRedisError creates APIError with ErrorTypeInternalServer
// and nested error with ReasonRedisFailure reason
func NewRedisError(message string, err error) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeInternalServer)

	nesetedError := apiErr.NewNestedError(ReasonRedisFailure, message)
	nesetedError.OriginalError = err

	return apiErr
}

// NewTwilioError creates APIError with ErrorTypeInternalServer
// and nested error with ReasonTwilioFailure reason
func NewTwilioError(message string, err error) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeInternalServer)

	nesetedError := apiErr.NewNestedError(ReasonTwilioFailure, message)
	nesetedError.OriginalError = err

	return apiErr
}

// NewTokenError creates APIError with ErrorTypeInternalServer
// and nested error with ReasonTokenGenerationFailure reason
func NewTokenError(message string, err error) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeInternalServer)

	nesetedError := apiErr.NewNestedError(ReasonTokenGenerationFailure, message)
	nesetedError.OriginalError = err

	return apiErr
}

// NewRoutingError creates APIError with ErrorTypeInternalServer
// and nested error with NestedErrorJSONFailure reason
func NewRoutingError(err error) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeInternalServer)

	nesetedError := apiErr.NewNestedError(ReasonRoutingFailure, "Unexpected routing error")
	nesetedError.OriginalError = err
	return apiErr
}

// NewUserDoesntExistError creates APIError with ErrorTypeUnauthorized
// and nested error with ReasonUserDoesntExist reason
// This error is silenced by default (not shown to the client by authAPI)
func NewUserDoesntExistError(message string) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeUnauthorized)
	apiErr.NewNestedError(ReasonUserDoesntExist, message)
	return apiErr
}

// NewOrganisationDoesntExistError creates APIError with ErrorTypeBadRequest
// and nested error with ReasonOrganisationDoesntExist reason
func NewOrganisationDoesntExistError(message string) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeBadRequest)
	apiErr.NewNestedError(ReasonOrganisationDoesntExist, message)
	return apiErr
}

// NewOrganisationUserDoesntExistError creates APIError with ErrorTypeBadRequest
// and nested error with ReasonOrganisationDoesntExist reason
func NewOrganisationUserDoesntExistError(message string) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeBadRequest)
	apiErr.NewNestedError(ReasonOrganisationUserDoesntExist, message)
	return apiErr
}

// NewAccessRightsError creates APIError with ErrorTypeUnauthorized
// and nested error with ReasonNotAllowed reason
func NewAccessRightsError(message string) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeUnauthorized)
	apiErr.NewNestedError(ReasonNotAllowed, message)
	return apiErr
}

// NewAccessForbiddenError creates APIError with ErrorTypeForbidden
// and nested error with ReasonNotAllowed reason
func NewAccessForbiddenError(message string) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeForbidden)
	apiErr.NewNestedError(ReasonNotAllowed, message)
	return apiErr
}

// NewRequiredFieldError creates APIError with ErrorTypeBadRequest
// and nested error(s) with NestedErrorFieldMissing reason and filled field name
func NewRequiredFieldError(fields []string) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeBadRequest)

	for _, fieldName := range fields {
		nesetedError := apiErr.NewNestedError(ReasonFieldMissing, "Required field missing")
		nesetedError.Field = fieldName
	}
	return apiErr
}

// NewInvalidFieldError creates APIError with ErrorTypeBadRequest
// and nested error with ReasonFieldInvalid reason with filled message and field name
func NewInvalidFieldError(fieldName, message string) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeBadRequest)

	nesetedError := apiErr.NewNestedError(ReasonFieldInvalid, message)
	nesetedError.Field = fieldName
	return apiErr
}

// NewJSONDecodingError creates APIError with ErrorTypeBadRequest
// and nested error with NestedErrorJSONFailure reason
func NewJSONDecodingError(message string, err error) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeBadRequest)

	nesetedError := apiErr.NewNestedError(ReasonJSONFailure, message)
	nesetedError.OriginalError = err
	return apiErr
}

// NewLanguagesJSONDecodingError creates APIError with ErrorTypeBadRequest
// and nested error with NestedErrorJSONFailure reason
func NewLanguagesJSONDecodingError(err error) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeBadRequest)

	nesetedError := apiErr.NewNestedError(ReasonJSONFailure, "Request body decoding failed. Expected format for 'offering_direct_url' filed:{\"en\":\"url\",\"fr\":\"url\",\"it\":\"url\",\"de\":\"url\"}.")
	nesetedError.OriginalError = err
	return apiErr
}

// NewJSONEncodingError creates APIError with ErrorTypeBadRequest
// and nested error with NestedErrorJSONFailure reason
func NewJSONEncodingError(message string, err error) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeBadRequest)

	nesetedError := apiErr.NewNestedError(ReasonJSONFailure, message)
	nesetedError.OriginalError = err
	return apiErr
}

// NewRequestDecodingError creates APIError with ErrorTypeBadRequest
// and nested error with NestedErrorJSONFailure reason
func NewRequestDecodingError(err error) *APIError {
	apiErr := &APIError{}
	apiErr.SetErrorType(ErrorTypeBadRequest)

	nesetedError := apiErr.NewNestedError(ReasonJSONFailure, MessageRequestJSONDecoding)
	nesetedError.OriginalError = err
	return apiErr
}

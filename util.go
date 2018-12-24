package cigExchange

import (
	"encoding/json"
	"net/http"
)

// apiError is a struct representing server error response
type apiError struct {
	Message string
	Code    int
}

// Message creates api call response format
func Message(status bool, message string) map[string]interface{} {
	return map[string]interface{}{"status": status, "message": message}
}

// RespondWithError writes error message into http response
func RespondWithError(w http.ResponseWriter, statusCode int, err error) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	resp := make(map[string]interface{})
	resp["error"] = apiError{
		Message: err.Error(),
		Code:    statusCode,
	}
	json.NewEncoder(w).Encode(resp)
}

// Respond writes object into http response
func Respond(w http.ResponseWriter, object interface{}) {
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(object)
}

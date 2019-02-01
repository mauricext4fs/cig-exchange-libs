package cigExchange

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
)

const letterBytes = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// RandCode generates random access code for email auth
func RandCode(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

// GenerateRedisKey generates key for storing email auth access code in redis
func GenerateRedisKey(UUID string) string {
	return fmt.Sprintf("%s_signup_key", UUID)
}

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

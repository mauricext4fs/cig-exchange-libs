package cigExchange

import (
	"errors"
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

// Errors that are not intended to be shown to end user in auth API
var ErrUserNotFound = errors.New("User doesn't exist")
var ErrUserAlreadyExists = errors.New("User already exist")

// ShouldSilenceError returns true if the error is not intended to be shown to end user
// for security reasons
func ShouldSilenceError(err error) bool {
	if err == ErrUserNotFound || err == ErrUserAlreadyExists {
		return true
	}
	return false
}

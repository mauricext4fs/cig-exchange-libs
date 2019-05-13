package cigExchange

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mattbaird/gochimp"
	uuid "github.com/satori/go.uuid"
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

// RandomUUID generates new random V4 UUID string
func RandomUUID() string {
	UUID, err := uuid.NewV4()
	if err != nil {
		// uuid for an unlikely event of NewV4 failure
		fmt.Printf("[WARNING] Error creating V4 UUID, generating it manually: %v", err.Error())
		res := RandCode(8) + "-" + RandCode(4) + "-" + RandCode(4) + "-" + RandCode(4) + "-" + RandCode(12)
		return strings.ToLower(res)
	}
	return UUID.String()
}

// keys for storing strings in redis
const (
	KeySignUp           = "_signup_key"
	KeyWebAuthnRegister = "_web_authn_register"
	KeyWebAuthnLogin    = "_web_authn_login"
)

// GenerateRedisKey generates key for storing strings in redis
func GenerateRedisKey(UUID, suffix string) string {
	return fmt.Sprintf("%s%s", UUID, suffix)
}

// BEGIN SECTION: this api will be deprecated soon

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

// END SECTION: this api will be deprecated soon

// RespondWithAPIError writes APIError into http.ResponseWriter,
// populates the content type and request status code
func RespondWithAPIError(w http.ResponseWriter, apiErr *APIError) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(apiErr.Code)
	json.NewEncoder(w).Encode(apiErr)
}

// PrintAPIError prints apiError
func PrintAPIError(info *ActivityInformation) {
	if info.APIError != nil {
		fmt.Println(info.APIError.ToString())
	}
}

// LoggedInUser is passed to controllers after jwt auth
type LoggedInUser struct {
	UserUUID         string    `json:"user_id"`
	OrganisationUUID string    `json:"organisation_id"`
	CreationDate     time.Time `json:"creation_date"`
	ExpirationDate   time.Time `json:"expiration_date"`
}

// ActivityInformation stores activity information for logging
type ActivityInformation struct {
	APIError     *APIError
	LoggedInUser *LoggedInUser
	RemoteAddr   string
}

// PrepareActivityInformation creates ActivityInformation with prefilled remote address
// X-Real-IP examined first, X-Forwarded-For examined if X-Real-IP is not present
func PrepareActivityInformation(r *http.Request) *ActivityInformation {

	info := &ActivityInformation{}
	remoteIP := r.Header.Get("X-Real-IP")
	if len(remoteIP) == 0 {
		forwardedForParts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		remoteIP = forwardedForParts[0]
	}

	info.RemoteAddr = remoteIP
	return info
}

type emailType int

// Constants defining email type
const (
	EmailTypeWelcome emailType = iota
	EmailTypePinCode
	EmailTypeInvitation
)

// SendEmail sends template emails
func SendEmail(eType emailType, email string, parameters map[string]string) error {

	mandrillClient := GetMandrill()

	subject := ""
	templateName := ""
	mergeVars := make([]gochimp.Var, 0)

	switch eType {
	case EmailTypeWelcome:
		templateName = "welcome"
		subject = "Welcome aboard!"
	case EmailTypePinCode:
		templateName = "pin-code"
		subject = "CIG Exchange Verification Code"
	case EmailTypeInvitation:
		templateName = "invitation"
		subject = "CIG Exchange Invitation"
	default:
		return fmt.Errorf("Unsupported email type: %v", eType)
	}

	for key, value := range parameters {
		mVar := gochimp.Var{
			Name:    key,
			Content: value,
		}
		mergeVars = append(mergeVars, mVar)
	}

	// TemplateRender sometimes returns zero length string without giving any error (wtf???)
	// retry is a workaround that helps to render it properly
	renderedTemplate := ""
	attempts := 0
	for {
		if len(renderedTemplate) > 0 {
			break
		}
		if attempts > 5 {
			return fmt.Errorf("Mandrill failure: unable to render template in %v attempts", attempts)
		}
		var err error
		renderedTemplate, err = mandrillClient.TemplateRender(templateName, []gochimp.Var{}, mergeVars)
		if err != nil {
			return err
		}
		attempts++
	}

	recipients := []gochimp.Recipient{
		gochimp.Recipient{Email: email},
	}

	message := gochimp.Message{
		Html:      renderedTemplate,
		Subject:   subject,
		FromEmail: os.Getenv("FROM_EMAIL"),
		FromName:  "CIG Exchange",
		To:        recipients,
	}

	_, err := mandrillClient.MessageSend(message, false)
	return err
}

// ParseIndex parses required field 'index' from map
func ParseIndex(originalRequest map[string]interface{}) (int32, *APIError) {

	index := int32(0)
	// get index from original map
	if indexVal, ok := originalRequest["index"]; ok {
		if indexInt, ok := convertToInt32(indexVal); ok {
			index = indexInt
		} else {
			return index, NewInvalidFieldError("index", "Index is not integer")
		}
	} else {
		return index, NewInvalidFieldError("index", "Index is missing")
	}
	return index, nil
}

// convertToInt32 converts interface to int32
func convertToInt32(val interface{}) (returnVal int32, result bool) {

	var i int32
	result = true

	switch t := val.(type) {
	case int:
		i = int32(t)
	case int8:
		i = int32(t)
	case int16:
		i = int32(t)
	case int32:
		i = t
	case int64:
		i = int32(t)
	case bool:
		if t {
			i = 1
		} else {
			i = 0
		}
	case float32:
		i = int32(t)
	case float64:
		i = int32(t)
	case uint8:
		i = int32(t)
	case uint16:
		i = int32(t)
	case uint32:
		i = int32(t)
	case uint64:
		i = int32(t)
	default:
		result = false
	}
	return i, result
}

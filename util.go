package cigExchange

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"reflect"
	"sort"

	"github.com/mattbaird/gochimp"
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
func PrintAPIError(apiErrorP **APIError) {
	if *apiErrorP != nil {
		fmt.Println((*apiErrorP).ToString())
	}
}

// FilterUnknownFields prepares map[string]interface{} for gorm Update
func FilterUnknownFields(model interface{}, fields []string, d map[string]interface{}) map[string]interface{} {

	result := make(map[string]interface{})

	ignoreFields := [3]string{"created_at", "updated_at", "deleted_at"}

	s := reflect.ValueOf(model).Elem()
	typeOfP := s.Type()
	// iterate fields
	for i := 0; i < s.NumField(); i++ {
		for jsonName, value := range d {
			// always skip ignored fields
			for _, ignoreField := range ignoreFields {
				if jsonName == ignoreField {
					continue
				}
			}
			if typeOfP.Field(i).Tag.Get("json") == jsonName {
				result[jsonName] = value
			} else {
				// don't filter keys from 'fields'
				sort.Strings(fields)
				i := sort.SearchStrings(fields, jsonName)
				if i < len(fields) && fields[i] == jsonName {
					result[jsonName] = value
				}
			}
		}
	}

	return result
}

type emailType int

// Constants defining email type
const (
	EmailTypeWelcome emailType = iota
	EmailTypePinCode
)

// SendEmail sends template emails
func SendEmail(eType emailType, email, pinCode string) error {

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
		mVar := gochimp.Var{
			Name:    "pincode",
			Content: pinCode,
		}
		mergeVars = append(mergeVars, mVar)
	default:
		return fmt.Errorf("Unsupported email type: %v", eType)
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

package cigExchange

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"reflect"
	"sort"

	"github.com/jinzhu/gorm/dialects/postgres"
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

// MultilangModel interface for all multilang models
type MultilangModel interface {
	GetMultilangFields() []string
}

// FilterUnknownFields prepares map[string]interface{} for gorm Update
func FilterUnknownFields(model MultilangModel, d map[string]interface{}) map[string]interface{} {

	result := make(map[string]interface{})

	ignoreFields := [3]string{"created_at", "updated_at", "deleted_at"}

	s := reflect.ValueOf(model).Elem()
	typeOfP := s.Type()

	// get multilang fields and sort for search
	fields := model.GetMultilangFields()
	sort.Strings(fields)

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
				// keep multilang fields
				i := sort.SearchStrings(fields, jsonName)
				if i < len(fields) && fields[i] == jsonName {
					result[jsonName] = value
				}
			}
		}
	}

	return result
}

// MultilangString contains multilanguage string
type MultilangString struct {
	En string `json:"en"`
	It string `json:"it"`
	Fr string `json:"fr"`
	De string `json:"de"`
}

// PrepareResponseForMultilangModel converts model to map with all multilang fields as jsonb
func PrepareResponseForMultilangModel(model MultilangModel) (map[string]interface{}, *APIError) {

	modelMap := make(map[string]interface{})
	// marshal to json
	modelBytes, err := json.Marshal(model)
	if err != nil {
		return modelMap, NewJSONEncodingError(err)
	}

	// fill map
	err = json.Unmarshal(modelBytes, &modelMap)
	if err != nil {
		return modelMap, NewJSONDecodingError(err)
	}

	// handle multilanguage text
	for _, name := range model.GetMultilangFields() {

		// prepare default value
		mString := MultilangString{}

		val, ok := modelMap[name]
		if ok {
			// convert interface to struct and filter unknown fields
			valBytes, err := json.Marshal(val)
			if err != nil {
				return modelMap, NewJSONDecodingError(err)
			}

			if err := json.Unmarshal(valBytes, &mString); err != nil {
				return modelMap, NewJSONDecodingError(err)
			}
		}

		modelMap[name+"_map"] = mString
		modelMap[name] = mString.En
	}

	return modelMap, nil
}

// ConvertRequestMapToJSONB replaces multilang string to jsonb if needed
func ConvertRequestMapToJSONB(offeringMap *map[string]interface{}, model MultilangModel) *APIError {

	localMap := *offeringMap

	for _, name := range model.GetMultilangFields() {
		val, ok := localMap[name]
		if !ok {
			continue
		}
		switch v := val.(type) {
		case string:
			strVal := `{"en":"` + v + `"}`
			metadata := json.RawMessage(strVal)
			localMap[name] = postgres.Jsonb{RawMessage: metadata}
		case int32, int64:
			return NewInvalidFieldError(name, "Field '"+name+"' has invalid type")
		default:
			mapB, err := json.Marshal(v)
			if err != nil {
				return NewJSONEncodingError(err)
			}
			metadata := json.RawMessage(mapB)
			localMap[name] = postgres.Jsonb{RawMessage: metadata}
		}
	}
	return nil
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

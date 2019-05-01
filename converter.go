package cigExchange

import (
	"encoding/json"
	"io"
	"reflect"
	"sort"

	"github.com/jinzhu/gorm/dialects/postgres"
)

// MultilangModel interface for all multilang models
type MultilangModel interface {
	GetMultilangFields() []string
}

// MultilangString contains multilanguage string
type MultilangString struct {
	En string `json:"en"`
	It string `json:"it"`
	Fr string `json:"fr"`
	De string `json:"de"`
}

// ReadAndParseRequest fills 'model', 'original' and 'filtered' with data from body
func ReadAndParseRequest(body io.ReadCloser, model MultilangModel) (original, filtered map[string]interface{}, apiError *APIError) {

	// create maps
	original = make(map[string]interface{})

	err := json.NewDecoder(body).Decode(&original)
	if err != nil {
		apiError = NewRequestDecodingError(err)
		return
	}

	// remove unknow fields from map
	filtered = FilterUnknownFields(model, original)

	// convert multilang fields to jsonb
	ConvertRequestMapToJSONB(&filtered, model)

	jsonBytes, err := json.Marshal(filtered)
	if err != nil {
		apiError = NewJSONEncodingError(MessageRequestJSONDecoding, err)
		return
	}

	// decode offering object from request body
	err = json.Unmarshal(jsonBytes, model)
	if err != nil {
		apiError = NewRequestDecodingError(err)
		return
	}
	return
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

// PrepareResponseForMultilangModel converts model to map with all multilang fields as jsonb
func PrepareResponseForMultilangModel(model MultilangModel) (map[string]interface{}, *APIError) {

	modelMap := make(map[string]interface{})
	// marshal to json
	modelBytes, err := json.Marshal(model)
	if err != nil {
		return modelMap, NewJSONEncodingError(MessageResponseJSONEncoding, err)
	}

	// fill map
	err = json.Unmarshal(modelBytes, &modelMap)
	if err != nil {
		return modelMap, NewJSONDecodingError(MessageResponseJSONEncoding, err)
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
				return modelMap, NewJSONDecodingError(MessageResponseJSONEncoding, err)
			}

			if err := json.Unmarshal(valBytes, &mString); err != nil {
				return modelMap, NewJSONDecodingError(MessageResponseJSONEncoding, err)
			}
		}

		modelMap[name+"_map"] = mString
		modelMap[name] = mString.En
	}

	return modelMap, nil
}

// ConvertRequestMapToJSONB replaces multilang string to jsonb if needed
func ConvertRequestMapToJSONB(modelMap *map[string]interface{}, model MultilangModel) *APIError {

	localMap := *modelMap

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
				return NewJSONEncodingError(MessageRequestJSONDecoding, err)
			}
			metadata := json.RawMessage(mapB)
			localMap[name] = postgres.Jsonb{RawMessage: metadata}
		}
	}
	return nil
}

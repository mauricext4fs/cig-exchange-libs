package twilio

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
)

// Twilio api urls
const (
	verificationStartURL = "https://api.authy.com/protected/json/phones/verification/start"
	verificationCheckURL = "https://api.authy.com/protected/json/phones/verification/check"
)

// OTP struct for Twilio "Verify" application https://www.twilio.com/console/verify/applications
type OTP struct {
	APIKey string
}

const missingAPIKeyError = "Need to set Twilio api key"

// twilioResponse struct for parsing twilio response
type twilioResponse struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

// NewOTP initialize a new OTP struct with given Api Key
func NewOTP(apiKey string) *OTP {
	return &OTP{APIKey: apiKey}
}

// ReceiveOTP sends request to receive OTP for phone number
func (twilioOTP *OTP) ReceiveOTP(countryCode, phoneNumber string) (message string, err error) {

	// check api key
	if len(twilioOTP.APIKey) == 0 {
		return missingAPIKeyError, errors.New(missingAPIKeyError)
	}

	// fill request parameters
	vals := url.Values{
		"api_key":      {twilioOTP.APIKey},
		"via":          {"sms"},
		"phone_number": {phoneNumber},
		"country_code": {countryCode},
	}
	resp, err := http.PostForm(verificationStartURL, vals)
	if err != nil {
		return "Can't execute request", err
	}

	defer resp.Body.Close()

	return twilioOTP.parseTwilioResponse(resp.Body)
}

// VerifyOTP verifies OTP for phone number
func (twilioOTP *OTP) VerifyOTP(otp, countryCode, phoneNumber string) (message string, err error) {

	// check api key
	if len(twilioOTP.APIKey) == 0 {
		return missingAPIKeyError, errors.New(missingAPIKeyError)
	}

	client := &http.Client{}

	req, err := http.NewRequest("GET", verificationCheckURL, nil)
	if err != nil {
		return "Can't create new request", err
	}

	// fill request parameters
	q := req.URL.Query()
	q.Add("api_key", twilioOTP.APIKey)
	q.Add("verification_code", otp)
	q.Add("phone_number", phoneNumber)
	q.Add("country_code", countryCode)
	req.URL.RawQuery = q.Encode()

	resp, err := client.Do(req)
	if err != nil {
		return "Can't execute request", err
	}

	defer resp.Body.Close()

	return twilioOTP.parseTwilioResponse(resp.Body)
}

func (twilioOTP *OTP) parseTwilioResponse(rBody io.ReadCloser) (message string, err error) {

	// read response
	body, err := ioutil.ReadAll(rBody)

	if err != nil {
		return "Can't read response body", err
	}

	// parse response to twilioResponse struct
	var response twilioResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return "Can't unmarshal response", err
	}

	if !response.Success {
		err = errors.New(response.Message)
	}
	return response.Message, err
}

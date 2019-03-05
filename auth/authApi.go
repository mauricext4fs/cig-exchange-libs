package auth

import (
	"cig-exchange-libs"
	"cig-exchange-libs/models"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/mattbaird/gochimp"
	uuid "github.com/satori/go.uuid"
)

// Constants defining the active platform
const (
	PlatformP2P     = "p2p"
	PlatformTrading = "trading"
)

// Expiration time is one month
const tokenExpirationTimeInMin = 60 * 24 * 31

type userResponse struct {
	UUID string `json:"uuid"`
}

func (resp *userResponse) randomUUID() {
	UUID, err := uuid.NewV4()
	if err != nil {
		// uuid for an unlikely event of NewV4 failure
		resp.UUID = "fdb283d4-7341-4517-b501-371d22d27cfc"
		return
	}
	resp.UUID = UUID.String()
}

type verificationCodeRequest struct {
	UUID string `json:"uuid"`
	Type string `json:"type"`
	Code string `json:"code"`
}

// UserRequest is a structure to represent the signup api request
type userRequest struct {
	Sex              string `json:"sex"`
	Name             string `json:"name"`
	LastName         string `json:"lastname"`
	Email            string `json:"email"`
	PhoneCountryCode string `json:"phone_country_code"`
	PhoneNumber      string `json:"phone_number"`
	ReferenceKey     string `json:"reference_key"`
	Platform         string `json:"platform"`
}

func (user *userRequest) convertRequestToUser() *models.User {
	mUser := &models.User{}

	mUser.Sex = user.Sex
	mUser.Role = "Platform"
	mUser.Name = user.Name
	mUser.LastName = user.LastName

	mUser.LoginEmail = &models.Contact{Type: "email", Level: "primary", Value1: user.Email}
	mUser.LoginPhone = &models.Contact{Type: "phone", Level: "secondary", Value1: user.PhoneCountryCode, Value2: user.PhoneNumber}

	return mUser
}

// UserAPI stores site based variables
type UserAPI struct {
	Platform string
	BaseURI  string
	SkipJWT  []string
}

// NewUserAPI creates UserApi instance
func NewUserAPI(platform, baseURI string, skipJWT []string) *UserAPI {
	return &UserAPI{Platform: platform, BaseURI: baseURI, SkipJWT: skipJWT}
}

// LoggedInUser is passed to controllers after jwt auth
type LoggedInUser struct {
	UserUUID         string
	OrganisationUUID string
	CreationDate     time.Time
	ExpirationDate   time.Time
}

type token struct {
	UserUUID         string
	OrganisationUUID string
	jwt.StandardClaims
}

type key int

const (
	keyJWT key = iota
)

// GetContextValues extracts the userID and organisationID from the request context
// Should be used by JWT enabled API calls
func GetContextValues(r *http.Request) (loggedInUser *LoggedInUser, err error) {
	// extract the entire token struct
	tk, ok := r.Context().Value(keyJWT).(*token)
	if !ok {
		fmt.Println("GetContextValues: no context value exists")
		err = fmt.Errorf("Invalid access token")
		return
	}

	loggedInUser = &LoggedInUser{}
	loggedInUser.UserUUID = tk.UserUUID
	loggedInUser.OrganisationUUID = tk.OrganisationUUID
	issued := time.Unix(tk.IssuedAt, 0)
	expires := time.Unix(tk.ExpiresAt, 0)
	if issued.IsZero() || expires.IsZero() {
		fmt.Println("GetContextValues: broken context value")
		err = fmt.Errorf("Invalid access token")
		return
	}

	loggedInUser.CreationDate = issued
	loggedInUser.ExpirationDate = expires

	return
}

// JwtAuthenticationHandler handles auth for endpoints
func (userAPI *UserAPI) JwtAuthenticationHandler(next http.Handler) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// current request path
		requestPath := r.URL.Path

		// check if request does not need authentication, serve the request if it doesn't need it
		for _, value := range userAPI.SkipJWT {

			if requestPath == userAPI.BaseURI+value {
				next.ServeHTTP(w, r)
				return
			}
		}

		response := make(map[string]interface{})
		tokenHeader := r.Header.Get("Authorization") // Grab the token from the header

		if tokenHeader == "" { // Token is missing, returns with error code 403 Unauthorized
			response = cigExchange.Message(false, "Missing auth token")
			w.WriteHeader(http.StatusForbidden)
			w.Header().Add("Content-Type", "application/json")
			cigExchange.Respond(w, response)
			return
		}

		// The token normally comes in format `Bearer {token-body}`, we check if the retrieved token matched this requirement
		splitted := strings.Split(tokenHeader, " ")
		if len(splitted) != 2 {
			response = cigExchange.Message(false, "Invalid/Malformed auth token")
			w.WriteHeader(http.StatusForbidden)
			w.Header().Add("Content-Type", "application/json")
			cigExchange.Respond(w, response)
			return
		}

		tokenPart := splitted[1] // Grab the token part, what we are truly interested in
		tk := &token{}

		token, err := jwt.ParseWithClaims(tokenPart, tk, func(token *jwt.Token) (interface{}, error) {
			return []byte(os.Getenv("TOKEN_PASSWORD")), nil
		})

		if err != nil { // Malformed token, returns with http code 403 as usual
			response = cigExchange.Message(false, "Malformed authentication token")
			w.WriteHeader(http.StatusForbidden)
			w.Header().Add("Content-Type", "application/json")
			cigExchange.Respond(w, response)
			return
		}

		if !token.Valid { // Token is invalid, maybe not signed on this server
			response = cigExchange.Message(false, "Token is not valid.")
			w.WriteHeader(http.StatusForbidden)
			w.Header().Add("Content-Type", "application/json")
			cigExchange.Respond(w, response)
			return
		}

		// Everything went well, proceed with the request and set the caller to the user retrieved from the parsed token
		ctx := context.WithValue(r.Context(), keyJWT, tk)

		r = r.WithContext(ctx)
		// proceed in the middleware chain!
		next.ServeHTTP(w, r)
	})
}

// CreateUserHandler handles POST api/users/signup endpoint
func (userAPI *UserAPI) CreateUserHandler(w http.ResponseWriter, r *http.Request) {

	resp := &userResponse{}
	resp.randomUUID()

	userReq := &userRequest{}

	// decode user object from request body
	err := json.NewDecoder(r.Body).Decode(userReq)
	if err != nil {
		apiError := cigExchange.NewJSONDecodingError(err)
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	// check that we received 'platform' parameter
	if len(userReq.Platform) == 0 {
		apiError := cigExchange.NewRequiredFieldError([]string{"platform"})
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	// user must use p2p or trading platform
	if userReq.Platform != PlatformP2P && userReq.Platform != PlatformTrading {
		apiError := cigExchange.NewInvalidFieldError("platform", "Invalid platform parameter")
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	user := userReq.convertRequestToUser()

	// P2P users are required to have an organisation reference key
	if userReq.Platform == PlatformP2P && len(userReq.ReferenceKey) == 0 {
		apiError := cigExchange.NewRequiredFieldError([]string{"reference_key"})
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	// try to create user
	apiError := user.Create(userReq.ReferenceKey)
	if apiError != nil {
		fmt.Println(apiError.ToString())
		if apiError.ShouldSilenceError() {
			cigExchange.Respond(w, resp)
		} else {
			cigExchange.RespondWithAPIError(w, apiError)
		}
		return
	}

	// send welcome email async
	go func() {
		err = sendEmail(emailTypeWelcome, userReq.Email, "")
		if err != nil {
			fmt.Println("CreateUser: email sending error:")
			fmt.Println(err.Error())
		}
	}()

	resp.UUID = user.ID
	cigExchange.Respond(w, resp)
}

// GetUserHandler handles POST api/users/signin endpoint
func (userAPI *UserAPI) GetUserHandler(w http.ResponseWriter, r *http.Request) {

	resp := &userResponse{}
	resp.randomUUID()

	userReq := &userRequest{}
	// decode user object from request body
	err := json.NewDecoder(r.Body).Decode(userReq)
	if err != nil {
		apiError := cigExchange.NewJSONDecodingError(err)
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	user := &models.User{}
	var apiError *cigExchange.APIError
	// login using email or phone number
	if len(userReq.Email) > 0 {
		user, apiError = models.GetUserByEmail(userReq.Email)
	} else if len(userReq.PhoneCountryCode) > 0 && len(userReq.PhoneNumber) > 0 {
		user, apiError = models.GetUserByMobile(userReq.PhoneCountryCode, userReq.PhoneNumber)
	} else {
		// neither email or phone specified
		apiError = cigExchange.NewRequiredFieldError([]string{"email", "phone_number", "phone_country_code"})
	}

	if apiError != nil {
		fmt.Println(apiError.ToString())
		if apiError.ShouldSilenceError() {
			cigExchange.Respond(w, resp)
		} else {
			cigExchange.RespondWithAPIError(w, apiError)
		}
		return
	}

	resp.UUID = user.ID
	cigExchange.Respond(w, resp)
}

// SendCodeHandler handles POST api/users/send_otp endpoint
func (userAPI *UserAPI) SendCodeHandler(w http.ResponseWriter, r *http.Request) {

	reqStruct := &verificationCodeRequest{}
	// decode verificationCodeRequest object from request body
	err := json.NewDecoder(r.Body).Decode(reqStruct)
	if err != nil {
		apiError := cigExchange.NewJSONDecodingError(err)
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	// respond with 204
	w.WriteHeader(204)

	// process the send OTP async so that client won't see any email sending delays
	go func() {
		user, apiError := models.GetUser(reqStruct.UUID)
		if apiError != nil {
			fmt.Println(apiError.ToString())
			return
		}

		// send code to email or phone number
		if reqStruct.Type == "phone" {
			if user.LoginPhone == nil {
				fmt.Println("SendCode: User doesn't have phone contact")
				return
			}
			twilioClient := cigExchange.GetTwilio()
			_, err = twilioClient.ReceiveOTP(user.LoginPhone.Value1, user.LoginPhone.Value2)
			if err != nil {
				fmt.Println("SendCode: twillio error:")
				fmt.Println(err.Error())
			}
		} else if reqStruct.Type == "email" {
			if user.LoginEmail == nil {
				fmt.Println("SendCode: User doesn't have email contact")
				return
			}
			rediskey := cigExchange.GenerateRedisKey(reqStruct.UUID)
			expiration := 5 * time.Minute

			code := cigExchange.RandCode(6)
			err = cigExchange.GetRedis().Set(rediskey, code, expiration).Err()
			if err != nil {
				fmt.Println("SendCode: redis error:")
				fmt.Println(err.Error())
				return
			}
			err = sendEmail(emailTypePinCode, user.LoginEmail.Value1, code)
			if err != nil {
				fmt.Println("SendCode: email sending error:")
				fmt.Println(err.Error())
				return
			}
		} else {
			fmt.Println("SendCode: Error: unsupported otp type")
		}
	}()
}

// VerifyCodeHandler handles POST api/users/verify_otp endpoint
func (userAPI *UserAPI) VerifyCodeHandler(w http.ResponseWriter, r *http.Request) {

	// prepare the default response to send (unauthorized / invalid code)
	secureErrorResponse := &cigExchange.APIError{}
	secureErrorResponse.SetErrorType(cigExchange.ErrorTypeUnauthorized)
	secureErrorResponse.NewNestedError(cigExchange.NestedErrorFieldInvalid, "Invalid code")

	reqStruct := &verificationCodeRequest{}
	// decode verificationCodeRequest object from request body
	err := json.NewDecoder(r.Body).Decode(reqStruct)
	if err != nil {
		apiError := cigExchange.NewJSONDecodingError(err)
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	user, apiError := models.GetUser(reqStruct.UUID)
	if err != nil {
		fmt.Println(apiError.ToString())
		if apiError.ShouldSilenceError() {
			cigExchange.RespondWithAPIError(w, secureErrorResponse)
		} else {
			cigExchange.RespondWithAPIError(w, apiError)
		}
		return
	}

	// get organisation UUID related to user
	organisationUser := &models.OrganisationUser{}
	db := cigExchange.GetDB().Model(user).Related(organisationUser, "UserID")
	if db.Error != nil {
		// organization can be missed
		if !db.RecordNotFound() {
			apiError = cigExchange.NewGormError("Organization user links lookup failed", db.Error)
			fmt.Printf(apiError.ToString())
			cigExchange.RespondWithAPIError(w, apiError)
			return
		}
	}

	// verify code
	if reqStruct.Type == "phone" {
		if user.LoginPhone == nil {
			apiError = cigExchange.NewInvalidFieldError("type", "User doesn't have phone contact")
			fmt.Printf(apiError.ToString())
			cigExchange.RespondWithAPIError(w, apiError)
			return
		}
		twilioClient := cigExchange.GetTwilio()
		_, err := twilioClient.VerifyOTP(reqStruct.Code, user.LoginPhone.Value1, user.LoginPhone.Value2)
		if err != nil {
			apiError = cigExchange.NewTwilioError("Verify OTP", err)
			fmt.Printf(apiError.ToString())
			cigExchange.RespondWithAPIError(w, apiError)
			return
		}

	} else if reqStruct.Type == "email" {
		if user.LoginEmail == nil {
			apiError = cigExchange.NewInvalidFieldError("type", "User doesn't have email contact")
			fmt.Printf(apiError.ToString())
			cigExchange.RespondWithAPIError(w, apiError)
			return
		}
		rediskey := cigExchange.GenerateRedisKey(reqStruct.UUID)

		redisCmd := cigExchange.GetRedis().Get(rediskey)
		if redisCmd.Err() != nil {
			apiError = cigExchange.NewRedisError("Get code failure", redisCmd.Err())
			fmt.Printf(apiError.ToString())
			cigExchange.RespondWithAPIError(w, apiError)
			return
		}
		if redisCmd.Val() != reqStruct.Code {
			fmt.Println("VerifyCode: code mismatch, expecting " + redisCmd.Val())
			cigExchange.RespondWithAPIError(w, secureErrorResponse)
			return
		}
	} else {
		apiError = cigExchange.NewRequiredFieldError([]string{"email", "phone_number", "phone_country_code"})
		fmt.Printf(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	// user is verified
	user.Verified = 1
	apiError = user.Save()
	if err != nil {
		fmt.Printf(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	// verification passed, generate jwt and return it
	tk := &token{
		user.ID,
		organisationUser.OrganisationID,
		jwt.StandardClaims{
			IssuedAt:  time.Now().Unix(),
			ExpiresAt: time.Now().Add(time.Minute * tokenExpirationTimeInMin).Unix(),
		},
	}
	token := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), tk)
	tokenString, err := token.SignedString([]byte(os.Getenv("TOKEN_PASSWORD")))
	if err != nil {
		fmt.Println("VerifyCode: jwt generation failed:")
		fmt.Println(err.Error())
		cigExchange.RespondWithAPIError(w, secureErrorResponse)
		return
	}

	type verifyCodeResponse struct {
		JWT string `json:"jwt"`
	}

	resp := &verifyCodeResponse{JWT: tokenString}
	cigExchange.Respond(w, resp)
}

type emailType int

const (
	emailTypeWelcome emailType = iota
	emailTypePinCode
)

func sendEmail(eType emailType, email, pinCode string) error {

	mandrillClient := cigExchange.GetMandrill()

	templateName := ""
	mergeVars := make([]gochimp.Var, 0)

	switch eType {
	case emailTypeWelcome:
		templateName = "welcome"
	case emailTypePinCode:
		templateName = "pin-code"
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
		Subject:   "Welcome aboard!",
		FromEmail: "noreply@cig-exchange.ch",
		FromName:  "CIG Exchange",
		To:        recipients,
	}

	_, err := mandrillClient.MessageSend(message, false)
	return err
}

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

type userResponse struct {
	UUID string `json:"uuid"`
}

type verifyCodeResponse struct {
	JWT string `json:"jwt"`
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

func (resp *userResponse) randomUUID() {
	UUID, err := uuid.NewV4()
	if err != nil {
		// uuid for an unlikely event of NewV4 failure
		resp.UUID = "fdb283d4-7341-4517-b501-371d22d27cfc"
		return
	}
	resp.UUID = UUID.String()
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

type token struct {
	UserUUID         string
	OrganisationUUID string
	jwt.StandardClaims
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
		ctx := context.WithValue(r.Context(), "user", tk.UserUUID)

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
		fmt.Println("CreateUser: body JSON decoding error:")
		fmt.Println(err.Error())
		cigExchange.Respond(w, resp)
		return
	}

	// user must use p2p or trading platform
	if userReq.Platform != PlatformP2P && userReq.Platform != PlatformTrading {
		fmt.Println("CreateUser: users are required to have a p2p or trading platform")
		cigExchange.Respond(w, resp)
		return
	}

	user := userReq.convertRequestToUser()

	// P2P users are required to have an organisation reference key
	if userReq.Platform == PlatformP2P && len(userReq.ReferenceKey) == 0 {
		fmt.Println("CreateUser: P2P users are required to have a reference key")
		cigExchange.Respond(w, resp)
		return
	}

	// try to create user
	err = user.Create(userReq.ReferenceKey)
	if err != nil {
		fmt.Println("CreateUser: db Create error:")
		fmt.Println(err.Error())
		cigExchange.Respond(w, resp)
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

// GetUserHandler handles GET api/users/signin endpoint
func (userAPI *UserAPI) GetUserHandler(w http.ResponseWriter, r *http.Request) {

	resp := &userResponse{}
	resp.randomUUID()

	userReq := &userRequest{}
	// decode user object from request body
	err := json.NewDecoder(r.Body).Decode(userReq)
	if err != nil {
		fmt.Println("GetUser: body JSON decoding error:")
		fmt.Println(err.Error())
		cigExchange.Respond(w, resp)
		return
	}

	user := &models.User{}
	// login using email or phone number
	if len(userReq.Email) > 0 {
		user, err = models.GetUserByEmail(userReq.Email)
	} else if len(userReq.PhoneCountryCode) > 0 && len(userReq.PhoneNumber) > 0 {
		user, err = models.GetUserByMobile(userReq.PhoneCountryCode, userReq.PhoneNumber)
	} else {
		fmt.Println("GetUser: neither email or mobile number specified in post body")
		cigExchange.Respond(w, resp)
		return
	}

	if err != nil {
		fmt.Println("GetUser: db Lookup error:")
		fmt.Println(err.Error())
		cigExchange.Respond(w, resp)
		return
	}

	resp.UUID = user.ID
	cigExchange.Respond(w, resp)
}

// SendCodeHandler handles POST api/users/send_otp endpoint
func (userAPI *UserAPI) SendCodeHandler(w http.ResponseWriter, r *http.Request) {

	w.WriteHeader(204)

	reqStruct := &verificationCodeRequest{}
	// decode verificationCodeRequest object from request body
	err := json.NewDecoder(r.Body).Decode(reqStruct)
	if err != nil {
		fmt.Println("SendCode: body JSON decoding error:")
		fmt.Println(err.Error())
		return
	}

	// process the send OTP async so that client won't see any email sending delays
	go func() {
		user, err := models.GetUser(reqStruct.UUID)
		if err != nil {
			fmt.Println("SendCode: db Lookup error:")
			fmt.Println(err.Error())
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

// VerifyCodeHandler handles GET api/users/verify_otp endpoint
func (userAPI *UserAPI) VerifyCodeHandler(w http.ResponseWriter, r *http.Request) {

	retErr := fmt.Errorf("Invalid code")
	retCode := 401

	reqStruct := &verificationCodeRequest{}
	// decode verificationCodeRequest object from request body
	err := json.NewDecoder(r.Body).Decode(reqStruct)
	if err != nil {
		fmt.Println("VerifyCode: body JSON decoding error:")
		fmt.Println(err.Error())
		cigExchange.RespondWithError(w, retCode, retErr)
		return
	}

	user, err := models.GetUser(reqStruct.UUID)
	if err != nil {
		fmt.Println("VerifyCode: db Lookup error:")
		fmt.Println(err.Error())
		cigExchange.RespondWithError(w, retCode, retErr)
		return
	}

	// get organisation UUID related to user
	organisationUser := &models.OrganisationUser{}
	db := cigExchange.GetDB().Model(user).Related(organisationUser, "UserID")
	if db.Error != nil {
		// organization can be missed
		if !db.RecordNotFound() {
			fmt.Printf("VerifyCode: Database error:")
			fmt.Println(db.Error.Error())
			cigExchange.RespondWithError(w, retCode, retErr)
			return
		}
	}

	// verify code
	if reqStruct.Type == "phone" {
		if user.LoginPhone == nil {
			fmt.Println("VerifyCode: User doesn't have phone contact")
			cigExchange.RespondWithError(w, retCode, retErr)
			return
		}
		twilioClient := cigExchange.GetTwilio()
		_, err := twilioClient.VerifyOTP(reqStruct.Code, user.LoginPhone.Value1, user.LoginPhone.Value2)
		if err != nil {
			fmt.Println("VerifyCode: twillio error:")
			fmt.Println(err.Error())
			cigExchange.RespondWithError(w, retCode, retErr)
			return
		}

	} else if reqStruct.Type == "email" {
		if user.LoginEmail == nil {
			fmt.Println("VerifyCode: User doesn't have email contact")
			cigExchange.RespondWithError(w, retCode, retErr)
			return
		}
		rediskey := cigExchange.GenerateRedisKey(reqStruct.UUID)

		redisCmd := cigExchange.GetRedis().Get(rediskey)
		if redisCmd.Err() != nil {
			fmt.Println("VerifyCode: redis error:")
			fmt.Println(err.Error())
			cigExchange.RespondWithError(w, retCode, retErr)
			return
		}
		if redisCmd.Val() != reqStruct.Code {
			fmt.Println("VerifyCode: code mismatch, expecting " + redisCmd.Val())
			cigExchange.RespondWithError(w, retCode, retErr)
			return
		}
	} else {
		fmt.Println("VerifyCode: Error: unsupported otp type")
		cigExchange.RespondWithError(w, retCode, retErr)
		return
	}

	// user is verified
	user.Verified = 1
	err = user.Save()
	if err != nil {
		fmt.Println("VerifyCode: db Save error:")
		fmt.Println(err.Error())
		cigExchange.RespondWithError(w, retCode, retErr)
		return
	}

	// verification passed, generate jwt and return it
	tk := &token{
		UserUUID:         user.ID,
		OrganisationUUID: organisationUser.OrganisationID,
	}
	token := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), tk)
	tokenString, err := token.SignedString([]byte(os.Getenv("TOKEN_PASSWORD")))
	if err != nil {
		fmt.Println("VerifyCode: jwt generation failed:")
		fmt.Println(err.Error())
		cigExchange.RespondWithError(w, retCode, retErr)
		return
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

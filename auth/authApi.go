package auth

import (
	cigExchange "cig-exchange-libs"
	"cig-exchange-libs/models"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
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

type jwtResponse struct {
	JWT string `json:"jwt"`
}

// UserRequest is a structure to represent the signup api request
type UserRequest struct {
	Sex              string `json:"sex"`
	Name             string `json:"name"`
	LastName         string `json:"lastname"`
	Email            string `json:"email"`
	PhoneCountryCode string `json:"phone_country_code"`
	PhoneNumber      string `json:"phone_number"`
	ReferenceKey     string `json:"reference_key"`
	Platform         string `json:"platform"`
}

// ConvertRequestToUser convert UserRequest struct to User
func (user *UserRequest) ConvertRequestToUser() *models.User {
	mUser := &models.User{}

	mUser.Sex = user.Sex
	mUser.Role = "Platform"
	mUser.Name = user.Name
	mUser.LastName = user.LastName

	mUser.LoginEmail = &models.Contact{Type: "email", Level: "primary", Value1: user.Email}
	mUser.LoginPhone = &models.Contact{Type: "phone", Level: "secondary", Value1: user.PhoneCountryCode, Value2: user.PhoneNumber}

	return mUser
}

type organisationRequest struct {
	Sex              string `json:"sex"`
	Name             string `json:"name"`
	LastName         string `json:"lastname"`
	Email            string `json:"email"`
	PhoneCountryCode string `json:"phone_country_code"`
	PhoneNumber      string `json:"phone_number"`
	ReferenceKey     string `json:"reference_key"`
	OrganisationName string `json:"organisation_name"`
}

func (request *organisationRequest) convertRequestToUserAndOrganisation() (*models.User, *models.Organisation) {
	mUser := &models.User{}

	mUser.Sex = request.Sex
	mUser.Role = "Platform"
	mUser.Name = request.Name
	mUser.LastName = request.LastName

	mUser.LoginEmail = &models.Contact{Type: "email", Level: "primary", Value1: request.Email}
	mUser.LoginPhone = &models.Contact{Type: "phone", Level: "secondary", Value1: request.PhoneCountryCode, Value2: request.PhoneNumber}

	mOrganisation := &models.Organisation{}
	mOrganisation.ReferenceKey = request.ReferenceKey
	mOrganisation.Name = request.OrganisationName

	return mUser, mOrganisation
}

// UserAPI handles JWT auth and user management api calls
type UserAPI struct {
	SkipPrefix string
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
		if strings.HasPrefix(requestPath, userAPI.SkipPrefix) {
			next.ServeHTTP(w, r)
			return
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

	userReq := &UserRequest{}

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

	user := userReq.ConvertRequestToUser()

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
		err = cigExchange.SendEmail(cigExchange.EmailTypeWelcome, userReq.Email, "")
		if err != nil {
			fmt.Println("CreateUser: email sending error:")
			fmt.Println(err.Error())
		}
	}()

	resp.UUID = user.ID
	cigExchange.Respond(w, resp)
}

// CreateOrganisationHandler handles POST api/organisations/signup endpoint
func (userAPI *UserAPI) CreateOrganisationHandler(w http.ResponseWriter, r *http.Request) {

	orgRequest := &organisationRequest{}
	// decode organisation request object from request body
	err := json.NewDecoder(r.Body).Decode(orgRequest)
	if err != nil {
		apiError := cigExchange.NewJSONDecodingError(err)
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	// convert request to User and Organisation structs
	user, organisation := orgRequest.convertRequestToUserAndOrganisation()

	// prepare silence error response
	resp := &userResponse{}
	resp.randomUUID()

	if len(organisation.ReferenceKey) == 0 {
		apiError := cigExchange.NewInvalidFieldError("reference_key", "Organisation reference key is invalid")
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	// check that organisation doesn't exist
	orgWhere := &models.Organisation{
		ReferenceKey: organisation.ReferenceKey,
	}
	org := &models.Organisation{}
	db := cigExchange.GetDB().Where(orgWhere).First(org)
	if db.Error != nil {
		// handle database error
		if !db.RecordNotFound() {
			apiError := cigExchange.NewDatabaseError("Organization lookup failed", db.Error)
			fmt.Println(apiError.ToString())
			cigExchange.RespondWithAPIError(w, apiError)
			return
		}
		// organisation doen't exist
	} else {
		// handle wrong reference key
		apiError := cigExchange.NewInvalidFieldError("reference_key", "Organisation with reference key already exist")
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	// try to create user without reference key
	apiError := user.Create("")
	if apiError != nil {
		fmt.Println(apiError.ToString())
		if apiError.ShouldSilenceError() {
			cigExchange.Respond(w, resp)
		} else {
			cigExchange.RespondWithAPIError(w, apiError)
		}
		return
	}

	// insert organisation into db
	apiError = organisation.Create()
	if apiError != nil {
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	orgUser := &models.OrganisationUser{
		UserID:           user.ID,
		OrganisationID:   organisation.ID,
		OrganisationRole: "admin",
		IsHome:           true,
	}

	// insert organisation user into db
	apiError = orgUser.Create()
	if apiError != nil {
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	// send welcome email async
	go func() {
		err = cigExchange.SendEmail(cigExchange.EmailTypeWelcome, orgRequest.Email, "")
		if err != nil {
			fmt.Println("CreateOrganisation: email sending error:")
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

	userReq := &UserRequest{}
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

	user, apiError := models.GetUser(reqStruct.UUID)
	if apiError != nil {
		fmt.Println(apiError.ToString())
		if apiError.ShouldSilenceError() {
			// respond with 204
			w.WriteHeader(204)
		} else {
			cigExchange.RespondWithAPIError(w, apiError)
		}
		return
	}

	// check that we received 'type' parameter
	if len(reqStruct.Type) == 0 {
		apiError := cigExchange.NewRequiredFieldError([]string{"type"})
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	// send code to email or phone number
	if reqStruct.Type == "phone" {
		if user.LoginPhone == nil {
			apiError = cigExchange.NewInvalidFieldError("type", "User doesn't have phone contact")
			fmt.Printf(apiError.ToString())
			cigExchange.RespondWithAPIError(w, apiError)
			return
		}
		// process the send OTP async so that client won't see any delays
		go func() {
			twilioClient := cigExchange.GetTwilio()
			_, err = twilioClient.ReceiveOTP(user.LoginPhone.Value1, user.LoginPhone.Value2)
			if err != nil {
				fmt.Println("SendCode: twillio error:")
				fmt.Println(err.Error())
			}
		}()
	} else if reqStruct.Type == "email" {
		if user.LoginEmail == nil {
			apiError = cigExchange.NewInvalidFieldError("type", "User doesn't have email")
			fmt.Printf(apiError.ToString())
			cigExchange.RespondWithAPIError(w, apiError)
			return
		}
		rediskey := cigExchange.GenerateRedisKey(reqStruct.UUID)
		expiration := 5 * time.Minute

		code := cigExchange.RandCode(6)
		redisCmd := cigExchange.GetRedis().Set(rediskey, code, expiration)
		if redisCmd.Err() != nil {
			apiError = cigExchange.NewRedisError("Set code failure", redisCmd.Err())
			fmt.Printf(apiError.ToString())
			cigExchange.RespondWithAPIError(w, apiError)
			return
		}
		// process the send OTP async so that client won't see any delays
		go func() {
			err = cigExchange.SendEmail(cigExchange.EmailTypePinCode, user.LoginEmail.Value1, code)
			if err != nil {
				fmt.Println("SendCode: email sending error:")
				fmt.Println(err.Error())
				return
			}
		}()

		// in "DEV" environment we return the email signup code for testing purposes
		if cigExchange.IsDevEnv() {
			resp := make(map[string]string, 0)
			resp["code"] = code
			cigExchange.Respond(w, resp)
			return
		}
	} else {
		apiError = cigExchange.NewInvalidFieldError("type", "Invalid otp type")
		fmt.Printf(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}
	w.WriteHeader(204)
}

// VerifyCodeHandler handles POST api/users/verify_otp endpoint
func (userAPI *UserAPI) VerifyCodeHandler(w http.ResponseWriter, r *http.Request) {

	// prepare the default response to send (unauthorized / invalid code)
	secureErrorResponse := &cigExchange.APIError{}
	secureErrorResponse.SetErrorType(cigExchange.ErrorTypeUnauthorized)
	secureErrorResponse.NewNestedError(cigExchange.ReasonFieldInvalid, "Invalid code")

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

	// get OrganisationUsers related to user
	organisationUser := &models.OrganisationUser{}
	orgUsers := make([]*models.OrganisationUser, 0)
	db := cigExchange.GetDB().Model(user).Related(&orgUsers, "UserID")
	if db.Error != nil {
		// organization can be missed
		if !db.RecordNotFound() {
			apiError = cigExchange.NewDatabaseError("Organization user links lookup failed", db.Error)
			fmt.Printf(apiError.ToString())
			cigExchange.RespondWithAPIError(w, apiError)
			return
		}
	}

	// choose home organisation
	if len(orgUsers) > 0 {
		// set default home organisation
		organisationUser = orgUsers[0]

		// look for home organisation
		for _, orgUser := range orgUsers {
			if orgUser.Status != models.OrganisationUserStatusActive {
				orgUser.Status = models.OrganisationUserStatusActive
				orgUser.Update()
			}

			if orgUser.IsHome {
				organisationUser = orgUser
				break
			}
		}

		// apply home organisation
		apiError = models.SetHomeOrganisation(organisationUser)
		if apiError != nil {
			fmt.Println(apiError.ToString())
			cigExchange.RespondWithAPIError(w, apiError)
			return
		}
	}

	// check that we received 'type' parameter
	if len(reqStruct.Type) == 0 {
		apiError := cigExchange.NewRequiredFieldError([]string{"type"})
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
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
		apiError = cigExchange.NewInvalidFieldError("type", "Invalid otp type")
		fmt.Printf(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	// user is verified
	user.Status = models.UserStatusVerified
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

	resp := &jwtResponse{JWT: tokenString}
	cigExchange.Respond(w, resp)
}

// ChangeOrganisationHandler handles POST api/users/switch/{organisation_id} endpoint
func (userAPI *UserAPI) ChangeOrganisationHandler(w http.ResponseWriter, r *http.Request) {

	organisationID := mux.Vars(r)["organisation_id"]

	// load context user info
	loggedInUser, err := GetContextValues(r)
	if err != nil {
		apiError := cigExchange.NewRoutingError(err)
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	// find organisation user
	searchOrgUser := &models.OrganisationUser{
		OrganisationID: organisationID,
		UserID:         loggedInUser.UserUUID,
	}

	orgUser, apiError := searchOrgUser.Find()
	if apiError != nil {
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	// check that user belong to organisation
	if orgUser.UserID != loggedInUser.UserUUID {
		apiError = cigExchange.NewInvalidFieldError("organisation_id", "User don't belong to organisation")
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	// select new home organisation
	apiError = models.SetHomeOrganisation(orgUser)
	if apiError != nil {
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	// verification passed, generate jwt and return it
	tk := &token{
		loggedInUser.UserUUID,
		organisationID,
		jwt.StandardClaims{
			IssuedAt:  time.Now().Unix(),
			ExpiresAt: time.Now().Add(time.Minute * tokenExpirationTimeInMin).Unix(),
		},
	}
	token := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), tk)
	tokenString, err := token.SignedString([]byte(os.Getenv("TOKEN_PASSWORD")))
	if err != nil {
		apiError := cigExchange.NewTokenError("Token generation failed", err)
		fmt.Println(apiError.ToString())
		cigExchange.RespondWithAPIError(w, apiError)
		return
	}

	resp := &jwtResponse{JWT: tokenString}
	cigExchange.Respond(w, resp)
}

package auth

import (
	"bytes"
	cigExchange "cig-exchange-libs"
	"cig-exchange-libs/models"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/duo-labs/webauthn/protocol"
	"github.com/duo-labs/webauthn/webauthn"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm/dialects/postgres"
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

type verificationCodeRequest struct {
	UUID string `json:"uuid"`
	Type string `json:"type"`
	Code string `json:"code"`
}

// Constants for JwtResponse status
const (
	JWTResponseStatusFinished = "finished"
	JWTResponseStatusWebAuthn = "web authn needed"
)

// JwtResponse structure
type JwtResponse struct {
	JWT    string `json:"jwt"`
	Status string `json:"status"`
}

type infoResponse struct {
	UserUUID         string `json:"user_id"`
	Role             string `json:"role"`
	OrganisationUUID string `json:"organisation_id"`
	OrganisationRole string `json:"organisation_role"`
	UserEmail        string `json:"email"`
}

// UserRequest is a structure to represent the signup api request
type UserRequest struct {
	Title            string `json:"title"`
	Name             string `json:"name"`
	LastName         string `json:"lastname"`
	Email            string `json:"email"`
	PhoneCountryCode string `json:"phone_country_code"`
	PhoneNumber      string `json:"phone_number"`
	ReferenceKey     string `json:"reference_key"`
	Platform         string `json:"platform"`
	WebAuthn         bool   `json:"webauthn"`
}

// ConvertRequestToUser convert UserRequest struct to User
func (user *UserRequest) ConvertRequestToUser() *models.User {
	mUser := &models.User{}

	mUser.Title = user.Title
	mUser.Role = models.UserRoleUser
	mUser.Name = user.Name
	mUser.LastName = user.LastName

	mUser.LoginEmail = &models.Contact{Type: models.ContactTypeEmail, Level: models.ContactLevelPrimary, Value1: user.Email}
	mUser.LoginPhone = &models.Contact{Type: models.ContactTypePhone, Level: models.ContactLevelSecondary, Value1: user.PhoneCountryCode, Value2: user.PhoneNumber}

	return mUser
}

type organisationRequest struct {
	Title            string `json:"title"`
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

	mUser.Title = request.Title
	mUser.Role = models.UserRoleUser
	mUser.Name = request.Name
	mUser.LastName = request.LastName

	mUser.LoginEmail = &models.Contact{Type: models.ContactTypeEmail, Level: models.ContactLevelPrimary, Value1: request.Email}
	mUser.LoginPhone = &models.Contact{Type: models.ContactTypePhone, Level: models.ContactLevelSecondary, Value1: request.PhoneCountryCode, Value2: request.PhoneNumber}

	mOrganisation := &models.Organisation{}
	mOrganisation.ReferenceKey = request.ReferenceKey
	mOrganisation.Name = request.OrganisationName

	return mUser, mOrganisation
}

// UserAPI handles JWT auth and user management api calls
type UserAPI struct {
	SkipPrefix string
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

// GenerateJWTString generates JWT token string based on user and organisation UUIDS
func GenerateJWTString(userUUID, organisationUUID string) (string, *token, *cigExchange.APIError) {
	tk := &token{
		userUUID,
		organisationUUID,
		jwt.StandardClaims{
			IssuedAt:  time.Now().Unix(),
			ExpiresAt: time.Now().Add(time.Minute * tokenExpirationTimeInMin).Unix(),
		},
	}
	token := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), tk)
	tokenString, err := token.SignedString([]byte(os.Getenv("TOKEN_PASSWORD")))
	if err != nil {
		apiError := cigExchange.NewTokenError("Token generation failed", err)
		return "", nil, apiError
	}

	// save token in redis
	redisKey := tk.UserUUID + "|" + tk.OrganisationUUID

	redisCmd := cigExchange.GetRedis().Set(redisKey, tokenString, time.Minute*tokenExpirationTimeInMin)
	if redisCmd.Err() != nil {
		apiError := cigExchange.NewRedisError("Set token failure", redisCmd.Err())
		return "", nil, apiError
	}

	return tokenString, tk, nil
}

// GetContextValues extracts the userID and organisationID from the request context
// Should be used by JWT enabled API calls
func GetContextValues(r *http.Request) (loggedInUser *cigExchange.LoggedInUser, err error) {
	// extract the entire token struct
	tk, ok := r.Context().Value(keyJWT).(*token)
	if !ok {
		fmt.Println("GetContextValues: no context value exists")
		err = fmt.Errorf("Invalid access token")
		return
	}

	loggedInUser = &cigExchange.LoggedInUser{}
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

		tokenHeader := r.Header.Get("Authorization") // Grab the token from the header

		if tokenHeader == "" { // Token is missing, returns with error code 403 Unauthorized
			apiError := cigExchange.NewAccessForbiddenError("Missing auth token.")
			fmt.Println(apiError.ToString())
			cigExchange.RespondWithAPIError(w, apiError)
			return
		}

		// The token normally comes in format `Bearer {token-body}`, we check if the retrieved token matched this requirement
		splitted := strings.Split(tokenHeader, " ")
		if len(splitted) != 2 {
			apiError := cigExchange.NewAccessForbiddenError("Invalid/Malformed auth token.")
			fmt.Println(apiError.ToString())
			cigExchange.RespondWithAPIError(w, apiError)
			return
		}

		tokenPart := splitted[1] // Grab the token part, what we are truly interested in
		tk := &token{}

		token, err := jwt.ParseWithClaims(tokenPart, tk, func(token *jwt.Token) (interface{}, error) {
			return []byte(os.Getenv("TOKEN_PASSWORD")), nil
		})

		if err != nil { // Malformed token, returns with http code 403 as usual
			apiError := cigExchange.NewAccessForbiddenError("Malformed authentication token.")
			fmt.Println(apiError.ToString())
			cigExchange.RespondWithAPIError(w, apiError)
			return
		}

		if !token.Valid { // Token is invalid, maybe not signed on this server
			apiError := cigExchange.NewAccessForbiddenError("Token is not valid.")
			fmt.Println(apiError.ToString())
			cigExchange.RespondWithAPIError(w, apiError)
			return
		}

		// check token in redis
		redisKey := tk.UserUUID + "|" + tk.OrganisationUUID
		redisCmd := cigExchange.GetRedis().Get(redisKey)
		if redisCmd.Err() != nil {
			apiError := cigExchange.NewAccessForbiddenError("Token is not valid (not issued by the server).")
			fmt.Println(apiError.ToString())
			cigExchange.RespondWithAPIError(w, apiError)
			return
		}
		if redisCmd.Val() != tokenPart {
			apiError := cigExchange.NewAccessForbiddenError("Token is corrupted (not issued by the server).")
			fmt.Println(apiError.ToString())
			cigExchange.RespondWithAPIError(w, apiError)
			return
		}

		// Everything went well, proceed with the request and set the caller to the user retrieved from the parsed token
		ctx := context.WithValue(r.Context(), keyJWT, tk)

		r = r.WithContext(ctx)
		// proceed in the middleware chain!
		next.ServeHTTP(w, r)
	})
}

// CreateUserHandlerPingdom is a pingdom api endpoint to test user registration
// Real registration is called, then cleanup gets performed
func (userAPI *UserAPI) CreateUserHandlerPingdom(w http.ResponseWriter, r *http.Request) {

	// we need to delete the created user and all created objects
	// read the request body and prepare a bytes copy for original call
	buffer := bytes.NewBuffer(make([]byte, 0))
	reader := io.TeeReader(r.Body, buffer)

	userReq := &UserRequest{}
	// decode user object from request body
	err := json.NewDecoder(reader).Decode(userReq)
	if err != nil {
		fmt.Printf("PingdomSignup: error decoding request body: %v", err.Error())
		cigExchange.RespondWithAPIError(w, cigExchange.NewRequestDecodingError(err))
		return
	}

	// close the original request body and replace it with data copy
	defer r.Body.Close()
	r.Body = ioutil.NopCloser(buffer)

	// call the original api call
	userAPI.CreateUserHandler(w, r)

	user, apiError := models.GetUserByEmail(userReq.Email, false)
	if apiError != nil {
		fmt.Printf("PingdomSignup: error during user lookup: %v", apiError.ToString())
		return
	}

	// user should be unverified
	if user.Status != models.UserStatusUnverified {
		fmt.Printf("PingdomSignup: error during user lookup: unexpected user status [active]")
		return
	}

	// delete user and all associated objects
	apiError = models.DeleteUnverifiedUser(user)
	if apiError != nil {
		fmt.Printf("PingdomSignup: error during user deletion: %v", apiError.ToString())
		return
	}
}

// CreateUserHandler handles POST api/users/signup endpoint
func (userAPI *UserAPI) CreateUserHandler(w http.ResponseWriter, r *http.Request) {

	// create user activity record and print error with defer
	info := cigExchange.PrepareActivityInformation(r)
	defer CreateUserActivity(info, models.ActivityTypeSignUp)
	defer cigExchange.PrintAPIError(info)

	resp := &userResponse{}
	resp.UUID = cigExchange.RandomUUID()

	userReq := &UserRequest{}

	// decode user object from request body
	err := json.NewDecoder(r.Body).Decode(userReq)
	if err != nil {
		info.APIError = cigExchange.NewRequestDecodingError(err)
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	// check that we received 'platform' parameter
	if len(userReq.Platform) == 0 {
		info.APIError = cigExchange.NewRequiredFieldError([]string{"platform"})
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	// user must use p2p or trading platform
	if userReq.Platform != PlatformP2P && userReq.Platform != PlatformTrading {
		info.APIError = cigExchange.NewInvalidFieldError("platform", "Invalid platform parameter")
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	user := userReq.ConvertRequestToUser()

	// P2P users are required to have an organisation reference key
	if userReq.Platform == PlatformP2P && len(userReq.ReferenceKey) == 0 {
		info.APIError = cigExchange.NewRequiredFieldError([]string{"reference_key"})
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	// try to create user
	createdUser, apiError := models.CreateUser(user, userReq.ReferenceKey)
	if apiError != nil {
		info.APIError = apiError
		if info.APIError.ShouldSilenceError() {
			cigExchange.Respond(w, resp)
		} else {
			cigExchange.RespondWithAPIError(w, info.APIError)
		}
		return
	}

	// send welcome email async
	go func() {
		parameters := map[string]string{}
		err = cigExchange.SendEmail(cigExchange.EmailTypeWelcome, userReq.Email, parameters)
		if err != nil {
			fmt.Println("CreateUser: email sending error:")
			fmt.Println(err.Error())
		}
	}()

	// handle web authn
	if userReq.WebAuthn {
		// generate session data and public key
		options, sessionData, err := cigExchange.GetWebAuthn().BeginRegistration(createdUser)
		if err != nil {
			info.APIError = cigExchange.NewRequestDecodingError(err)
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}

		// get redis key uuid_web_authn
		rediskey := cigExchange.GenerateRedisKey(createdUser.ID, cigExchange.KeyWebAuthnRegister)
		expiration := 5 * time.Minute

		// marshal session data for storing in redis
		session, err := json.Marshal(sessionData)
		if err != nil {
			info.APIError = cigExchange.NewRequestDecodingError(err)
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}

		redisCmd := cigExchange.GetRedis().Set(rediskey, string(session), expiration)
		if redisCmd.Err() != nil {
			info.APIError = cigExchange.NewRedisError("Set web authn failure", redisCmd.Err())
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}

		// fill response struct
		optionsWithID := struct {
			*protocol.CredentialCreation
			UUID string `json:"uuid"`
		}{
			options,
			createdUser.ID,
		}

		cigExchange.Respond(w, optionsWithID)
		return
	}

	resp.UUID = createdUser.ID
	cigExchange.Respond(w, resp)
}

// CreateUserWebAuthnHandler handles POST api/users/signup/{user_id}/webauthn endpoint
func (userAPI *UserAPI) CreateUserWebAuthnHandler(w http.ResponseWriter, r *http.Request) {

	// create user activity record and print error with defer
	info := cigExchange.PrepareActivityInformation(r)
	defer CreateUserActivity(info, models.ActivityTypeSignUpWebAuth)
	defer cigExchange.PrintAPIError(info)

	userID := mux.Vars(r)["user_id"]

	if len(userID) == 0 {
		info.APIError = cigExchange.NewInvalidFieldError("user_id", "Invalid user id")
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	user, apiError := models.GetUser(userID)
	if apiError != nil {
		info.APIError = apiError
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	// get redis key
	rediskey := cigExchange.GenerateRedisKey(user.ID, cigExchange.KeyWebAuthnRegister)

	// get session id json
	redisCmd := cigExchange.GetRedis().Get(rediskey)
	if redisCmd.Err() != nil {
		info.APIError = cigExchange.NewRedisError("Get session failure", redisCmd.Err())
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	sessionData := webauthn.SessionData{}
	if err := json.Unmarshal([]byte(redisCmd.Val()), &sessionData); err != nil {
		info.APIError = cigExchange.NewRedisError("Get session failure. Can't parse redis value", redisCmd.Err())
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	credential, err := cigExchange.GetWebAuthn().FinishRegistration(user, sessionData, r)
	if err != nil {
		info.APIError = cigExchange.NewInternalServerError("Web Auth finish registration failed", err.Error())
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	// marshal session data for storing in redis
	credString, err := json.Marshal(credential)
	if err != nil {
		info.APIError = cigExchange.NewRequestDecodingError(err)
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	user.LoginWebAuthn = string(credString)
	apiError = user.Save()
	if apiError != nil {
		info.APIError = apiError
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	w.WriteHeader(204)
}

// CreateOrganisationHandler handles POST api/organisations/signup endpoint
func (userAPI *UserAPI) CreateOrganisationHandler(w http.ResponseWriter, r *http.Request) {

	// create user activity record and print error with defer
	info := cigExchange.PrepareActivityInformation(r)
	defer CreateUserActivity(info, models.ActivityTypeOrganisationSignUp)
	defer cigExchange.PrintAPIError(info)

	orgRequest := &organisationRequest{}
	// decode organisation request object from request body
	err := json.NewDecoder(r.Body).Decode(orgRequest)
	if err != nil {
		info.APIError = cigExchange.NewRequestDecodingError(err)
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	// convert request to User and Organisation structs
	user, organisation := orgRequest.convertRequestToUserAndOrganisation()

	// prepare silence error response
	resp := &userResponse{}
	resp.UUID = cigExchange.RandomUUID()

	// check user
	apiError := user.TrimFieldsAndValidate()
	if apiError != nil {
		info.APIError = apiError
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	// query user by email. Email checked in TrimFieldsAndValidate.
	existingUser, apiError := models.GetUserByEmail(user.LoginEmail.Value1, true)
	if apiError != nil {
		info.APIError = apiError
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	// check organisation
	if len(organisation.ReferenceKey) == 0 {
		info.APIError = cigExchange.NewInvalidFieldError("reference_key", "Organisation reference key is invalid")
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}
	if len(organisation.Name) == 0 {
		info.APIError = cigExchange.NewInvalidFieldError("organisation_name", "Organisation name key is invalid")
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	// get organisation by reference key
	orgRefWhere := &models.Organisation{
		ReferenceKey: organisation.ReferenceKey,
	}
	orgRef := &models.Organisation{}
	db := cigExchange.GetDB().Where(orgRefWhere).First(orgRef)
	if db.Error != nil {
		// handle database error
		if !db.RecordNotFound() {
			info.APIError = cigExchange.NewDatabaseError("Organization lookup failed", db.Error)
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}
		// organisation with reference key doesn't exist
		orgRef = nil
	} else {
		if orgRef.Name != organisation.Name {
			// reference key already in use by another organisation
			info.APIError = cigExchange.NewInvalidFieldError("reference_key", "Organisation reference key already in use")
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}
	}

	// check that organisation doesn't exist
	orgWhere := &models.Organisation{
		Name: organisation.Name,
	}
	org := &models.Organisation{}
	db = cigExchange.GetDB().Where(orgWhere).First(org)
	if db.Error != nil {
		// handle database error
		if !db.RecordNotFound() {
			info.APIError = cigExchange.NewDatabaseError("Organization lookup failed", db.Error)
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}
		// organisation doesn't exist
		org = nil
	} else {
		if org.Status == models.OrganisationStatusVerified {
			// check user unverified and organisation is verified
			if existingUser != nil {
				if existingUser.Status == models.UserStatusUnverified {
					info.APIError = cigExchange.NewAccessRightsError("Organisation already exists. Please use the organisation reference key for registration.")
					cigExchange.RespondWithAPIError(w, info.APIError)
					return
				}
			}
			info.APIError = cigExchange.NewAccessRightsError("Organisation already exists. Please ask admin of the organisation to invite you")
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}
		// unverified organisation exists
		// get organisation admin
		orgUserWhere := &models.OrganisationUser{
			OrganisationID:   org.ID,
			OrganisationRole: models.OrganisationRoleAdmin,
			Status:           models.OrganisationUserStatusActive,
		}
		orgUserAdmin := &models.OrganisationUser{}
		db := cigExchange.GetDB().Where(orgUserWhere).First(orgUserAdmin)
		if db.Error != nil {
			if !db.RecordNotFound() {
				info.APIError = cigExchange.NewDatabaseError("Organization user links lookup failed", db.Error)
				cigExchange.RespondWithAPIError(w, info.APIError)
				return
			}
			// organisation without verified admin
		} else {
			// organisation has verified admin
			info.APIError = cigExchange.NewAccessRightsError("Organisation already exists. Please ask admin of the organisation to invite you")
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}
	}

	// existingUser and org can be nil at this point

	// organisation doesn't exists
	if org == nil {
		apiError = organisation.Create()
		if apiError != nil {
			info.APIError = apiError
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}
		org = organisation
	}

	// user doesn't exists
	if existingUser == nil {
		// try to create user with reference key
		existingUser, apiError = models.CreateUser(user, org.ReferenceKey)
		if apiError != nil {
			info.APIError = apiError
			if apiError.ShouldSilenceError() {
				cigExchange.Respond(w, resp)
			} else {
				cigExchange.RespondWithAPIError(w, info.APIError)
			}
			return
		}
	}

	// query organisationUser
	orgUserWhere := &models.OrganisationUser{
		UserID:         existingUser.ID,
		OrganisationID: org.ID,
	}
	orgUser := &models.OrganisationUser{}

	db = cigExchange.GetDB().Where(orgUserWhere).First(orgUser)
	if db.Error != nil {
		if !db.RecordNotFound() {
			info.APIError = cigExchange.NewDatabaseError("OrganizationUser lookup failed", db.Error)
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}

		// organisationUser doesn't exist
		orgUser = &models.OrganisationUser{
			UserID:           existingUser.ID,
			OrganisationID:   org.ID,
			OrganisationRole: models.OrganisationRoleUser,
			IsHome:           false,
			Status:           models.OrganisationUserStatusUnverified,
		}
		apiError = orgUser.Create()
		if apiError != nil {
			info.APIError = apiError
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}
	}

	// send welcome email async
	go func() {
		parameters := map[string]string{}
		err = cigExchange.SendEmail(cigExchange.EmailTypeWelcome, orgRequest.Email, parameters)
		if err != nil {
			fmt.Println("CreateOrganisation: email sending error:")
			fmt.Println(err.Error())
		}
	}()

	resp.UUID = existingUser.ID
	cigExchange.Respond(w, resp)
}

// GetUserHandler handles POST api/users/signin endpoint
func (userAPI *UserAPI) GetUserHandler(w http.ResponseWriter, r *http.Request) {

	// create user activity record and print error with defer
	info := cigExchange.PrepareActivityInformation(r)
	defer CreateUserActivity(info, models.ActivityTypeSignIn)
	defer cigExchange.PrintAPIError(info)

	resp := &userResponse{}
	resp.UUID = cigExchange.RandomUUID()

	userReq := &UserRequest{}
	// decode user object from request body
	err := json.NewDecoder(r.Body).Decode(userReq)
	if err != nil {
		info.APIError = cigExchange.NewRequestDecodingError(err)
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	var apiError *cigExchange.APIError
	user := &models.User{}
	// login using email or phone number
	if len(userReq.Email) > 0 {
		user, apiError = models.GetUserByEmail(userReq.Email, false)
	} else if len(userReq.PhoneCountryCode) > 0 && len(userReq.PhoneNumber) > 0 {
		user, apiError = models.GetUserByMobile(userReq.PhoneCountryCode, userReq.PhoneNumber)
	} else {
		// neither email or phone specified
		apiError = cigExchange.NewRequiredFieldError([]string{"email", "phone_number", "phone_country_code"})
	}

	if apiError != nil {
		info.APIError = apiError
		if apiError.ShouldSilenceError() {
			cigExchange.Respond(w, resp)
		} else {
			cigExchange.RespondWithAPIError(w, info.APIError)
		}
		return
	}

	resp.UUID = user.ID
	cigExchange.Respond(w, resp)
}

// GetUserWebAuthnHandler handles POST api/users/signin/{user_id}/webauthn endpoint
func (userAPI *UserAPI) GetUserWebAuthnHandler(w http.ResponseWriter, r *http.Request) {

	// create user activity record and print error with defer
	info := cigExchange.PrepareActivityInformation(r)
	defer CreateUserActivity(info, models.ActivityTypeSignUpWebAuth)
	defer cigExchange.PrintAPIError(info)

	userID := mux.Vars(r)["user_id"]

	if len(userID) == 0 {
		info.APIError = cigExchange.NewInvalidFieldError("user_id", "Invalid user id")
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	user, apiError := models.GetUser(userID)
	if apiError != nil {
		info.APIError = apiError
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	// get redis key
	rediskey := cigExchange.GenerateRedisKey(user.ID, cigExchange.KeyWebAuthnLogin)

	// get session id json
	redisCmd := cigExchange.GetRedis().Get(rediskey)
	if redisCmd.Err() != nil {
		info.APIError = cigExchange.NewRedisError("Get login session failure", redisCmd.Err())
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	sessionData := webauthn.SessionData{}
	if err := json.Unmarshal([]byte(redisCmd.Val()), &sessionData); err != nil {
		info.APIError = cigExchange.NewRedisError("Get session failure. Can't parse redis value", redisCmd.Err())
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	_, err := cigExchange.GetWebAuthn().FinishLogin(user, sessionData, r)
	if err != nil {
		info.APIError = cigExchange.NewInternalServerError("Web Auth finish registration failed", err.Error())
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	organisationUser, apiError := verifyOrganisationUserAndReturnHome(user)
	if apiError != nil {
		info.APIError = apiError
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	// verification passed, generate jwt and return it
	tokenString, token, apiError := GenerateJWTString(user.ID, organisationUser.OrganisationID)

	if apiError != nil {
		info.APIError = apiError
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	loggedInUser := &cigExchange.LoggedInUser{}
	loggedInUser.UserUUID = token.UserUUID
	loggedInUser.OrganisationUUID = token.OrganisationUUID
	loggedInUser.CreationDate = time.Unix(token.StandardClaims.IssuedAt, 0)
	loggedInUser.ExpirationDate = time.Unix(token.StandardClaims.ExpiresAt, 0)

	info.LoggedInUser = loggedInUser

	resp := &JwtResponse{
		JWT:    tokenString,
		Status: JWTResponseStatusFinished,
	}
	cigExchange.Respond(w, resp)
	CreateUserActivity(info, models.ActivityTypeSessionLength)
}

// SendCodeHandler handles POST api/users/send_otp endpoint
func (userAPI *UserAPI) SendCodeHandler(w http.ResponseWriter, r *http.Request) {

	// create user activity record and print error with defer
	info := cigExchange.PrepareActivityInformation(r)
	defer CreateUserActivity(info, models.ActivityTypeSendOtp)
	defer cigExchange.PrintAPIError(info)

	reqStruct := &verificationCodeRequest{}
	// decode verificationCodeRequest object from request body
	err := json.NewDecoder(r.Body).Decode(reqStruct)
	if err != nil {
		info.APIError = cigExchange.NewRequestDecodingError(err)
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	user, apiError := models.GetUser(reqStruct.UUID)
	if apiError != nil {
		info.APIError = apiError
		if apiError.ShouldSilenceError() {
			// respond with 204
			w.WriteHeader(204)
		} else {
			cigExchange.RespondWithAPIError(w, info.APIError)
		}
		return
	}

	// check that we received 'type' parameter
	if len(reqStruct.Type) == 0 {
		info.APIError = cigExchange.NewRequiredFieldError([]string{"type"})
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	// send code to email or phone number
	if reqStruct.Type == "phone" {
		if user.LoginPhone == nil {
			info.APIError = cigExchange.NewInvalidFieldError("type", "User doesn't have phone contact")
			cigExchange.RespondWithAPIError(w, info.APIError)
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
			info.APIError = cigExchange.NewInvalidFieldError("type", "User doesn't have email")
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}
		rediskey := cigExchange.GenerateRedisKey(reqStruct.UUID, cigExchange.KeySignUp)
		expiration := 5 * time.Minute

		code := cigExchange.RandCode(6)
		redisCmd := cigExchange.GetRedis().Set(rediskey, code, expiration)
		if redisCmd.Err() != nil {
			info.APIError = cigExchange.NewRedisError("Set code failure", redisCmd.Err())
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}
		// process the send OTP async so that client won't see any delays
		go func() {
			parameters := map[string]string{
				"pincode": code,
			}
			err = cigExchange.SendEmail(cigExchange.EmailTypePinCode, user.LoginEmail.Value1, parameters)
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
		info.APIError = cigExchange.NewInvalidFieldError("type", "Invalid otp type")
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}
	w.WriteHeader(204)
}

// VerifyCodeHandler handles POST api/users/verify_otp endpoint
func (userAPI *UserAPI) VerifyCodeHandler(w http.ResponseWriter, r *http.Request) {

	// create user activity record and print error with defer
	info := cigExchange.PrepareActivityInformation(r)
	defer CreateUserActivity(info, models.ActivityTypeVerifyOtp)
	defer cigExchange.PrintAPIError(info)

	// prepare the default response to send (unauthorized / invalid code)
	secureErrorResponse := &cigExchange.APIError{}
	secureErrorResponse.SetErrorType(cigExchange.ErrorTypeUnauthorized)
	secureErrorResponse.NewNestedError(cigExchange.ReasonFieldInvalid, "Invalid code")

	reqStruct := &verificationCodeRequest{}
	// decode verificationCodeRequest object from request body
	err := json.NewDecoder(r.Body).Decode(reqStruct)
	if err != nil {
		info.APIError = cigExchange.NewRequestDecodingError(err)
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	user, apiError := models.GetUser(reqStruct.UUID)
	if err != nil {
		info.APIError = apiError
		if apiError.ShouldSilenceError() {
			cigExchange.RespondWithAPIError(w, secureErrorResponse)
		} else {
			cigExchange.RespondWithAPIError(w, info.APIError)
		}
		return
	}

	// check that we received 'type' parameter
	if len(reqStruct.Type) == 0 {
		info.APIError = cigExchange.NewRequiredFieldError([]string{"type"})
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	// verify code
	if reqStruct.Type == "phone" {
		if user.LoginPhone == nil {
			info.APIError = cigExchange.NewInvalidFieldError("type", "User doesn't have phone contact")
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}
		twilioClient := cigExchange.GetTwilio()
		_, err := twilioClient.VerifyOTP(reqStruct.Code, user.LoginPhone.Value1, user.LoginPhone.Value2)
		if err != nil {
			info.APIError = cigExchange.NewTwilioError("Verify OTP", err)
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}

	} else if reqStruct.Type == "email" {
		if user.LoginEmail == nil {
			info.APIError = cigExchange.NewInvalidFieldError("type", "User doesn't have email contact")
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}
		rediskey := cigExchange.GenerateRedisKey(reqStruct.UUID, cigExchange.KeySignUp)

		redisCmd := cigExchange.GetRedis().Get(rediskey)
		if redisCmd.Err() != nil {
			info.APIError = cigExchange.NewRedisError("Get code failure", redisCmd.Err())
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}
		if redisCmd.Val() != reqStruct.Code {
			info.APIError = secureErrorResponse
			cigExchange.RespondWithAPIError(w, secureErrorResponse)
			return
		}
	} else {
		info.APIError = cigExchange.NewInvalidFieldError("type", "Invalid otp type")
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	// web authn autorization
	if len(user.LoginWebAuthn) > 0 {
		// generate session data and public key
		options, sessionData, err := cigExchange.GetWebAuthn().BeginLogin(user)
		if err != nil {
			info.APIError = cigExchange.NewRequestDecodingError(err)
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}

		// get redis key uuid_web_authn
		rediskey := cigExchange.GenerateRedisKey(user.ID, cigExchange.KeyWebAuthnLogin)
		expiration := 5 * time.Minute

		// marshal session data for storing in redis
		session, err := json.Marshal(sessionData)
		if err != nil {
			info.APIError = cigExchange.NewRequestDecodingError(err)
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}

		redisCmd := cigExchange.GetRedis().Set(rediskey, string(session), expiration)
		if redisCmd.Err() != nil {
			info.APIError = cigExchange.NewRedisError("Set web authn failure", redisCmd.Err())
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}

		// fill response struct
		webAuthResponse := struct {
			*protocol.CredentialAssertion
			Status string `json:"status"`
		}{
			options,
			"Web Authn",
		}

		cigExchange.Respond(w, webAuthResponse)
		return
	}

	organisationUser, apiError := verifyOrganisationUserAndReturnHome(user)
	if apiError != nil {
		info.APIError = apiError
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	// verification passed, generate jwt and return it
	tokenString, token, apiError := GenerateJWTString(user.ID, organisationUser.OrganisationID)

	if apiError != nil {
		info.APIError = apiError
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	loggedInUser := &cigExchange.LoggedInUser{}
	loggedInUser.UserUUID = token.UserUUID
	loggedInUser.OrganisationUUID = token.OrganisationUUID
	loggedInUser.CreationDate = time.Unix(token.StandardClaims.IssuedAt, 0)
	loggedInUser.ExpirationDate = time.Unix(token.StandardClaims.ExpiresAt, 0)

	info.LoggedInUser = loggedInUser

	resp := &JwtResponse{
		JWT:    tokenString,
		Status: JWTResponseStatusFinished,
	}
	cigExchange.Respond(w, resp)
	CreateUserActivity(info, models.ActivityTypeSessionLength)
}

func verifyOrganisationUserAndReturnHome(user *models.User) (*models.OrganisationUser, *cigExchange.APIError) {

	// get OrganisationUsers related to user
	organisationUser := &models.OrganisationUser{}
	orgUsers := make([]*models.OrganisationUser, 0)
	db := cigExchange.GetDB().Model(user).Related(&orgUsers, "UserID")
	if db.Error != nil {
		// organization can be missed
		if !db.RecordNotFound() {
			return organisationUser, cigExchange.NewDatabaseError("Organization user links lookup failed", db.Error)
		}
	}

	// select home organisation and activate organisationUsers
	if len(orgUsers) > 0 {
		// search for home organisation
		for _, orgUser := range orgUsers {
			if orgUser.IsHome {
				organisationUser = orgUser
				break
			}
		}

		// add home organisation if user hasn't
		if len(organisationUser.ID) == 0 {
			organisationUser = orgUsers[0]
			organisationUser.IsHome = true
			organisationUser.Update()
		}

		// activate organisationUsers
		for _, orgUser := range orgUsers {
			role := models.OrganisationRoleUser
			// search for organisation admin
			orgUserWhere := &models.OrganisationUser{
				OrganisationID:   orgUser.OrganisationID,
				OrganisationRole: models.OrganisationRoleAdmin,
				Status:           models.OrganisationUserStatusActive,
			}
			orgUserAdmin := &models.OrganisationUser{}
			db := cigExchange.GetDB().Where(orgUserWhere).First(orgUserAdmin)
			if db.Error != nil {
				if !db.RecordNotFound() {
					return organisationUser, cigExchange.NewDatabaseError("Organization user links lookup failed", db.Error)
				}
				role = models.OrganisationRoleAdmin
			}

			// do not activate invitations automatically... (OrganisationUserStatusInvited)
			// user still needs to follow the email link and accept invitation explicitely
			if orgUser.Status == models.OrganisationUserStatusUnverified {
				orgUser.Status = models.OrganisationUserStatusActive
				orgUser.OrganisationRole = role
				orgUser.Update()
			}
		}
	}

	// user is verified
	if user.Status != models.UserStatusVerified {
		user.Status = models.UserStatusVerified
		apiError := user.Save()
		if apiError != nil {
			return organisationUser, apiError
		}
	}

	return organisationUser, nil
}

// GetInfo handles Get api/me/info endpoint
func (userAPI *UserAPI) GetInfo(w http.ResponseWriter, r *http.Request) {

	// create user activity record and print error with defer
	info := cigExchange.PrepareActivityInformation(r)
	defer CreateUserActivity(info, models.ActivityTypeUserInfo)
	defer cigExchange.PrintAPIError(info)

	// load context user info
	loggedInUser, err := GetContextValues(r)
	if err != nil {
		info.APIError = cigExchange.NewRoutingError(err)
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}
	info.LoggedInUser = loggedInUser

	// get user
	user, apiError := models.GetUser(loggedInUser.UserUUID)
	if apiError != nil {
		info.APIError = apiError
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	orgUser := &models.OrganisationUser{}
	if len(loggedInUser.OrganisationUUID) > 0 {
		// find organisation user
		searchOrgUser := &models.OrganisationUser{
			OrganisationID: loggedInUser.OrganisationUUID,
			UserID:         loggedInUser.UserUUID,
		}

		orgUser, apiError = searchOrgUser.Find()
		if apiError != nil {
			info.APIError = apiError
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}
	}

	email := ""
	if user.LoginEmail != nil {
		email = user.LoginEmail.Value1
	}
	resp := &infoResponse{
		UserUUID:         loggedInUser.UserUUID,
		Role:             user.Role,
		OrganisationUUID: loggedInUser.OrganisationUUID,
		OrganisationRole: orgUser.OrganisationRole,
		UserEmail:        email,
	}
	cigExchange.Respond(w, resp)
}

// ChangeOrganisationHandler handles POST api/users/switch/{organisation_id} endpoint
func (userAPI *UserAPI) ChangeOrganisationHandler(w http.ResponseWriter, r *http.Request) {

	// create user activity record and print error with defer
	info := cigExchange.PrepareActivityInformation(r)
	defer CreateUserActivity(info, models.ActivityTypeSwitchOrganisation)
	defer cigExchange.PrintAPIError(info)

	organisationID := mux.Vars(r)["organisation_id"]

	// load context user info
	loggedInUser, err := GetContextValues(r)
	if err != nil {
		info.APIError = cigExchange.NewRoutingError(err)
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}
	info.LoggedInUser = loggedInUser

	// check if user is already logged into the organisation
	if loggedInUser.OrganisationUUID == organisationID {
		// respond with the same JWT
		authHeader := r.Header.Get("Authorization")
		splitted := strings.Split(authHeader, " ")
		if len(splitted) != 2 {
			info.APIError = cigExchange.NewAccessForbiddenError("Invalid/Malformed auth token.")
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}
		resp := &JwtResponse{
			JWT:    splitted[1],
			Status: JWTResponseStatusFinished,
		}
		cigExchange.Respond(w, resp)
		return
	}

	// check admin
	userRole, apiError := models.GetUserRole(loggedInUser.UserUUID)
	if apiError != nil {
		info.APIError = apiError
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	// skip check for admin
	if userRole != models.UserRoleAdmin {
		// find organisation user
		searchOrgUser := &models.OrganisationUser{
			OrganisationID: organisationID,
			UserID:         loggedInUser.UserUUID,
		}

		orgUser, apiError := searchOrgUser.Find()
		if apiError != nil {
			info.APIError = apiError
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}

		// check that user belong to organisation
		if orgUser.UserID != loggedInUser.UserUUID {
			info.APIError = cigExchange.NewInvalidFieldError("organisation_id", "User don't belong to organisation")
			cigExchange.RespondWithAPIError(w, info.APIError)
			return
		}
	}

	// verification passed, generate jwt and return it
	tokenString, _, apiError := GenerateJWTString(loggedInUser.UserUUID, organisationID)
	if apiError != nil {
		info.APIError = apiError
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	// remove previous token from redis
	redisKey := loggedInUser.UserUUID + "|" + loggedInUser.OrganisationUUID
	intRedisCmd := cigExchange.GetRedis().Del(redisKey)
	if intRedisCmd.Err() != nil {
		info.APIError = cigExchange.NewRedisError("Del token failure", intRedisCmd.Err())
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	resp := &JwtResponse{
		JWT:    tokenString,
		Status: JWTResponseStatusFinished,
	}
	cigExchange.Respond(w, resp)
}

// PingJWT handles GET api/ping-jwt endpoint
func (userAPI *UserAPI) PingJWT(w http.ResponseWriter, r *http.Request) {

	// create user activity record and print error with defer
	info := cigExchange.PrepareActivityInformation(r)
	defer cigExchange.PrintAPIError(info)

	// load context user info
	loggedInUser, err := GetContextValues(r)
	if err != nil {
		info.APIError = cigExchange.NewRoutingError(err)
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}
	info.LoggedInUser = loggedInUser

	apiError := UpdateUserActivity(info, models.ActivityTypeSessionLength)
	if apiError != nil {
		info.APIError = apiError
		cigExchange.RespondWithAPIError(w, info.APIError)
		return
	}

	w.WriteHeader(204)
}

// CreateUserActivity inserts new user activity object into db
func CreateUserActivity(info *cigExchange.ActivityInformation, activityType string) *cigExchange.APIError {

	activity, apiErr := convertToUserActivity(info, activityType)
	if apiErr != nil {
		fmt.Println(apiErr.ToString())
		return apiErr
	}

	// create user activity record
	err := cigExchange.GetDB().Create(activity).Error
	if err != nil {
		apiErr = cigExchange.NewDatabaseError("Create user activity call failed", err)
		fmt.Println(apiErr.ToString())
		return apiErr
	}
	return nil
}

// UpdateUserActivity inserts new user activity object into db
func UpdateUserActivity(info *cigExchange.ActivityInformation, activityType string) *cigExchange.APIError {

	activity, apiErr := convertToUserActivity(info, activityType)
	if apiErr != nil {
		fmt.Println(apiErr.ToString())
		return apiErr
	}

	activitySave, apiErr := activity.FindSessionActivity()
	if apiErr != nil {
		fmt.Println(apiErr.ToString())
		return apiErr
	}

	// create user activity record
	err := cigExchange.GetDB().Save(activitySave).Error
	if err != nil {
		apiErr = cigExchange.NewDatabaseError("Update user activity call failed", err)
		fmt.Println(apiErr.ToString())
		return apiErr
	}
	return nil
}

func convertToUserActivity(info *cigExchange.ActivityInformation, activityType string) (*models.UserActivity, *cigExchange.APIError) {

	activity := &models.UserActivity{}
	activity.Type = activityType

	// add jwt to user activity
	if info.LoggedInUser == nil {
		activity.UserID = models.UnknownUser
	} else {
		activity.UserID = info.LoggedInUser.UserUUID
		jsonBytes, err := json.Marshal(info.LoggedInUser)
		if err != nil {
			apiErr := cigExchange.NewJSONEncodingError(cigExchange.MessageJSONEncoding, err)
			return activity, apiErr
		}

		activity.JWT = postgres.Jsonb{RawMessage: jsonBytes}
	}

	// add api error to user activity
	if info.APIError != nil {
		jsonBytes, err := json.Marshal(info.APIError)
		if err != nil {
			apiErr := cigExchange.NewJSONEncodingError(cigExchange.MessageJSONEncoding, err)
			return activity, apiErr
		}
		jsonStr := string(jsonBytes)
		activity.Info = &jsonStr
	}

	// set remote address
	activity.RemoteAddr = info.RemoteAddr

	// check user activity type
	if len(activity.Type) == 0 {
		apiErr := &cigExchange.APIError{}
		apiErr.SetErrorType(cigExchange.ErrorTypeInternalServer)

		apiErr.NewNestedError(cigExchange.ReasonUserActivityFailure, "Missing activity type")
		return activity, apiErr
	}
	return activity, nil
}

// CreateCustomUserActivity inserts custom user activity object into db
func CreateCustomUserActivity(info *cigExchange.ActivityInformation, infoMap map[string]interface{}) *cigExchange.APIError {

	activity := &models.UserActivity{}

	// check 'type' field
	typeVal, ok := infoMap["type"]
	if !ok {
		return cigExchange.NewInvalidFieldError("type", "Required field 'type' missing")
	}

	typeStr, ok := typeVal.(string)
	if !ok {
		return cigExchange.NewInvalidFieldError("type", "Required field 'type' is not string")
	}

	if len(typeStr) == 0 {
		return cigExchange.NewInvalidFieldError("type", "Required field 'type' missing")
	}

	activity.Type = typeStr

	if info.LoggedInUser == nil {
		activity.UserID = models.UnknownUser
	} else {
		activity.UserID = info.LoggedInUser.UserUUID
		jsonBytes, err := json.Marshal(info.LoggedInUser)
		if err != nil {
			apiErr := cigExchange.NewJSONEncodingError(cigExchange.MessageJSONEncoding, err)
			fmt.Println(apiErr.ToString())
			return apiErr
		}

		activity.JWT = postgres.Jsonb{RawMessage: jsonBytes}
	}

	// add infoMap to user activity
	jsonBytes, err := json.Marshal(infoMap)
	if err != nil {
		apiErr := cigExchange.NewJSONEncodingError(cigExchange.MessageJSONEncoding, err)
		fmt.Println(apiErr.ToString())
		return apiErr
	}
	jsonStr := string(jsonBytes)
	activity.Info = &jsonStr

	// create user activity record
	err = cigExchange.GetDB().Create(activity).Error
	if err != nil {
		apiErr := cigExchange.NewDatabaseError("Create user activity  call failed", err)
		fmt.Println(apiErr.ToString())
		return apiErr
	}
	return nil
}

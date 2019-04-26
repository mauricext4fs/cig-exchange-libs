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

// JwtResponse structure
type JwtResponse struct {
	JWT string `json:"jwt"`
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

// LoggedInUser is passed to controllers after jwt auth
type LoggedInUser struct {
	UserUUID         string    `json:"user_id"`
	OrganisationUUID string    `json:"organisation_id"`
	CreationDate     time.Time `json:"creation_date"`
	ExpirationDate   time.Time `json:"expiration_date"`
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
func GenerateJWTString(userUUID, organisationUUID string) (string, *cigExchange.APIError) {
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
		return "", apiError
	}

	// save token in redis
	redisKey := tk.UserUUID + "|" + tk.OrganisationUUID

	redisCmd := cigExchange.GetRedis().Set(redisKey, tokenString, time.Minute*tokenExpirationTimeInMin)
	if redisCmd.Err() != nil {
		apiError := cigExchange.NewRedisError("Set token failure", redisCmd.Err())
		return "", apiError
	}

	return tokenString, nil
}

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

// CreateUserHandler handles POST api/users/signup endpoint
func (userAPI *UserAPI) CreateUserHandler(w http.ResponseWriter, r *http.Request) {

	// create user activity record and print error with defer
	apiErrorP, loggedInUserP := PrepareActivityVariables()
	defer CreateUserActivity(loggedInUserP, apiErrorP, models.ActivityTypeSignUp)
	defer cigExchange.PrintAPIError(apiErrorP)

	resp := &userResponse{}
	resp.UUID = cigExchange.RandomUUID()

	userReq := &UserRequest{}

	// decode user object from request body
	err := json.NewDecoder(r.Body).Decode(userReq)
	if err != nil {
		*apiErrorP = cigExchange.NewRequestDecodingError(err)
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}

	// check that we received 'platform' parameter
	if len(userReq.Platform) == 0 {
		*apiErrorP = cigExchange.NewRequiredFieldError([]string{"platform"})
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}

	// user must use p2p or trading platform
	if userReq.Platform != PlatformP2P && userReq.Platform != PlatformTrading {
		*apiErrorP = cigExchange.NewInvalidFieldError("platform", "Invalid platform parameter")
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}

	user := userReq.ConvertRequestToUser()

	// P2P users are required to have an organisation reference key
	if userReq.Platform == PlatformP2P && len(userReq.ReferenceKey) == 0 {
		*apiErrorP = cigExchange.NewRequiredFieldError([]string{"reference_key"})
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}

	// try to create user
	createdUser, apiError := models.CreateUser(user, userReq.ReferenceKey)
	if apiError != nil {
		*apiErrorP = apiError
		if (*apiErrorP).ShouldSilenceError() {
			cigExchange.Respond(w, resp)
		} else {
			cigExchange.RespondWithAPIError(w, *apiErrorP)
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

	resp.UUID = createdUser.ID
	cigExchange.Respond(w, resp)
}

// CreateOrganisationHandler handles POST api/organisations/signup endpoint
func (userAPI *UserAPI) CreateOrganisationHandler(w http.ResponseWriter, r *http.Request) {

	// create user activity record and print error with defer
	apiErrorP, loggedInUserP := PrepareActivityVariables()
	defer CreateUserActivity(loggedInUserP, apiErrorP, models.ActivityTypeOrganisationSignUp)
	defer cigExchange.PrintAPIError(apiErrorP)

	orgRequest := &organisationRequest{}
	// decode organisation request object from request body
	err := json.NewDecoder(r.Body).Decode(orgRequest)
	if err != nil {
		*apiErrorP = cigExchange.NewRequestDecodingError(err)
		cigExchange.RespondWithAPIError(w, *apiErrorP)
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
		*apiErrorP = apiError
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}

	// query user by email. Email checked in TrimFieldsAndValidate.
	existingUser, apiError := models.GetUserByEmail(user.LoginEmail.Value1, true)
	if apiError != nil {
		*apiErrorP = apiError
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}

	// check organisation
	if len(organisation.ReferenceKey) == 0 {
		*apiErrorP = cigExchange.NewInvalidFieldError("reference_key", "Organisation reference key is invalid")
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}
	if len(organisation.Name) == 0 {
		*apiErrorP = cigExchange.NewInvalidFieldError("organisation_name", "Organisation name key is invalid")
		cigExchange.RespondWithAPIError(w, *apiErrorP)
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
			*apiErrorP = cigExchange.NewDatabaseError("Organization lookup failed", db.Error)
			cigExchange.RespondWithAPIError(w, *apiErrorP)
			return
		}
		// organisation with reference key doesn't exist
		orgRef = nil
	} else {
		if orgRef.Name != organisation.Name {
			// reference key already in use by another organisation
			*apiErrorP = cigExchange.NewInvalidFieldError("reference_key", "Organisation reference key already in use")
			cigExchange.RespondWithAPIError(w, *apiErrorP)
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
			*apiErrorP = cigExchange.NewDatabaseError("Organization lookup failed", db.Error)
			cigExchange.RespondWithAPIError(w, *apiErrorP)
			return
		}
		// organisation doesn't exist
		org = nil
	} else {
		if org.Status == models.OrganisationStatusVerified {
			// check user unverified and organisation is verified
			if existingUser != nil {
				if existingUser.Status == models.UserStatusUnverified {
					*apiErrorP = cigExchange.NewAccessRightsError("Organisation already exists. Please use the organisation reference key for registration.")
					cigExchange.RespondWithAPIError(w, *apiErrorP)
					return
				}
			}
			*apiErrorP = cigExchange.NewAccessRightsError("Organisation already exists. Please ask admin of the organisation to invite you")
			cigExchange.RespondWithAPIError(w, *apiErrorP)
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
				*apiErrorP = cigExchange.NewDatabaseError("Organization user links lookup failed", db.Error)
				cigExchange.RespondWithAPIError(w, *apiErrorP)
				return
			}
			// organisation without verified admin
		} else {
			// organisation has verified admin
			*apiErrorP = cigExchange.NewAccessRightsError("Organisation already exists. Please ask admin of the organisation to invite you")
			cigExchange.RespondWithAPIError(w, *apiErrorP)
			return
		}
	}

	// existingUser and org can be nil at this point

	// organisation doesn't exists
	if org == nil {
		apiError = organisation.Create()
		if apiError != nil {
			*apiErrorP = apiError
			cigExchange.RespondWithAPIError(w, *apiErrorP)
			return
		}
		org = organisation
	}

	// user doesn't exists
	if existingUser == nil {
		// try to create user with reference key
		existingUser, apiError = models.CreateUser(user, org.ReferenceKey)
		if apiError != nil {
			*apiErrorP = apiError
			if apiError.ShouldSilenceError() {
				cigExchange.Respond(w, resp)
			} else {
				cigExchange.RespondWithAPIError(w, *apiErrorP)
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
			*apiErrorP = cigExchange.NewDatabaseError("OrganizationUser lookup failed", db.Error)
			cigExchange.RespondWithAPIError(w, *apiErrorP)
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
			*apiErrorP = apiError
			cigExchange.RespondWithAPIError(w, *apiErrorP)
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
	apiErrorP, loggedInUserP := PrepareActivityVariables()
	defer CreateUserActivity(loggedInUserP, apiErrorP, models.ActivityTypeOrganisationSignUp)
	defer cigExchange.PrintAPIError(apiErrorP)

	resp := &userResponse{}
	resp.UUID = cigExchange.RandomUUID()

	userReq := &UserRequest{}
	// decode user object from request body
	err := json.NewDecoder(r.Body).Decode(userReq)
	if err != nil {
		*apiErrorP = cigExchange.NewRequestDecodingError(err)
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}

	user := &models.User{}
	// login using email or phone number
	if len(userReq.Email) > 0 {
		user, *apiErrorP = models.GetUserByEmail(userReq.Email, false)
	} else if len(userReq.PhoneCountryCode) > 0 && len(userReq.PhoneNumber) > 0 {
		user, *apiErrorP = models.GetUserByMobile(userReq.PhoneCountryCode, userReq.PhoneNumber)
	} else {
		// neither email or phone specified
		*apiErrorP = cigExchange.NewRequiredFieldError([]string{"email", "phone_number", "phone_country_code"})
	}

	if *apiErrorP != nil {
		if (*apiErrorP).ShouldSilenceError() {
			cigExchange.Respond(w, resp)
		} else {
			cigExchange.RespondWithAPIError(w, *apiErrorP)
		}
		return
	}

	resp.UUID = user.ID
	cigExchange.Respond(w, resp)
}

// SendCodeHandler handles POST api/users/send_otp endpoint
func (userAPI *UserAPI) SendCodeHandler(w http.ResponseWriter, r *http.Request) {

	// create user activity record and print error with defer
	apiErrorP, loggedInUserP := PrepareActivityVariables()
	defer CreateUserActivity(loggedInUserP, apiErrorP, models.ActivityTypeSendOtp)
	defer cigExchange.PrintAPIError(apiErrorP)

	reqStruct := &verificationCodeRequest{}
	// decode verificationCodeRequest object from request body
	err := json.NewDecoder(r.Body).Decode(reqStruct)
	if err != nil {
		*apiErrorP = cigExchange.NewRequestDecodingError(err)
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}

	user, apiError := models.GetUser(reqStruct.UUID)
	if apiError != nil {
		*apiErrorP = apiError
		if apiError.ShouldSilenceError() {
			// respond with 204
			w.WriteHeader(204)
		} else {
			cigExchange.RespondWithAPIError(w, *apiErrorP)
		}
		return
	}

	// check that we received 'type' parameter
	if len(reqStruct.Type) == 0 {
		*apiErrorP = cigExchange.NewRequiredFieldError([]string{"type"})
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}

	// send code to email or phone number
	if reqStruct.Type == "phone" {
		if user.LoginPhone == nil {
			*apiErrorP = cigExchange.NewInvalidFieldError("type", "User doesn't have phone contact")
			cigExchange.RespondWithAPIError(w, *apiErrorP)
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
			*apiErrorP = cigExchange.NewInvalidFieldError("type", "User doesn't have email")
			cigExchange.RespondWithAPIError(w, *apiErrorP)
			return
		}
		rediskey := cigExchange.GenerateRedisKey(reqStruct.UUID)
		expiration := 5 * time.Minute

		code := cigExchange.RandCode(6)
		redisCmd := cigExchange.GetRedis().Set(rediskey, code, expiration)
		if redisCmd.Err() != nil {
			*apiErrorP = cigExchange.NewRedisError("Set code failure", redisCmd.Err())
			cigExchange.RespondWithAPIError(w, *apiErrorP)
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
		*apiErrorP = cigExchange.NewInvalidFieldError("type", "Invalid otp type")
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}
	w.WriteHeader(204)
}

// VerifyCodeHandler handles POST api/users/verify_otp endpoint
func (userAPI *UserAPI) VerifyCodeHandler(w http.ResponseWriter, r *http.Request) {

	// create user activity record and print error with defer
	apiErrorP, loggedInUserP := PrepareActivityVariables()
	defer CreateUserActivity(loggedInUserP, apiErrorP, models.ActivityTypeVerifyOtp)
	defer cigExchange.PrintAPIError(apiErrorP)

	// prepare the default response to send (unauthorized / invalid code)
	secureErrorResponse := &cigExchange.APIError{}
	secureErrorResponse.SetErrorType(cigExchange.ErrorTypeUnauthorized)
	secureErrorResponse.NewNestedError(cigExchange.ReasonFieldInvalid, "Invalid code")

	reqStruct := &verificationCodeRequest{}
	// decode verificationCodeRequest object from request body
	err := json.NewDecoder(r.Body).Decode(reqStruct)
	if err != nil {
		*apiErrorP = cigExchange.NewRequestDecodingError(err)
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}

	user, apiError := models.GetUser(reqStruct.UUID)
	if err != nil {
		*apiErrorP = apiError
		if apiError.ShouldSilenceError() {
			cigExchange.RespondWithAPIError(w, secureErrorResponse)
		} else {
			cigExchange.RespondWithAPIError(w, *apiErrorP)
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
			*apiErrorP = cigExchange.NewDatabaseError("Organization user links lookup failed", db.Error)
			cigExchange.RespondWithAPIError(w, *apiErrorP)
			return
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
					*apiErrorP = cigExchange.NewDatabaseError("Organization user links lookup failed", db.Error)
					cigExchange.RespondWithAPIError(w, *apiErrorP)
					return
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

	// check that we received 'type' parameter
	if len(reqStruct.Type) == 0 {
		*apiErrorP = cigExchange.NewRequiredFieldError([]string{"type"})
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}

	// verify code
	if reqStruct.Type == "phone" {
		if user.LoginPhone == nil {
			*apiErrorP = cigExchange.NewInvalidFieldError("type", "User doesn't have phone contact")
			cigExchange.RespondWithAPIError(w, *apiErrorP)
			return
		}
		twilioClient := cigExchange.GetTwilio()
		_, err := twilioClient.VerifyOTP(reqStruct.Code, user.LoginPhone.Value1, user.LoginPhone.Value2)
		if err != nil {
			*apiErrorP = cigExchange.NewTwilioError("Verify OTP", err)
			cigExchange.RespondWithAPIError(w, *apiErrorP)
			return
		}

	} else if reqStruct.Type == "email" {
		if user.LoginEmail == nil {
			*apiErrorP = cigExchange.NewInvalidFieldError("type", "User doesn't have email contact")
			cigExchange.RespondWithAPIError(w, *apiErrorP)
			return
		}
		rediskey := cigExchange.GenerateRedisKey(reqStruct.UUID)

		redisCmd := cigExchange.GetRedis().Get(rediskey)
		if redisCmd.Err() != nil {
			*apiErrorP = cigExchange.NewRedisError("Get code failure", redisCmd.Err())
			cigExchange.RespondWithAPIError(w, *apiErrorP)
			return
		}
		if redisCmd.Val() != reqStruct.Code {
			*apiErrorP = secureErrorResponse
			fmt.Println("VerifyCode: code mismatch, expecting " + redisCmd.Val())
			cigExchange.RespondWithAPIError(w, secureErrorResponse)
			return
		}
	} else {
		*apiErrorP = cigExchange.NewInvalidFieldError("type", "Invalid otp type")
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}

	// user is verified
	user.Status = models.UserStatusVerified
	apiError = user.Save()
	if apiError != nil {
		*apiErrorP = apiError
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}

	// verification passed, generate jwt and return it
	tokenString, apiError := GenerateJWTString(user.ID, organisationUser.OrganisationID)

	if apiError != nil {
		*apiErrorP = apiError
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}

	loggedInUser := &LoggedInUser{}
	loggedInUser.UserUUID = user.ID
	loggedInUser.OrganisationUUID = organisationUser.OrganisationID
	loggedInUser.CreationDate = time.Now()
	loggedInUser.ExpirationDate = time.Now().Add(time.Minute * tokenExpirationTimeInMin)

	*loggedInUserP = loggedInUser

	resp := &JwtResponse{
		JWT: tokenString,
	}
	cigExchange.Respond(w, resp)
	CreateUserActivity(loggedInUserP, apiErrorP, models.ActivityTypeSessionLength)
}

// GetInfo handles Get api/me/info endpoint
func (userAPI *UserAPI) GetInfo(w http.ResponseWriter, r *http.Request) {

	// create user activity record and print error with defer
	apiErrorP, loggedInUserP := PrepareActivityVariables()
	defer CreateUserActivity(loggedInUserP, apiErrorP, models.ActivityTypeUserInfo)
	defer cigExchange.PrintAPIError(apiErrorP)

	// load context user info
	loggedInUser, err := GetContextValues(r)
	if err != nil {
		*apiErrorP = cigExchange.NewRoutingError(err)
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}
	*loggedInUserP = loggedInUser

	// get user
	user, apiError := models.GetUser(loggedInUser.UserUUID)
	if apiError != nil {
		*apiErrorP = apiError
		cigExchange.RespondWithAPIError(w, *apiErrorP)
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
			*apiErrorP = apiError
			cigExchange.RespondWithAPIError(w, *apiErrorP)
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
	apiErrorP, loggedInUserP := PrepareActivityVariables()
	defer CreateUserActivity(loggedInUserP, apiErrorP, models.ActivityTypeSwitchOrganisation)
	defer cigExchange.PrintAPIError(apiErrorP)

	organisationID := mux.Vars(r)["organisation_id"]

	// load context user info
	loggedInUser, err := GetContextValues(r)
	if err != nil {
		*apiErrorP = cigExchange.NewRoutingError(err)
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}
	*loggedInUserP = loggedInUser

	// check admin
	userRole, apiError := models.GetUserRole(loggedInUser.UserUUID)
	if apiError != nil {
		*apiErrorP = apiError
		cigExchange.RespondWithAPIError(w, *apiErrorP)
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
			*apiErrorP = apiError
			cigExchange.RespondWithAPIError(w, *apiErrorP)
			return
		}

		// check that user belong to organisation
		if orgUser.UserID != loggedInUser.UserUUID {
			*apiErrorP = cigExchange.NewInvalidFieldError("organisation_id", "User don't belong to organisation")
			cigExchange.RespondWithAPIError(w, *apiErrorP)
			return
		}
	}

	// verification passed, generate jwt and return it
	tokenString, apiError := GenerateJWTString(loggedInUser.UserUUID, organisationID)
	if apiError != nil {
		*apiErrorP = apiError
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}

	// remove previous token from redis
	redisKey := loggedInUser.UserUUID + "|" + loggedInUser.OrganisationUUID
	intRedisCmd := cigExchange.GetRedis().Del(redisKey)
	if intRedisCmd.Err() != nil {
		*apiErrorP = cigExchange.NewRedisError("Del token failure", intRedisCmd.Err())
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}

	resp := &JwtResponse{
		JWT: tokenString,
	}
	cigExchange.Respond(w, resp)
}

// PingJWT handles GET api/ping-jwt endpoint
func (userAPI *UserAPI) PingJWT(w http.ResponseWriter, r *http.Request) {

	// create user activity record and print error with defer
	apiErrorP, loggedInUserP := PrepareActivityVariables()
	defer cigExchange.PrintAPIError(apiErrorP)

	// load context user info
	loggedInUser, err := GetContextValues(r)
	if err != nil {
		*apiErrorP = cigExchange.NewRoutingError(err)
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}
	*loggedInUserP = loggedInUser

	apiError := UpdateUserActivity(loggedInUserP, apiErrorP, models.ActivityTypeSessionLength)
	if apiError != nil {
		*apiErrorP = apiError
		cigExchange.RespondWithAPIError(w, *apiErrorP)
		return
	}

	w.WriteHeader(204)
}

// CreateUserActivity inserts new user activity object into db
func CreateUserActivity(loggedInUserP **LoggedInUser, apiErrorP **cigExchange.APIError, activityType string) *cigExchange.APIError {

	activity, apiErr := convertToUserActivity(loggedInUserP, apiErrorP, activityType)
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
func UpdateUserActivity(loggedInUserP **LoggedInUser, apiErrorP **cigExchange.APIError, activityType string) *cigExchange.APIError {

	activity, apiErr := convertToUserActivity(loggedInUserP, apiErrorP, activityType)
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

func convertToUserActivity(loggedInUserP **LoggedInUser, apiErrorP **cigExchange.APIError, activityType string) (*models.UserActivity, *cigExchange.APIError) {

	loggedInUser := *loggedInUserP
	apiError := *apiErrorP

	activity := &models.UserActivity{}
	activity.Type = activityType

	// add jwt to user activity
	if loggedInUser == nil {
		activity.UserID = models.UnknownUser
	} else {
		activity.UserID = loggedInUser.UserUUID
		jsonBytes, err := json.Marshal(loggedInUser)
		if err != nil {
			apiErr := cigExchange.NewJSONEncodingError(cigExchange.MessageJSONEncoding, err)
			return activity, apiErr
		}

		activity.JWT = postgres.Jsonb{RawMessage: jsonBytes}
	}

	// add api error to user activity
	if apiError != nil {
		jsonBytes, err := json.Marshal(apiError)
		if err != nil {
			apiErr := cigExchange.NewJSONEncodingError(cigExchange.MessageJSONEncoding, err)
			return activity, apiErr
		}
		jsonStr := string(jsonBytes)
		activity.Info = &jsonStr
	}

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
func CreateCustomUserActivity(loggedInUserP **LoggedInUser, info map[string]interface{}) *cigExchange.APIError {

	loggedInUser := *loggedInUserP

	activity := &models.UserActivity{}

	// check 'type' field
	typeVal, ok := info["type"]
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

	if loggedInUser == nil {
		activity.UserID = models.UnknownUser
	} else {
		activity.UserID = loggedInUser.UserUUID
		jsonBytes, err := json.Marshal(loggedInUser)
		if err != nil {
			apiErr := cigExchange.NewJSONEncodingError(cigExchange.MessageJSONEncoding, err)
			fmt.Println(apiErr.ToString())
			return apiErr
		}

		activity.JWT = postgres.Jsonb{RawMessage: jsonBytes}
	}

	// add info to user activity
	jsonBytes, err := json.Marshal(info)
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

// PrepareActivityVariables creates pointers to pointers to use in defer
func PrepareActivityVariables() (**cigExchange.APIError, **LoggedInUser) {

	var apiErrorP **cigExchange.APIError
	var innerAPIError *cigExchange.APIError
	apiErrorP = &innerAPIError

	var loggedInUserP **LoggedInUser
	var innerLoggedInUser *LoggedInUser
	loggedInUserP = &innerLoggedInUser

	return apiErrorP, loggedInUserP
}

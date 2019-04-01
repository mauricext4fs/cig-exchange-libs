package models

import (
	cigExchange "cig-exchange-libs"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	uuid "github.com/satori/go.uuid"
)

// Constants defining the user status
const (
	UserStatusUnverified = "unverified"
	UserStatusVerified   = "active"
)

// User is a struct to represent a user
type User struct {
	ID             string     `json:"id" gorm:"column:id;primary_key"`
	Sex            string     `json:"sex" gorm:"column:sex"`
	Role           string     `json:"-" gorm:"column:role"`
	Name           string     `json:"name" gorm:"column:name"`
	LastName       string     `json:"lastname" gorm:"column:lastname"`
	LoginEmail     *Contact   `json:"-" gorm:"foreignkey:LoginEmailUUID;association_foreignkey:ID"`
	LoginEmailUUID *string    `json:"-" gorm:"column:login_email"`
	LoginPhone     *Contact   `json:"-" gorm:"foreignkey:LoginPhoneUUID;association_foreignkey:ID"`
	LoginPhoneUUID *string    `json:"-" gorm:"column:login_phone"`
	Verified       int64      `json:"-" gorm:"column:verified"`
	Status         string     `json:"-" gorm:"column:status;default:'unverified'"`
	CreatedAt      time.Time  `json:"-" gorm:"column:created_at"`
	UpdatedAt      time.Time  `json:"-" gorm:"column:updated_at"`
	DeletedAt      *time.Time `json:"-" gorm:"column:deleted_at"`
}

// TableName returns table name for struct
func (user *User) TableName() string {
	return "user"
}

// BeforeCreate generates new unique UUIDs for new db records
func (user *User) BeforeCreate(scope *gorm.Scope) error {

	UUID, err := uuid.NewV4()
	if err != nil {
		return err
	}
	scope.SetColumn("ID", UUID.String())

	return nil
}

// Create inserts new user object into db
func (user *User) Create(referenceKey string) *cigExchange.APIError {

	// invalidate the uuid
	user.ID = ""

	apiErr := user.trimFieldsAndValidate()
	if apiErr != nil {
		return apiErr
	}

	contacts := make([]Contact, 0)

	// check that email is unique
	db := cigExchange.GetDB().Where("value1 = ?", user.LoginEmail.Value1).Find(&contacts)
	if db.Error != nil {
		// we expect record not found error here
		if !db.RecordNotFound() {
			return cigExchange.NewDatabaseError("Contact lookup failed", db.Error)
		}
	} else {
		unverifiedUsers := make([]*User, 0)
		for _, contact := range contacts {
			// handle existing users
			existingUser := &User{}
			if cigExchange.GetDB().Model(contact).Related(existingUser, "LoginEmail").Error == nil {
				if existingUser.Status == UserStatusVerified {
					apiErr = &cigExchange.APIError{}
					apiErr.SetErrorType(cigExchange.ErrorTypeUnauthorized)
					apiErr.NewNestedError(cigExchange.ReasonUserAlreadyExists, "User already exists and is verified")
					return apiErr
				}
				// add to unverified users
				unverifiedUsers = append(unverifiedUsers, existingUser)
			}
		}

		for _, unverifiedUser := range unverifiedUsers {
			apiError := deleteUnverifiedUser(unverifiedUser)
			if apiError != nil {
				return apiError
			}
		}
	}

	org := &Organisation{}
	// verify organisation reference key if present
	if len(referenceKey) > 0 {

		orgWhere := &Organisation{
			ReferenceKey: referenceKey,
		}
		db = cigExchange.GetDB().Where(orgWhere).First(org)
		if db.Error != nil {
			// handle wrong reference key error and database error separately
			if db.RecordNotFound() {
				apiErr = cigExchange.NewInvalidFieldError("reference_key", "Organisation reference key is invalid")
			} else {
				apiErr = cigExchange.NewDatabaseError("Organization lookup failed", db.Error)
			}
			return apiErr
		}
	}

	err := cigExchange.GetDB().Create(user).Error
	if err != nil {
		return cigExchange.NewDatabaseError("Create user call failed", err)
	}

	apiError := createUserContacts(user)
	if apiError != nil {
		return apiError
	}

	// create organisation link for the user if necessary
	if len(referenceKey) > 0 {
		orgUser := &OrganisationUser{
			UserID:           user.ID,
			OrganisationID:   org.ID,
			IsHome:           true,
			OrganisationRole: "",
			Status:           OrganisationUserStatusUnverified,
		}
		apiErr := orgUser.Create()
		if apiErr != nil {
			return apiErr
		}
	}

	return nil
}

// Save writes the user object changes into db
func (user *User) Save() *cigExchange.APIError {

	err := cigExchange.GetDB().Save(user).Error
	if err != nil {
		return cigExchange.NewDatabaseError("Save user call failed", err)
	}
	return nil
}

// CreateInvitedUser creates new user or invites existing
func CreateInvitedUser(user *User, organisation *Organisation) (*User, *cigExchange.APIError) {

	// invalidate the uuid
	user.ID = ""

	apiErr := user.trimFieldsAndValidate()
	if apiErr != nil {
		return nil, apiErr
	}

	existingUser := &User{}

	needToCreateUser := false

	contacts := make([]Contact, 0)

	// check that email is unique
	db := cigExchange.GetDB().Where("value1 = ?", user.LoginEmail.Value1).Find(&contacts)
	if db.Error != nil {
		// we expect record not found error here
		if !db.RecordNotFound() {
			return nil, cigExchange.NewDatabaseError("Contact lookup failed", db.Error)
		}

		// proceed to create organisation user
		needToCreateUser = true
	} else {
		unverifiedUsers := make([]*User, 0)
		isVerified := false
		for _, contact := range contacts {
			// handle existing contacts
			existingUser = &User{}
			db = cigExchange.GetDB().Model(contact).Related(existingUser, "LoginEmail")
			if db.Error != nil {
				if !db.RecordNotFound() {
					return nil, cigExchange.NewDatabaseError("User lookup failed", db.Error)
				}
				// ignoring contacts without user
			} else {
				if existingUser.Status == UserStatusVerified {
					// found verified user
					isVerified = true
					break
				}
				// add to unverified users
				unverifiedUsers = append(unverifiedUsers, existingUser)
			}
		}

		if isVerified {
			// prefill the uuid
			orgUserWhere := &OrganisationUser{
				UserID:         existingUser.ID,
				OrganisationID: organisation.ID,
			}
			existedOrgUser := &OrganisationUser{}
			// find organization user connections
			db := cigExchange.GetDB().Where(orgUserWhere).First(existedOrgUser)
			if db.Error != nil {
				if !db.RecordNotFound() {
					return nil, cigExchange.NewDatabaseError("OrganisationUser lookup failed", db.Error)
				}
				// proceed to create organisation user
			} else {
				if existedOrgUser.Status == OrganisationUserStatusInvited {
					return nil, cigExchange.NewInvalidFieldError("email", "User already invited")
				}
				return nil, cigExchange.NewInvalidFieldError("email", "User already belongs to organisation")
			}
		} else { // only unverified users
			// delete all users
			for _, unverifiedUser := range unverifiedUsers {
				apiError := deleteUnverifiedUser(unverifiedUser)
				if apiError != nil {
					return nil, apiError
				}
			}

			needToCreateUser = true
		}
	}

	if needToCreateUser {
		err := cigExchange.GetDB().Create(user).Error
		if err != nil {
			return nil, cigExchange.NewDatabaseError("Create user call failed", err)
		}
		existingUser = user

		apiError := createUserContacts(user)
		if apiError != nil {
			return nil, apiError
		}
	}

	// create organisation link for the user
	orgUser := &OrganisationUser{
		UserID:           existingUser.ID,
		OrganisationID:   organisation.ID,
		Status:           OrganisationUserStatusInvited,
		IsHome:           true,
		OrganisationRole: "user",
	}
	apiErr = orgUser.Create()
	if apiErr != nil {
		return nil, apiErr
	}

	return existingUser, nil
}

// Update existing user object in db
func (user *User) Update(update map[string]interface{}) *cigExchange.APIError {

	// check that UUID is set
	if _, ok := update["id"]; !ok || len(user.ID) == 0 {
		return cigExchange.NewInvalidFieldError("user_id", "User UUID is not set")
	}

	db := cigExchange.GetDB().Model(user).Updates(update)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Failed to update user ", db.Error)
	}
	return nil
}

// deleteUnverifiedUser deletes user, contacts, userContact, organisationUser
func deleteUnverifiedUser(user *User) *cigExchange.APIError {

	// prefill the uuid
	orgUserWhere := &OrganisationUser{
		UserID: user.ID,
	}
	userContactWhere := &UserContact{
		UserID: user.ID,
	}
	contactWhere := &Contact{
		ID: *user.LoginEmailUUID,
	}

	// delete contact
	err := cigExchange.GetDB().Where(contactWhere).Delete(Contact{}).Error
	if err != nil {
		return cigExchange.NewDatabaseError("Delete contact call failed", err)
	}

	// delete user contact connections
	err = cigExchange.GetDB().Where(userContactWhere).Delete(UserContact{}).Error
	if err != nil {
		return cigExchange.NewDatabaseError("Delete user contact links call failed", err)
	}

	// delete unverified user
	err = cigExchange.GetDB().Delete(user).Error
	if err != nil {
		return cigExchange.NewDatabaseError("Delete user call failed", err)
	}

	// delete organization user connections
	err = cigExchange.GetDB().Where(orgUserWhere).Delete(OrganisationUser{}).Error
	if err != nil {
		return cigExchange.NewDatabaseError("Delete organization user links call failed", err)
	}

	return nil
}

func createUserContacts(user *User) *cigExchange.APIError {

	if user.LoginEmailUUID != nil && len(*user.LoginEmailUUID) > 0 {
		// create email UserContact
		userContact := &UserContact{
			UserID:    user.ID,
			ContactID: *user.LoginEmailUUID,
		}

		err := cigExchange.GetDB().Create(userContact).Error
		if err != nil {
			return cigExchange.NewDatabaseError("Create user contact link failed", err)
		}
	}

	if user.LoginPhoneUUID != nil && len(*user.LoginPhoneUUID) > 0 {
		// create phone UserContact
		userContact := &UserContact{
			UserID:    user.ID,
			ContactID: *user.LoginPhoneUUID,
		}

		err := cigExchange.GetDB().Create(userContact).Error
		if err != nil {
			return cigExchange.NewDatabaseError("Create user contact link failed", err)
		}
	}

	return nil
}

// GetUser queries a single user from db
func GetUser(UUID string) (user *User, apiErr *cigExchange.APIError) {

	user = &User{}
	userWhere := &User{
		ID: strings.TrimSpace(UUID),
	}
	if len(userWhere.ID) == 0 {
		apiErr = cigExchange.NewRequiredFieldError([]string{"uuid"})
		return
	}

	db := cigExchange.GetDB().Preload("LoginEmail").Preload("LoginPhone").Where(userWhere).First(user)
	if db.Error != nil {
		if db.RecordNotFound() {
			apiErr = cigExchange.NewUserDoesntExistError("User with provided uuid doesn't exist")
		} else {
			apiErr = cigExchange.NewDatabaseError("User lookup failed", db.Error)
		}
		return
	}
	return
}

// GetUserByEmail queries a single user from db
func GetUserByEmail(email string) (user *User, apiErr *cigExchange.APIError) {

	cont := &Contact{}
	contWhere := &Contact{
		Value1: strings.TrimSpace(email),
	}
	if len(contWhere.Value1) == 0 {
		apiErr = cigExchange.NewRequiredFieldError([]string{"email"})
		return
	}

	db := cigExchange.GetDB().Where(contWhere).First(cont)
	if db.Error != nil {
		if db.RecordNotFound() {
			apiErr = cigExchange.NewUserDoesntExistError("Contact with provided email doesn't exist")
		} else {
			apiErr = cigExchange.NewDatabaseError("Contact lookup failed", db.Error)
		}
		return
	}

	user = &User{}
	db = cigExchange.GetDB().Model(cont).Preload("LoginEmail").Preload("LoginPhone").Related(user, "LoginEmail")
	if db.Error != nil {
		if db.RecordNotFound() {
			apiErr = cigExchange.NewUserDoesntExistError("User with provided email doesn't exist")
		} else {
			apiErr = cigExchange.NewDatabaseError("User lookup failed", db.Error)
		}
		return
	}

	return
}

// GetUserByMobile queries a single user from db
func GetUserByMobile(code, number string) (user *User, apiErr *cigExchange.APIError) {

	cont := &Contact{}
	contWhere := &Contact{
		Value1: strings.TrimSpace(code),
		Value2: strings.TrimSpace(number),
	}

	missingFieldNames := make([]string, 0)
	if len(contWhere.Value1) == 0 {
		missingFieldNames = append(missingFieldNames, "phone_country_code")
	}
	if len(contWhere.Value2) == 0 {
		missingFieldNames = append(missingFieldNames, "phone_number")
	}
	if len(missingFieldNames) > 0 {
		apiErr = cigExchange.NewRequiredFieldError(missingFieldNames)
		return
	}

	db := cigExchange.GetDB().Where(contWhere).First(cont)
	if db.Error != nil {
		if db.RecordNotFound() {
			apiErr = cigExchange.NewUserDoesntExistError("User with provided phone number doesn't exist")
		} else {
			apiErr = cigExchange.NewDatabaseError("Contact lookup failed", db.Error)
		}
		return
	}

	user = &User{}
	db = cigExchange.GetDB().Model(cont).Preload("LoginEmail").Preload("LoginPhone").Related(user, "LoginPhone")
	if db.Error != nil {
		if db.RecordNotFound() {
			apiErr = cigExchange.NewUserDoesntExistError("User with provided phone number doesn't exist")
		} else {
			apiErr = cigExchange.NewDatabaseError("User lookup failed", db.Error)
		}
		return
	}

	return
}

func (user *User) trimFieldsAndValidate() *cigExchange.APIError {

	user.Name = strings.TrimSpace(user.Name)
	user.LastName = strings.TrimSpace(user.LastName)
	user.LoginEmail.Value1 = strings.TrimSpace(user.LoginEmail.Value1)
	user.LoginPhone.Value1 = strings.TrimSpace(user.LoginPhone.Value1)
	user.LoginPhone.Value2 = strings.TrimSpace(user.LoginPhone.Value2)

	missingFieldNames := make([]string, 0)
	if len(user.Name) == 0 {
		missingFieldNames = append(missingFieldNames, "name")
	}
	if len(user.LastName) == 0 {
		missingFieldNames = append(missingFieldNames, "lastname")
	}
	if len(user.LoginEmail.Value1) == 0 {
		missingFieldNames = append(missingFieldNames, "email")
	}
	if len(user.LoginPhone.Value1) == 0 {
		missingFieldNames = append(missingFieldNames, "phone_country_code")
	}
	if len(user.LoginPhone.Value2) == 0 {
		missingFieldNames = append(missingFieldNames, "phone_number")
	}
	if len(missingFieldNames) > 0 {
		return cigExchange.NewRequiredFieldError(missingFieldNames)
	}

	if !strings.Contains(user.LoginEmail.Value1, "@") {
		return cigExchange.NewInvalidFieldError("email", "Invalid email address")
	}

	return nil
}

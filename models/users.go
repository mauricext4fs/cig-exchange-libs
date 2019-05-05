package models

import (
	cigExchange "cig-exchange-libs"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
)

// Constants defining the user status
const (
	UserStatusUnverified = "unverified"
	UserStatusVerified   = "active"
)

// Constants defining the user role
const (
	UserRoleAdmin = "admin"
	UserRoleUser  = "regular-p2p-user"
)

// User is a struct to represent a user
type User struct {
	ID             string     `json:"id" gorm:"column:id;primary_key"`
	Title          string     `json:"title" gorm:"column:title"`
	Role           string     `json:"-" gorm:"column:role;default:'regular-p2p-user'"`
	Name           string     `json:"name" gorm:"column:name"`
	LastName       string     `json:"lastname" gorm:"column:lastname"`
	LoginEmail     *Contact   `json:"-" gorm:"foreignkey:LoginEmailUUID;association_foreignkey:ID"`
	LoginEmailUUID *string    `json:"-" gorm:"column:login_email"`
	LoginPhone     *Contact   `json:"-" gorm:"foreignkey:LoginPhoneUUID;association_foreignkey:ID"`
	LoginPhoneUUID *string    `json:"-" gorm:"column:login_phone"`
	Info           *Info      `json:"-" gorm:"foreignkey:InfoUUID;association_foreignkey:ID"`
	InfoUUID       *string    `json:"-" gorm:"column:info"`
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

	scope.SetColumn("ID", cigExchange.RandomUUID())
	return nil
}

// GetMultilangFields returns jsonb fields
func (*User) GetMultilangFields() []string {

	return []string{}
}

// CreateUser inserts new user object into db
func CreateUser(user *User, referenceKey string) (*User, *cigExchange.APIError) {

	// invalidate the uuid
	user.ID = ""

	apiErr := user.TrimFieldsAndValidate()
	if apiErr != nil {
		return nil, apiErr
	}

	org := &Organisation{}
	// verify organisation reference key if present
	if len(referenceKey) > 0 {

		orgWhere := &Organisation{
			ReferenceKey: referenceKey,
		}
		db := cigExchange.GetDB().Where(orgWhere).First(org)
		if db.Error != nil {
			// handle wrong reference key error and database error separately
			if db.RecordNotFound() {
				apiErr = cigExchange.NewInvalidFieldError("reference_key", "Organisation reference key is invalid")
			} else {
				apiErr = cigExchange.NewDatabaseError("Organization lookup failed", db.Error)
			}
			return nil, apiErr
		}
	}

	contacts := make([]Contact, 0)

	// check that email is unique
	db := cigExchange.GetDB().Where("value1 = ?", user.LoginEmail.Value1).Find(&contacts)
	if db.Error != nil {
		// we expect record not found error here
		if !db.RecordNotFound() {
			return nil, cigExchange.NewDatabaseError("Contact lookup failed", db.Error)
		}
	} else {
		unverifiedUsers := make([]*User, 0)
		for _, contact := range contacts {
			// handle existing users
			existingUser := &User{}
			if cigExchange.GetDB().Model(contact).Related(existingUser, "LoginEmail").Error == nil {
				if existingUser.Status == UserStatusVerified {
					// return real user

					// create organisation link for the user if necessary
					if len(referenceKey) > 0 {
						// check existing link to organisation
						_, apiError := GetOrgUserRole(existingUser.ID, org.ID)
						if apiError != nil {
							// user don't belong to organisation
							orgUser := &OrganisationUser{
								UserID:           existingUser.ID,
								OrganisationID:   org.ID,
								IsHome:           false,
								OrganisationRole: OrganisationRoleUser,
								Status:           OrganisationUserStatusUnverified,
							}
							apiErr := orgUser.Create()
							if apiErr != nil {
								return nil, apiErr
							}
						}
					}

					return existingUser, nil
				}
				// add to unverified users
				unverifiedUsers = append(unverifiedUsers, existingUser)
			}
		}

		for _, unverifiedUser := range unverifiedUsers {
			apiError := DeleteUnverifiedUser(unverifiedUser)
			if apiError != nil {
				return nil, apiError
			}
		}
	}

	// create new user
	err := cigExchange.GetDB().Create(user).Error
	if err != nil {
		return nil, cigExchange.NewDatabaseError("Create user call failed", err)
	}

	// create user contacts links
	apiError := createUserContacts(user)
	if apiError != nil {
		return nil, apiError
	}

	// create organisation link for the user if necessary
	if len(referenceKey) > 0 {
		orgUser := &OrganisationUser{
			UserID:           user.ID,
			OrganisationID:   org.ID,
			IsHome:           false,
			OrganisationRole: OrganisationRoleUser,
			Status:           OrganisationUserStatusUnverified,
		}
		apiErr := orgUser.Create()
		if apiErr != nil {
			return nil, apiErr
		}
	}

	return user, nil
}

// Save writes the user object changes into db
func (user *User) Save() *cigExchange.APIError {

	err := cigExchange.GetDB().Save(user).Error
	if err != nil {
		return cigExchange.NewDatabaseError("Save user call failed", err)
	}
	return nil
}

// Update existing user object in db
func (user *User) Update(update map[string]interface{}) *cigExchange.APIError {

	// check that UUID is set
	if _, ok := update["id"]; !ok {
		return cigExchange.NewInvalidFieldError("user_id", "User UUID is not set")
	}

	db := cigExchange.GetDB().Model(user).Updates(update)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Failed to update user ", db.Error)
	}
	return nil
}

// HasUserHomeOrganisation checks for user home organisation in db
func (user *User) HasUserHomeOrganisation() (bool, *cigExchange.APIError) {

	// query organisationUser
	orgUserWhere := &OrganisationUser{
		UserID: user.ID,
		IsHome: true,
	}
	orgUser := &OrganisationUser{}

	// delete contact
	db := cigExchange.GetDB().Where(orgUserWhere).First(&orgUser)
	if db.Error != nil {
		// user hasn't any home organisation
		if db.RecordNotFound() {
			return false, nil
		}
		return false, cigExchange.NewDatabaseError("Lookup home organisation call failed", db.Error)
	}

	// user has home organisation
	return true, nil
}

// DeleteUnverifiedUser deletes user, contacts, userContact, organisationUser
func DeleteUnverifiedUser(user *User) *cigExchange.APIError {

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
// Fucntions can return (nil, nil) if ignoreRecordNotFound is true
func GetUserByEmail(email string, ignoreRecordNotFound bool) (user *User, apiErr *cigExchange.APIError) {

	contWhere := &Contact{
		Value1: strings.TrimSpace(email),
	}
	// check email length
	if len(contWhere.Value1) == 0 {
		apiErr = cigExchange.NewRequiredFieldError([]string{"email"})
		return
	}

	user = nil

	// query all contacts
	conts := make([]*Contact, 0)
	db := cigExchange.GetDB().Where(contWhere).Find(&conts)
	if db.Error != nil {
		if db.RecordNotFound() {
			if ignoreRecordNotFound {
				return nil, nil
			}
			apiErr = cigExchange.NewUserDoesntExistError("Contact with provided email doesn't exist")
		} else {
			apiErr = cigExchange.NewDatabaseError("Contact lookup failed", db.Error)
		}
		return
	}

	for _, cont := range conts {
		u := &User{}
		db = cigExchange.GetDB().Model(cont).Preload("LoginEmail").Preload("LoginPhone").Related(u, "LoginEmail")
		if db.Error != nil {
			// ignore contacts
			if !db.RecordNotFound() {
				apiErr = cigExchange.NewUserDoesntExistError("User with provided email doesn't exist")
			}
		} else {
			user = u
		}
	}

	if user == nil && !ignoreRecordNotFound {
		apiErr = cigExchange.NewUserDoesntExistError("User lookup failed")
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

// GetUserRole returns user role
func GetUserRole(userUUID string) (role string, apiError *cigExchange.APIError) {

	// check user id
	if len(userUUID) == 0 {
		return "", cigExchange.NewInvalidFieldError("user_id", "UserID is invalid")
	}

	// get user
	user, apiErr := GetUser(userUUID)
	if apiErr != nil {
		return "", apiErr
	}

	return user.Role, nil
}

// GetOrgUserRole returns user role in organisation
func GetOrgUserRole(userUUID, organisationUUID string) (role string, apiError *cigExchange.APIError) {

	// check user id
	if len(userUUID) == 0 {
		return "", cigExchange.NewInvalidFieldError("user_id", "UserID is invalid")
	}

	// check organisation id
	if len(organisationUUID) == 0 {
		return "", cigExchange.NewInvalidFieldError("organization_id", "OrganisationID is invalid")
	}

	// get role in organisation
	orgUserWhere := &OrganisationUser{
		OrganisationID: organisationUUID,
		UserID:         userUUID,
	}
	orgUser, apiErr := orgUserWhere.Find()
	if apiErr != nil {
		return "", apiErr
	}

	return orgUser.OrganisationRole, nil
}

// TrimFieldsAndValidate checks user for invalid fields
func (user *User) TrimFieldsAndValidate() *cigExchange.APIError {

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

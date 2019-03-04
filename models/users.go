package models

import (
	"cig-exchange-libs"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	uuid "github.com/satori/go.uuid"
)

// User is a struct to represent a user
type User struct {
	ID             string     `gorm:"column:id;primary_key"`
	Sex            string     `gorm:"column:sex"`
	Role           string     `gorm:"column:role"`
	Name           string     `gorm:"column:name"`
	LastName       string     `gorm:"column:lastname"`
	LoginEmail     *Contact   `gorm:"foreignkey:LoginEmailUUID;association_foreignkey:ID"`
	LoginEmailUUID *string    `gorm:"column:login_email"`
	LoginPhone     *Contact   `gorm:"foreignkey:LoginPhoneUUID;association_foreignkey:ID"`
	LoginPhoneUUID *string    `gorm:"column:login_phone"`
	Verified       int64      `gorm:"column:verified"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
	DeletedAt      *time.Time `gorm:"column:deleted_at"`
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

	temp := &Contact{}

	// check that email is unique
	db := cigExchange.GetDB().Where("value1 = ?", user.LoginEmail.Value1).First(temp)
	if db.Error != nil {
		// we expect record not found error here
		if !db.RecordNotFound() {
			return cigExchange.NewGormError("Contact lookup failed", db.Error)
		}
	} else {
		existingUser := &User{}
		if cigExchange.GetDB().Model(temp).Related(existingUser, "LoginEmail").Error == nil {
			if existingUser.Verified > 0 {
				apiErr = &cigExchange.APIError{}
				apiErr.SetErrorType(cigExchange.ErrorTypeUnauthorized)

				nesetedError := apiErr.NewNestedError()
				nesetedError.Reason = cigExchange.NestedErrorUserAlreadyExists
				nesetedError.Message = "User already exists and is verified"
				return apiErr
			}

			// prefill the uuid
			orgUserWhere := &OrganisationUser{
				UserID: existingUser.ID,
			}

			// delete unverified user
			err := cigExchange.GetDB().Delete(existingUser).Error
			if err != nil {
				return cigExchange.NewGormError("Delete user call failed", err)
			}

			// delete organization user connections
			err = cigExchange.GetDB().Where(orgUserWhere).Delete(OrganisationUser{}).Error
			if err != nil {
				return cigExchange.NewGormError("Delete organization user links call failed", err)
			}
		}
		// reuse the contact
		user.LoginEmail = nil
		user.LoginEmailUUID = &temp.ID
	}

	// Remove the phone contact for now
	user.LoginPhone = nil
	user.LoginPhoneUUID = nil
	/*  Phone is disabled for now
	// check that mobile is unique
	db = cigExchange.GetDB().Where("value1 = ? AND value2 = ?", user.LoginPhone.Value1, user.LoginPhone.Value2).First(temp)
	if db.Error != nil {
		// we expect record not found error here
		if !db.RecordNotFound() {
			return fmt.Errorf("Database error: %s", db.Error.Error())
		}
	} else {
		return fmt.Errorf("Mobile already in use by another user")
	}
	*/

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
				apiErr = cigExchange.NewGormError("Organization lookup failed", db.Error)
			}
			return apiErr
		}
	}

	err := cigExchange.GetDB().Create(user).Error
	if err != nil {
		return cigExchange.NewGormError("Create user call failed", err)
	}

	// create organisation link for the user if necessary
	if len(referenceKey) > 0 {
		orgUser := &OrganisationUser{
			UserID:         user.ID,
			OrganisationID: org.ID,
		}
		err = cigExchange.GetDB().Create(orgUser).Error
		if err != nil {
			return cigExchange.NewGormError("Create organization user link call failed", err)
		}
	}

	return nil
}

// Save writes the user object changes into db
func (user *User) Save() *cigExchange.APIError {

	err := cigExchange.GetDB().Save(user).Error
	if err != nil {
		return cigExchange.NewGormError("Save user call failed", err)
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
			apiErr = cigExchange.NewGormError("User lookup failed", db.Error)
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
			apiErr = cigExchange.NewUserDoesntExistError("User with provided email doesn't exist")
		} else {
			apiErr = cigExchange.NewGormError("Contact lookup failed", db.Error)
		}
		return
	}

	user = &User{}
	db = cigExchange.GetDB().Model(cont).Preload("LoginEmail").Preload("LoginPhone").Related(user, "LoginEmail")
	if db.Error != nil {
		if db.RecordNotFound() {
			apiErr = cigExchange.NewUserDoesntExistError("User with provided email doesn't exist")
		} else {
			apiErr = cigExchange.NewGormError("User lookup failed", db.Error)
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
			apiErr = cigExchange.NewGormError("Contact lookup failed", db.Error)
		}
		return
	}

	user = &User{}
	db = cigExchange.GetDB().Model(cont).Preload("LoginEmail").Preload("LoginPhone").Related(user, "LoginPhone")
	if db.Error != nil {
		if db.RecordNotFound() {
			apiErr = cigExchange.NewUserDoesntExistError("User with provided phone number doesn't exist")
		} else {
			apiErr = cigExchange.NewGormError("User lookup failed", db.Error)
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

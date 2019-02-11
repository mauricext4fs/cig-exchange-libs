package models

import (
	"cig-exchange-libs"
	"fmt"
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
func (user *User) Create(referenceKey string) error {

	// invalidate the uuid
	user.ID = ""

	user.trimFields()

	reqError := fmt.Errorf("Required field validation failed: %#v", user)
	if len(user.Name) == 0 {
		return reqError
	} else if len(user.LastName) == 0 {
		return reqError
	} else if len(user.LoginEmail.Value1) == 0 {
		return reqError
	} else if len(user.LoginPhone.Value1) == 0 {
		return reqError
	} else if len(user.LoginPhone.Value2) == 0 {
		return reqError
	} else if !strings.Contains(user.LoginEmail.Value1, "@") {
		return reqError
	}

	temp := &Contact{}

	// check that email is unique
	db := cigExchange.GetDB().Where("value1 = ?", user.LoginEmail.Value1).First(temp)
	if db.Error != nil {
		// we expect record not found error here
		if !db.RecordNotFound() {
			return fmt.Errorf("Database error: %s", db.Error.Error())
		}
	} else {
		existingUser := &User{}
		if cigExchange.GetDB().Model(temp).Related(existingUser, "LoginEmail").Error == nil {
			if existingUser.Verified > 0 {
				return fmt.Errorf("Email already in use by another user")
			}

			// prefill the uuid
			orgUserWhere := &OrganisationUser{
				UserID: existingUser.ID,
			}

			// delete unverified user
			err := cigExchange.GetDB().Delete(existingUser).Error
			if err != nil {
				return fmt.Errorf("Database error: %s", err.Error())
			}

			// delete organization user connections
			err = cigExchange.GetDB().Where(orgUserWhere).Delete(OrganisationUser{}).Error
			if err != nil {
				return fmt.Errorf("Database error: %s", err.Error())
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
		err := cigExchange.GetDB().Where(orgWhere).First(org).Error
		if err != nil {
			return fmt.Errorf("Database error when loading organisation: %s", err.Error())
		}
	}

	err := cigExchange.GetDB().Create(user).Error
	if err != nil {
		return err
	}

	// create organisation link for the user if necessary
	if len(referenceKey) > 0 {
		orgUser := &OrganisationUser{
			UserID:         user.ID,
			OrganisationID: org.ID,
		}
		return cigExchange.GetDB().Create(orgUser).Error
	}

	return nil
}

// Save writes the user object changes into db
func (user *User) Save() error {
	return cigExchange.GetDB().Save(user).Error
}

// GetUser queries a single user from db
func GetUser(UUID string) (user *User, err error) {

	user = &User{}
	userWhere := &User{
		ID: strings.TrimSpace(UUID),
	}
	if len(userWhere.ID) == 0 {
		err = fmt.Errorf("GetUser: empty search criteria")
		return
	}
	err = cigExchange.GetDB().Preload("LoginEmail").Preload("LoginPhone").Where(userWhere).First(user).Error

	return
}

// GetUserByEmail queries a single user from db
func GetUserByEmail(email string) (user *User, err error) {

	cont := &Contact{}
	contWhere := &Contact{
		Value1: strings.TrimSpace(email),
	}
	if len(contWhere.Value1) == 0 {
		err = fmt.Errorf("GetUserByEmail: empty search criteria")
		return
	}

	err = cigExchange.GetDB().Where(contWhere).First(cont).Error
	if err != nil {
		return
	}

	user = &User{}
	err = cigExchange.GetDB().Model(cont).Preload("LoginEmail").Preload("LoginPhone").Related(user, "LoginEmail").Error

	return
}

// GetUserByMobile queries a single user from db
func GetUserByMobile(code, number string) (user *User, err error) {

	cont := &Contact{}
	contWhere := &Contact{
		Value1: strings.TrimSpace(code),
		Value2: strings.TrimSpace(number),
	}
	if len(contWhere.Value1) == 0 || len(contWhere.Value2) == 0 {
		err = fmt.Errorf("GetUserByMobile: empty search criteria")
		return
	}

	err = cigExchange.GetDB().Where(contWhere).First(cont).Error
	if err != nil {
		return
	}

	user = &User{}
	err = cigExchange.GetDB().Model(cont).Preload("LoginEmail").Preload("LoginPhone").Related(user, "LoginPhone").Error

	return
}

func (user *User) trimFields() {

	user.Name = strings.TrimSpace(user.Name)
	user.LastName = strings.TrimSpace(user.LastName)
	user.LoginEmail.Value1 = strings.TrimSpace(user.LoginEmail.Value1)
	user.LoginPhone.Value1 = strings.TrimSpace(user.LoginPhone.Value1)
	user.LoginPhone.Value2 = strings.TrimSpace(user.LoginPhone.Value2)
}

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
	LoginEmail     Contact    `gorm:"foreignkey:LoginEmailUUID"`
	LoginEmailUUID string     `gorm:"column:login_email"`
	LoginPhone     Contact    `gorm:"foreignkey:LoginPhoneUUID"`
	LoginPhoneUUID string     `gorm:"column:login_phone"`
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
func (user *User) Create() error {

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
		return fmt.Errorf("Email already in use by another user")
	}

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

	return cigExchange.GetDB().Create(user).Error
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
	userWhere := &User{
		LoginEmailUUID: cont.ID,
	}
	err = cigExchange.GetDB().Preload("LoginEmail").Preload("LoginPhone").Where(userWhere).First(user).Error

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
	userWhere := &User{
		LoginPhoneUUID: cont.ID,
	}
	err = cigExchange.GetDB().Preload("LoginEmail").Preload("LoginPhone").Where(userWhere).First(user).Error

	return
}

func (user *User) trimFields() {

	user.Name = strings.TrimSpace(user.Name)
	user.LastName = strings.TrimSpace(user.LastName)
	user.LoginEmail.Value1 = strings.TrimSpace(user.LoginEmail.Value1)
	user.LoginPhone.Value1 = strings.TrimSpace(user.LoginPhone.Value1)
	user.LoginPhone.Value2 = strings.TrimSpace(user.LoginPhone.Value2)
}

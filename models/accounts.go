package models

import (
	"cig-exchange-libs"
	"fmt"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	uuid "github.com/satori/go.uuid"
)

// Account is a struct to represent an offering
type Account struct {
	ID             string     `json:"id" gorm:"column:id;primary_key"`
	FirstName      string     `json:"first_name" gorm:"column:first_name"`
	LastName       string     `json:"last_name" gorm:"column:last_name"`
	ReferenceKey   string     `json:"reference_key" gorm:"column:reference_key"`
	Email          string     `json:"email" gorm:"column:email"`
	MobileCode     string     `json:"mobile_code" gorm:"column:mobile_code"`
	MobileNumber   string     `json:"mobile_number" gorm:"column:mobile_number"`
	VerifiedEmail  bool       `json:"-" gorm:"column:verified_email"`
	VerifiedMobile bool       `json:"-" gorm:"column:verified_mobile"`
	CreatedAt      time.Time  `json:"-" gorm:"column:created_at"`
	UpdatedAt      time.Time  `json:"-" gorm:"column:updated_at"`
	DeletedAt      *time.Time `json:"-" gorm:"column:deleted_at"`
}

// BeforeCreate generates new unique UUIDs for new db records
func (account *Account) BeforeCreate(scope *gorm.Scope) error {

	UUID, err := uuid.NewV4()
	if err != nil {
		return err
	}
	scope.SetColumn("ID", UUID.String())

	return nil
}

// Create inserts new account object into db
func (account *Account) Create() error {

	// invalidate the uuid
	account.ID = ""

	account.trimFields()

	reqError := fmt.Errorf("Required field validation failed: %#v", account)
	if len(account.FirstName) == 0 {
		return reqError
	} else if len(account.LastName) == 0 {
		return reqError
	} else if len(account.Email) == 0 {
		return reqError
	} else if len(account.ReferenceKey) == 0 {
		return reqError
	} else if len(account.MobileCode) == 0 {
		return reqError
	} else if len(account.MobileNumber) == 0 {
		return reqError
	} else if !strings.Contains(account.Email, "@") {
		return reqError
	}

	temp := &Account{}

	// check that email is unique
	db := cigExchange.GetDB().Where("email = ?", account.Email).First(temp)
	if db.Error != nil {
		// we expect record not found error here
		if !db.RecordNotFound() {
			return fmt.Errorf("Database error: %s", db.Error.Error())
		}
	} else {
		return fmt.Errorf("Email already in use by another user")
	}

	// check that mobile is unique
	db = cigExchange.GetDB().Where("mobile_code = ? AND mobile_number = ?", account.MobileCode, account.MobileNumber).First(temp)
	if db.Error != nil {
		// we expect record not found error here
		if !db.RecordNotFound() {
			return fmt.Errorf("Database error: %s", db.Error.Error())
		}
	} else {
		return fmt.Errorf("Mobile already in use by another user")
	}

	return cigExchange.GetDB().Create(account).Error
}

// GetAccount queries a single account from db
func GetAccount(UUID string) (account *Account, err error) {

	account = &Account{}
	accountWhere := &Account{
		ID: strings.TrimSpace(UUID),
	}
	if len(accountWhere.ID) == 0 {
		err = fmt.Errorf("GetAccount: empty search criteria")
		return
	}
	err = cigExchange.GetDB().Where(accountWhere).First(account).Error

	return
}

// GetAccountByEmail queries a single account from db
func GetAccountByEmail(email string) (account *Account, err error) {

	account = &Account{}
	accountWhere := &Account{
		Email: strings.TrimSpace(email),
	}
	if len(accountWhere.Email) == 0 {
		err = fmt.Errorf("GetAccountByEmail: empty search criteria")
		return
	}
	err = cigExchange.GetDB().Where(accountWhere).First(account).Error

	return
}

// GetAccountByMobile queries a single account from db
func GetAccountByMobile(code, number string) (account *Account, err error) {

	account = &Account{}
	accountWhere := &Account{
		MobileCode:   strings.TrimSpace(code),
		MobileNumber: strings.TrimSpace(number),
	}
	if len(accountWhere.MobileCode) == 0 || len(accountWhere.MobileNumber) == 0 {
		err = fmt.Errorf("GetAccountByMobile: empty search criteria")
		return
	}
	err = cigExchange.GetDB().Where(accountWhere).First(account).Error

	return
}

func (account *Account) trimFields() {

	account.FirstName = strings.TrimSpace(account.FirstName)
	account.LastName = strings.TrimSpace(account.LastName)
	account.Email = strings.TrimSpace(account.Email)
	account.ReferenceKey = strings.TrimSpace(account.ReferenceKey)
	account.MobileCode = strings.TrimSpace(account.MobileCode)
	account.MobileNumber = strings.TrimSpace(account.MobileNumber)
}

package models

import (
	"cig-exchange-libs"
	"github.com/dgrijalva/jwt-go"
	"github.com/jinzhu/gorm"
	"os"
	"strings"
)

/*
JWT claims struct
*/
type Token struct {
	UserId uint
	jwt.StandardClaims
}

//a struct to rep user account
type Account struct {
	gorm.Model
	Email string `json:"email"`
	Token string `json:"token" sql:"-"`
}

//Validate incoming user details...
func (account *Account) Validate() (map[string]interface{}, bool) {

	if !strings.Contains(account.Email, "@") {
		return cigExchange.Message(false, "Email address is required"), false
	}

	//Email must be unique
	temp := &Account{}

	//check for errors and duplicate emails
	err := cigExchange.GetDB().Table("accounts").Where("email = ?", account.Email).First(temp).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return cigExchange.Message(false, "Connection error. Please retry"), false
	}
	if temp.Email != "" {
		return cigExchange.Message(false, "Email address already in use by another user."), false
	}

	return cigExchange.Message(false, "Requirement passed"), true
}

func (account *Account) Create() map[string]interface{} {

	if resp, ok := account.Validate(); !ok {
		return resp
	}

	cigExchange.GetDB().Create(account)

	if account.ID <= 0 {
		return cigExchange.Message(false, "Failed to create account, connection error.")
	}

	//Create new JWT token for the newly registered account
	tk := &Token{UserId: account.ID}
	token := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), tk)
	tokenString, _ := token.SignedString([]byte(os.Getenv("token_password")))
	account.Token = tokenString

	response := cigExchange.Message(true, "Account has been created")
	response["account"] = account
	return response
}

func Login(email string) map[string]interface{} {

	account := &Account{}
	err := cigExchange.GetDB().Table("accounts").Where("email = ?", email).First(account).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return cigExchange.Message(false, "Email address not found")
		}
		return cigExchange.Message(false, "Connection error. Please retry")
	}

	//Worked! Logged In

	//Create JWT token
	tk := &Token{UserId: account.ID}
	token := jwt.NewWithClaims(jwt.GetSigningMethod("HS256"), tk)
	tokenString, _ := token.SignedString([]byte(os.Getenv("token_password")))
	account.Token = tokenString //Store the token in the response

	resp := cigExchange.Message(true, "Logged In")
	resp["account"] = account
	return resp
}

func GetUser(u uint) *Account {

	acc := &Account{}
	cigExchange.GetDB().Table("accounts").Where("id = ?", u).First(acc)
	if acc.Email == "" { //User not found!
		return nil
	}

	return acc
}

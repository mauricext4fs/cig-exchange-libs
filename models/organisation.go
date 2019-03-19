package models

import (
	cigExchange "cig-exchange-libs"
	"fmt"
	"time"

	"github.com/jinzhu/gorm"
	uuid "github.com/satori/go.uuid"
)

// Organisation is a struct to represent an organisation
type Organisation struct {
	ID           string     `gorm:"column:id;primary_key"`
	Type         string     `gorm:"column:type"`
	Name         string     `gorm:"column:name"`
	Website      string     `gorm:"column:website"`
	ReferenceKey string     `gorm:"column:reference_key"`
	Verified     int64      `gorm:"column:verified"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at"`
	DeletedAt    *time.Time `gorm:"column:deleted_at"`
}

// TableName returns table name for struct
func (*Organisation) TableName() string {
	return "organisation"
}

// BeforeCreate generates new unique UUIDs for new db records
func (*Organisation) BeforeCreate(scope *gorm.Scope) error {

	UUID, err := uuid.NewV4()
	if err != nil {
		return err
	}
	scope.SetColumn("ID", UUID.String())

	return nil
}

// OrganisationUser is a struct to represent an organisation to user link
type OrganisationUser struct {
	OrganisationID string     `gorm:"column:organisation_id"`
	UserID         string     `gorm:"column:user_id"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
	DeletedAt      *time.Time `gorm:"column:deleted_at"`
}

// TableName returns table name for struct
func (*OrganisationUser) TableName() string {
	return "organisation_user"
}

// Delete existing offering object in db
func (orgUser *OrganisationUser) Delete() *cigExchange.APIError {

	// check that both ID's are set
	if len(orgUser.UserID) == 0 {
		return cigExchange.NewInvalidFieldError("user_id", "UserID is invalid")
	}
	if len(orgUser.OrganisationID) == 0 {
		return cigExchange.NewInvalidFieldError("organization_id", "OrganisationID is invalid")
	}

	err := cigExchange.GetDB().Delete(orgUser).Error
	if err != nil {
		return cigExchange.NewDatabaseError("Error deleting organisation user", err)
	}
	return nil
}

// GetUsersForOrganisation queries all users for organisation from db
func GetUsersForOrganisation(organisationID string) (users []*User, apiErr *cigExchange.APIError) {

	var orgUsers []OrganisationUser

	// find all organisationUser objects for organisation
	cigExchange.GetDB().Where(&OrganisationUser{OrganisationID: organisationID}).Find(&orgUsers)

	for _, orgUser := range orgUsers {
		var user User
		db := cigExchange.GetDB().Where(&User{ID: orgUser.UserID}).First(&user)
		if db.Error != nil {
			if db.RecordNotFound() {
				apiErr = cigExchange.NewDatabaseError("User lookup failed", fmt.Errorf("User doesn't exist for organisation_user record"))
			} else {
				apiErr = cigExchange.NewDatabaseError("User lookup failed", db.Error)
			}
			return
		}
		// add user to response
		users = append(users, &user)
	}

	return
}

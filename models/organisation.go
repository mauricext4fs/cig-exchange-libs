package models

import (
	cigExchange "cig-exchange-libs"
	"fmt"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	uuid "github.com/satori/go.uuid"
)

// Organisation is a struct to represent an organisation
type Organisation struct {
	ID                        string     `json:"id" gorm:"column:id;primary_key"`
	Type                      string     `json:"type" gorm:"column:type"`
	Name                      string     `json:"name" gorm:"column:name"`
	Website                   string     `json:"website" gorm:"column:website"`
	MainContactPerson         string     `json:"main_contact_person" gorm:"column:main_contact_person"`
	ReferenceKey              string     `json:"reference_key" gorm:"column:reference_key"`
	OfferingRatingDescription string     `json:"offering_rating_description" gorm:"column:offering_rating_description"`
	Verified                  int64      `json:"verified" gorm:"column:verified"`
	CreatedAt                 time.Time  `json:"-" gorm:"column:created_at"`
	UpdatedAt                 time.Time  `json:"-" gorm:"column:updated_at"`
	DeletedAt                 *time.Time `json:"-" gorm:"column:deleted_at"`
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

// Create inserts new organisation object into db
func (organisation *Organisation) Create() *cigExchange.APIError {

	// invalidate the uuid
	organisation.ID = ""

	if apiErr := organisation.trimFieldsAndValidate(); apiErr != nil {
		return apiErr
	}

	db := cigExchange.GetDB().Create(organisation)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Failed to create organisation", db.Error)
	}
	return nil
}

// Update existing organisation object in db
func (organisation *Organisation) Update() *cigExchange.APIError {

	// check that UUID is set
	if len(organisation.ID) == 0 {
		return cigExchange.NewInvalidFieldError("organisation_id", "Invalid organisation id")
	}

	err := cigExchange.GetDB().Model(organisation).Updates(organisation).Error
	if err != nil {
		return cigExchange.NewDatabaseError("Failed to update organisation ", err)
	}
	return nil
}

// Delete existing organisation object in db
func (organisation *Organisation) Delete() *cigExchange.APIError {

	// check that UUID is set
	if len(organisation.ID) == 0 {
		return cigExchange.NewInvalidFieldError("organisation_id", "Invalid organisation id")
	}

	err := cigExchange.GetDB().Delete(organisation).Error
	if err != nil {
		return cigExchange.NewDatabaseError("Failed to delete organisation", err)
	}
	return nil
}

// GetOrganisation queries a single organisation from db
func GetOrganisation(UUID string) (*Organisation, *cigExchange.APIError) {

	// check that UUID is set
	if len(UUID) == 0 {
		return nil, cigExchange.NewInvalidFieldError("organisation_id", "Invalid organisation id")
	}

	organisation := &Organisation{
		ID: UUID,
	}
	db := cigExchange.GetDB().First(organisation)

	if db.Error != nil {
		if !db.RecordNotFound() {
			return nil, cigExchange.NewDatabaseError("Organisation lookup failed", db.Error)
		}
		return nil, cigExchange.NewOrganisationDoesntExistError("Organisation with provided uuid doesn't exist")
	}

	return organisation, nil
}

func (organisation *Organisation) trimFieldsAndValidate() *cigExchange.APIError {

	organisation.Name = strings.TrimSpace(organisation.Name)
	organisation.Type = strings.TrimSpace(organisation.Type)
	organisation.ReferenceKey = strings.TrimSpace(organisation.ReferenceKey)
	organisation.OfferingRatingDescription = strings.TrimSpace(organisation.OfferingRatingDescription)

	missingFieldNames := make([]string, 0)
	if len(organisation.Name) == 0 {
		missingFieldNames = append(missingFieldNames, "name")
	}
	if len(organisation.Type) == 0 {
		missingFieldNames = append(missingFieldNames, "type")
	}
	if len(organisation.ReferenceKey) == 0 {
		missingFieldNames = append(missingFieldNames, "reference_key")
	}

	if len(missingFieldNames) > 0 {
		return cigExchange.NewRequiredFieldError(missingFieldNames)
	}
	return nil
}

// OrganisationUser is a struct to represent an organisation to user link
type OrganisationUser struct {
	OrganisationID   string     `gorm:"column:organisation_id"`
	UserID           string     `gorm:"column:user_id"`
	OrganisationRole string     `gorm:"column:organisation_role"`
	IsHome           bool       `gorm:"column:is_home"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
	DeletedAt        *time.Time `gorm:"column:deleted_at"`
}

// TableName returns table name for struct
func (*OrganisationUser) TableName() string {
	return "organisation_user"
}

// Create inserts new user organisation object into db
func (orgUser *OrganisationUser) Create() *cigExchange.APIError {

	// check that both ID's are set
	if len(orgUser.UserID) == 0 {
		return cigExchange.NewInvalidFieldError("user_id", "UserID is invalid")
	}
	if len(orgUser.OrganisationID) == 0 {
		return cigExchange.NewInvalidFieldError("organization_id", "OrganisationID is invalid")
	}

	db := cigExchange.GetDB().Create(orgUser)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Failed to create organisation user", db.Error)
	}
	return nil
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

// GetOrganisationUsersForOrganisation queries all organisation users for organisation from db
func GetOrganisationUsersForOrganisation(organisationID string) (orgUsers []*OrganisationUser, apiErr *cigExchange.APIError) {

	// find all organisationUser objects for organisation
	db := cigExchange.GetDB().Where(&OrganisationUser{OrganisationID: organisationID}).Find(&orgUsers)
	if db.Error != nil {
		if !db.RecordNotFound() {
			apiErr = cigExchange.NewDatabaseError("Organisation Users lookup failed", db.Error)
		}
	}
	return
}

// GetUsersForOrganisation queries all users for organisation from db
func GetUsersForOrganisation(organisationID string) (users []*User, apiErr *cigExchange.APIError) {

	var orgUsers []OrganisationUser

	// find all organisationUser objects for organisation
	db := cigExchange.GetDB().Where(&OrganisationUser{OrganisationID: organisationID}).Find(&orgUsers)
	if db.Error != nil {
		if !db.RecordNotFound() {
			apiErr = cigExchange.NewDatabaseError("Organisation Users lookup failed", db.Error)
		}
		return
	}

	for _, orgUser := range orgUsers {
		var user User
		db = cigExchange.GetDB().Where(&User{ID: orgUser.UserID}).First(&user)
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

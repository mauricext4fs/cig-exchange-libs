package models

import (
	cigExchange "cig-exchange-libs"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	uuid "github.com/satori/go.uuid"
)

// Constants defining the user role in organisation
const (
	OrganisationRoleAdmin = "admin"
	OrganisationRoleUser  = "user"
)

// Organisation is a struct to represent an organisation
type Organisation struct {
	ID                        string     `json:"id" gorm:"column:id;primary_key"`
	Type                      string     `json:"type" gorm:"column:type"`
	Name                      string     `json:"name" gorm:"column:name"`
	Website                   string     `json:"website" gorm:"column:website"`
	ReferenceKey              string     `json:"reference_key" gorm:"column:reference_key"`
	OfferingRatingDescription string     `json:"offering_rating_description" gorm:"column:offering_rating_description"`
	Verified                  int64      `json:"verified" gorm:"column:verified"`
	CreatedAt                 time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt                 time.Time  `json:"updated_at" gorm:"column:updated_at"`
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
func (organisation *Organisation) Update(update map[string]interface{}) *cigExchange.APIError {

	// check that UUID is set
	if _, ok := update["id"]; !ok || len(organisation.ID) == 0 {
		return cigExchange.NewInvalidFieldError("organisation_id", "Invalid organisation id")
	}

	err := cigExchange.GetDB().Model(organisation).Updates(update).Error
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

	db := cigExchange.GetDB().Delete(organisation)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Failed to delete organisation", db.Error)
	}
	if db.RowsAffected == 0 {
		return cigExchange.NewInvalidFieldError("organisation_id", "Organisation with provided id doesn't exist")
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

// GetOrganisations queries all organisations for user from db
func GetOrganisations(userUUID string) ([]*Organisation, *cigExchange.APIError) {

	// check that UUID is set
	if len(userUUID) == 0 {
		return nil, cigExchange.NewInvalidFieldError("user_id", "Invalid user id")
	}

	organisations := make([]*Organisation, 0)

	var orgUsers []OrganisationUser

	// find all organisationUser objects for user
	db := cigExchange.GetDB().Where(&OrganisationUser{UserID: userUUID}).Find(&orgUsers)
	if db.Error != nil {
		if !db.RecordNotFound() {
			return nil, cigExchange.NewDatabaseError("OrganisationUser lookup failed", db.Error)
		}
	}

	for _, orgUser := range orgUsers {
		organisation := &Organisation{
			ID: orgUser.OrganisationID,
		}
		db := cigExchange.GetDB().First(organisation)
		if db.Error != nil {
			if !db.RecordNotFound() {
				return nil, cigExchange.NewDatabaseError("Organisation lookup failed", db.Error)
			}
		}
		organisations = append(organisations, organisation)
	}

	return organisations, nil
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
	if len(organisation.ReferenceKey) == 0 {
		missingFieldNames = append(missingFieldNames, "reference_key")
	}

	if len(missingFieldNames) > 0 {
		return cigExchange.NewRequiredFieldError(missingFieldNames)
	}
	return nil
}

// Constants defining the organisation user status
const (
	OrganisationUserStatusInvited    = "invited"
	OrganisationUserStatusUnverified = "unverified"
	OrganisationUserStatusActive     = "active"
)

// OrganisationUser is a struct to represent an organisation to user link
type OrganisationUser struct {
	ID               string     `gorm:"column:id;primary_key"`
	OrganisationID   string     `gorm:"column:organisation_id"`
	UserID           string     `gorm:"column:user_id"`
	OrganisationRole string     `gorm:"column:organisation_role"`
	IsHome           bool       `gorm:"column:is_home"`
	Status           string     `gorm:"column:status;default:'invited'"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
	DeletedAt        *time.Time `gorm:"column:deleted_at"`
}

// TableName returns table name for struct
func (*OrganisationUser) TableName() string {
	return "organisation_user"
}

// BeforeCreate generates new unique UUIDs for new db records
func (*OrganisationUser) BeforeCreate(scope *gorm.Scope) error {

	UUID, err := uuid.NewV4()
	if err != nil {
		return err
	}
	scope.SetColumn("ID", UUID.String())

	return nil
}

// Create inserts new organisation user object into db
func (orgUser *OrganisationUser) Create() *cigExchange.APIError {

	// invalidate the uuid
	orgUser.ID = ""

	// check that both ID's are set
	if len(orgUser.UserID) == 0 {
		return cigExchange.NewInvalidFieldError("user_id", "UserID is invalid")
	}
	if len(orgUser.OrganisationID) == 0 {
		return cigExchange.NewInvalidFieldError("organization_id", "OrganisationID is invalid")
	}

	db := cigExchange.GetDB().Create(orgUser)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Create organization user link call failed", db.Error)
	}
	return nil
}

// Update existing organisation user object in db
func (orgUser *OrganisationUser) Update() *cigExchange.APIError {

	// check that both ID's are set
	if len(orgUser.ID) == 0 {
		return cigExchange.NewInvalidFieldError("id", "ID is invalid")
	}
	if len(orgUser.UserID) == 0 {
		return cigExchange.NewInvalidFieldError("user_id", "UserID is invalid")
	}
	if len(orgUser.OrganisationID) == 0 {
		return cigExchange.NewInvalidFieldError("organization_id", "OrganisationID is invalid")
	}

	// update OrganisationUser
	err := cigExchange.GetDB().Save(orgUser).Error
	if err != nil {
		return cigExchange.NewDatabaseError("Failed to update organisation user ", err)
	}
	return nil
}

// Find queries organisation user from db
func (orgUser *OrganisationUser) Find() (organisationUser *OrganisationUser, apiError *cigExchange.APIError) {

	organisationUser = &OrganisationUser{}
	db := cigExchange.GetDB().Where(orgUser).First(organisationUser)
	if db.Error != nil {
		if db.RecordNotFound() {
			return nil, cigExchange.NewOrganisationUserDoesntExistError("Organisation User with provided parameters doesn't exist")
		}
		return nil, cigExchange.NewDatabaseError("Organisation Users lookup failed", db.Error)
	}
	return organisationUser, nil
}

// Delete existing user organisation object in db
func (orgUser *OrganisationUser) Delete() *cigExchange.APIError {

	// check that UUID is set
	if len(orgUser.ID) == 0 {
		return cigExchange.NewInvalidFieldError("id", "Invalid organisation user id")
	}

	db := cigExchange.GetDB().Delete(orgUser)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Error deleting organisation user", db.Error)
	}
	if db.RowsAffected == 0 {
		return cigExchange.NewInvalidFieldError("organisation_id, user_id", "Organisation User doesn't exist")
	}
	return nil
}

// SetHomeOrganisation marks only 1 OrganisationUser as home
func SetHomeOrganisation(homeOrgUser *OrganisationUser) *cigExchange.APIError {

	// found all OrganisationUser for user
	orgUsers := make([]*OrganisationUser, 0)
	db := cigExchange.GetDB().Where(&OrganisationUser{UserID: homeOrgUser.UserID}).Find(&orgUsers)
	if db.Error != nil {
		if db.RecordNotFound() {
			return cigExchange.NewOrganisationUserDoesntExistError("Organisation User with provided parameters doesn't exist")
		}
		return cigExchange.NewDatabaseError("Organisation Users lookup failed", db.Error)
	}

	// modify IsHome field
	for _, orgUser := range orgUsers {
		if orgUser.ID == homeOrgUser.ID {
			if !orgUser.IsHome {
				// select IsHome in new organisation
				orgUser.IsHome = true
				apiError := orgUser.Update()
				if apiError != nil {
					return apiError
				}
			}
		} else {
			if orgUser.IsHome {
				// deselect IsHome in organisations
				orgUser.IsHome = false
				apiError := orgUser.Update()
				if apiError != nil {
					return apiError
				}
			}
		}
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
func GetUsersForOrganisation(organisationID string, invitedUsers bool) (users []*User, apiErr *cigExchange.APIError) {

	users = make([]*User, 0)
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
		if invitedUsers {
			if orgUser.Status != OrganisationUserStatusInvited {
				continue
			}
		} else {
			if orgUser.Status != OrganisationUserStatusActive {
				continue
			}
		}
		var user User
		db = cigExchange.GetDB().Where(&User{ID: orgUser.UserID}).First(&user)
		if db.Error != nil {
			if db.RecordNotFound() {
				continue
			}
			apiErr = cigExchange.NewDatabaseError("User lookup failed", db.Error)
			return
		}

		// add user to response
		users = append(users, &user)
	}

	return
}

package models

import (
	cigExchange "cig-exchange-libs"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
)

// Offering is a struct to represent an offering
type Offering struct {
	ID             string         `json:"id" gorm:"column:id;primary_key"`
	Title          string         `json:"title" gorm:"column:title"`
	Type           pq.StringArray `json:"type" gorm:"column:type"`
	Description    *string        `json:"description" gorm:"column:description"`
	Rating         string         `json:"rating" gorm:"column:rating"`
	Amount         float64        `json:"amount" gorm:"column:amount"`
	Remaining      float64        `json:"remaining" gorm:"column:remaining"`
	Interest       float64        `json:"interest" gorm:"column:interest"`
	Period         int64          `json:"period" gorm:"column:period"`
	Origin         string         `json:"origin" gorm:"column:origin"`
	IsVisible      bool           `json:"is_visible" gorm:"is_visible"`
	Organisation   Organisation   `json:"-" gorm:"foreignkey:OrganisationID;association_foreignkey:ID"`
	OrganisationID string         `json:"organisation_id" gorm:"column:organisation_id"`
	CreatedAt      time.Time      `json:"created_at" gorm:"column:created_at"`
	UpdatedAt      time.Time      `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt      *time.Time     `json:"-" gorm:"column:deleted_at"`
}

// TableName returns table name for struct
func (*Offering) TableName() string {
	return "offering"
}

// BeforeCreate generates new unique UUIDs for new db records
func (*Offering) BeforeCreate(scope *gorm.Scope) error {

	UUID, err := uuid.NewV4()
	if err != nil {
		return err
	}
	scope.SetColumn("ID", UUID.String())

	return nil
}

// Validate checks that:
// - required fields are pressent and not empty
func (offering *Offering) Validate() *cigExchange.APIError {

	if len(offering.OrganisationID) == 0 {
		return cigExchange.NewInvalidFieldError("organisation_id", "Required field 'organisation_id' missing")
	}
	// check that organisation UUID is valid
	organization := &Organisation{}
	db := cigExchange.GetDB().Where(&Organisation{ID: offering.OrganisationID}).First(&organization)
	if db.Error != nil {

		if db.RecordNotFound() {
			// organisation with UUID doesn't exist
			return cigExchange.NewInvalidFieldError("organisation_id", "Organisation with provided id doesn't exist")
		}
		// database error
		return cigExchange.NewDatabaseError("Fetch organisation failed", db.Error)
	}
	return nil
}

// Create inserts new offering object into db
func (offering *Offering) Create() *cigExchange.APIError {

	// invalidate the uuid
	offering.ID = ""

	if apiError := offering.Validate(); apiError != nil {
		return apiError
	}

	db := cigExchange.GetDB().Create(offering)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Create offering failed", db.Error)
	}
	return nil
}

// Update existing offering object in db
func (offering *Offering) Update(update map[string]interface{}) *cigExchange.APIError {

	// check that UUID is set
	if _, ok := update["id"]; !ok || len(offering.ID) == 0 {
		return cigExchange.NewInvalidFieldError("offering_id", "Offering UUID is not set")
	}

	apiError := offering.Validate()
	if apiError != nil {
		return apiError
	}

	db := cigExchange.GetDB().Model(offering).Updates(update)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Failed to update organisation ", db.Error)
	}
	return nil
}

// Delete existing offering object in db
func (offering *Offering) Delete() *cigExchange.APIError {

	// check that UUID is set
	if len(offering.ID) == 0 {
		return cigExchange.NewInvalidFieldError("offering_id", "Offering id is invalid")
	}

	db := cigExchange.GetDB().Delete(offering)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Failed to delete offering", db.Error)
	}
	if db.RowsAffected == 0 {
		return cigExchange.NewInvalidFieldError("offering_id", "Offering with provided id doesn't exist")
	}
	return nil
}

// GetOffering queries a single offering from db
func GetOffering(UUID string) (*Offering, *cigExchange.APIError) {

	offering := &Offering{
		ID: UUID,
	}
	db := cigExchange.GetDB().First(offering)
	if db.Error != nil {
		if db.RecordNotFound() {
			return nil, cigExchange.NewInvalidFieldError("offering_id", "Offering with provided id doesn't exist")
		}
		return nil, cigExchange.NewDatabaseError("Fetch offering failed", db.Error)
	}

	return offering, nil
}

// GetOfferings queries all offering objects from db
func GetOfferings() ([]*Offering, error) {

	offering := make([]*Offering, 0)
	err := cigExchange.GetDB().Preload("Organisation").Find(&offering).Error

	return offering, err
}

// GetOrganisationOfferings queries all offering objects from db for a given organisation
func GetOrganisationOfferings(organisationID string) ([]*Offering, error) {

	offering := make([]*Offering, 0)
	err := cigExchange.GetDB().Preload("Organisation").Where(&Offering{OrganisationID: organisationID}).Find(&offering).Error

	return offering, err
}

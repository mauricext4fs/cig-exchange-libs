package models

import (
	"cig-exchange-libs"
	"fmt"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/lib/pq"
	"github.com/satori/go.uuid"
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
	Organisation   *Organisation  `json:"-" gorm:"foreignkey:OrganisationID;association_foreignkey:ID"`
	OrganisationID *string        `json:"-" gorm:"column:organisation_id"`
	CreatedAt      time.Time      `json:"-" gorm:"column:created_at"`
	UpdatedAt      time.Time      `json:"-" gorm:"column:updated_at"`
	DeletedAt      *time.Time     `json:"-" gorm:"column:deleted_at"`
}

// TableName returns table name for struct
func (offering *Offering) TableName() string {
	return "offering"
}

// BeforeCreate generates new unique UUIDs for new db records
func (offering *Offering) BeforeCreate(scope *gorm.Scope) error {

	UUID, err := uuid.NewV4()
	if err != nil {
		return err
	}
	scope.SetColumn("ID", UUID.String())

	return nil
}

// Validate checks that:
// - required fields are pressent and not empty
func (offering *Offering) Validate() error {

	if offering.Organisation == nil || len(offering.Organisation.Name) == 0 {
		return fmt.Errorf("Required field 'platform' missing")
	}
	return nil
}

// Create inserts new offering object into db
func (offering *Offering) Create() error {

	// invalidate the uuid
	offering.ID = ""

	if err := offering.Validate(); err != nil {
		return err
	}

	return cigExchange.GetDB().Create(offering).Error
}

// Update existing offering object in db
func (offering *Offering) Update() error {

	// check that UUID is set
	if len(offering.ID) == 0 {
		return fmt.Errorf("Offering UUID is not set")
	}
	if err := offering.Validate(); err != nil {
		return err
	}

	return cigExchange.GetDB().Updates(offering).Error
}

// Delete existing offering object in db
func (offering *Offering) Delete() error {

	// check that UUID is set
	if len(offering.ID) == 0 {
		return fmt.Errorf("Offering UUID is not set")
	}

	return cigExchange.GetDB().Delete(offering).Error
}

// GetOffering queries a single offering from db
func GetOffering(UUID string) (*Offering, error) {

	offering := &Offering{
		ID: UUID,
	}
	err := cigExchange.GetDB().First(offering).Error

	return offering, err
}

// GetOfferings queries all offering objects from db
func GetOfferings() ([]*Offering, error) {

	offering := make([]*Offering, 0)
	err := cigExchange.GetDB().Preload("Organisation").Find(&offering).Error

	return offering, err
}

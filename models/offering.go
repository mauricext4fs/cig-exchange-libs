package models

import (
	cigExchange "cig-exchange-libs"
	"encoding/json"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/jinzhu/gorm/dialects/postgres"
	"github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
)

// Offering is a struct to represent an offering
type Offering struct {
	ID                     string         `json:"id" gorm:"column:id;primary_key"`
	Title                  postgres.Jsonb `json:"title" gorm:"column:title"`
	Type                   pq.StringArray `json:"type" gorm:"column:type"`
	Description            postgres.Jsonb `json:"description" gorm:"column:description"`
	Rating                 postgres.Jsonb `json:"rating" gorm:"column:rating"`
	Slug                   postgres.Jsonb `json:"slug" gorm:"column:slug"`
	Amount                 *float64       `json:"amount" gorm:"column:amount"`
	Remaining              float64        `json:"remaining" gorm:"column:remaining"`
	Interest               *float64       `json:"interest" gorm:"column:interest"`
	Period                 *int64         `json:"period" gorm:"column:period"`
	Origin                 postgres.Jsonb `json:"origin" gorm:"column:origin"`
	Map                    postgres.Jsonb `json:"map" gorm:"column:map"`
	Location               postgres.Jsonb `json:"location" gorm:"column:location"`
	Tagline1               postgres.Jsonb `json:"tagline1" gorm:"column:tagline1"`
	Tagline2               postgres.Jsonb `json:"tagline2" gorm:"column:tagline2"`
	Tagline3               postgres.Jsonb `json:"tagline3" gorm:"column:tagline3"`
	CurrentDebtLevel       postgres.Jsonb `json:"current_debt_level" gorm:"column:current_debt_level"`
	CurrentDebtEndDatetime *string        `json:"current_debt_end_datetime" gorm:"column:current_debt_end_datetime;type:date"`
	AmountAlreadyTaken     *float64       `json:"amount_already_taken" gorm:"column:amount_already_taken"`
	MinimumInvestment      *float64       `json:"minimum_investment" gorm:"column:minimum_investment"`
	MaximumInvestment      *float64       `json:"maximum_investment" gorm:"column:maximum_investment"`
	TransactionFee         *float64       `json:"transaction_fee" gorm:"column:transaction_fee"`
	P2PFee                 *float64       `json:"p2p_fee" gorm:"column:p2p_fee"`
	ReferralReward         *float64       `json:"referral_reward" gorm:"column:referral_reward"`
	ClosingDate            *string        `json:"closing_date" gorm:"column:closing_date"`
	IsVisible              bool           `json:"is_visible" gorm:"is_visible"`
	Organisation           Organisation   `json:"-" gorm:"foreignkey:OrganisationID;association_foreignkey:ID"`
	OrganisationID         string         `json:"organisation_id" gorm:"column:organisation_id"`
	OfferingDirectURL      postgres.Jsonb `json:"offering_direct_url" gorm:"column:offering_direct_url"`
	CreatedAt              time.Time      `json:"created_at" gorm:"column:created_at"`
	UpdatedAt              time.Time      `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt              *time.Time     `json:"-" gorm:"column:deleted_at"`
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

// GetMultilangFields returns jsonb fields
func (offering *Offering) GetMultilangFields() []string {

	return []string{"title", "slug", "origin", "description", "location", "tagline1", "tagline2", "tagline3", "current_debt_level", "rating"}
}

// Validate checks that:
// - required fields are pressent and not empty
func (offering *Offering) Validate() *cigExchange.APIError {

	if len(offering.OrganisationID) == 0 {
		return cigExchange.NewInvalidFieldError("organisation_id", "Required field 'organisation_id' missing")
	}

	// check OfferingDirectURL
	if len(offering.OfferingDirectURL.RawMessage) == 0 {
		return cigExchange.NewInvalidFieldError("offering_direct_url", "Required field 'offering_direct_url' missing")
	}
	value, err := offering.OfferingDirectURL.MarshalJSON()
	if err != nil {
		return cigExchange.NewJSONDecodingError(err)
	}

	type Langs struct {
		En string `json:"en"`
		Fr string `json:"fr"`
		It string `json:"it"`
		De string `json:"de"`
	}

	// check that all languages present
	var langsObject Langs
	if err := json.Unmarshal(value, &langsObject); err != nil {
		return cigExchange.NewJSONDecodingError(err)
	}

	missingFieldNames := make([]string, 0)
	if len(langsObject.En) == 0 {
		missingFieldNames = append(missingFieldNames, "offering_direct_url.en")
	}
	if len(langsObject.Fr) == 0 {
		missingFieldNames = append(missingFieldNames, "offering_direct_url.fr")
	}
	if len(langsObject.It) == 0 {
		missingFieldNames = append(missingFieldNames, "offering_direct_url.it")
	}
	if len(langsObject.De) == 0 {
		missingFieldNames = append(missingFieldNames, "offering_direct_url.de")
	}
	if len(offering.Origin.RawMessage) == 0 {
		missingFieldNames = append(missingFieldNames, "origin")
	}
	if len(offering.Title.RawMessage) == 0 {
		missingFieldNames = append(missingFieldNames, "title")
	}

	if len(missingFieldNames) > 0 {
		return cigExchange.NewRequiredFieldError(missingFieldNames)
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

	db := cigExchange.GetDB().Model(offering).Updates(update)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Failed to update offering", db.Error)
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
func GetOfferings() ([]*Offering, *cigExchange.APIError) {

	offerings := make([]*Offering, 0)
	db := cigExchange.GetDB().Preload("Organisation").Find(&offerings)
	if db.Error != nil {
		if !db.RecordNotFound() {
			return offerings, cigExchange.NewDatabaseError("Fetch all offerings failed", db.Error)
		}
	}

	return offerings, nil
}

// GetOrganisationOfferings queries all offering objects from db for a given organisation
func GetOrganisationOfferings(organisationID string) ([]*Offering, *cigExchange.APIError) {

	offerings := make([]*Offering, 0)
	db := cigExchange.GetDB().Preload("Organisation").Where(&Offering{OrganisationID: organisationID}).Find(&offerings)
	if db.Error != nil {
		if !db.RecordNotFound() {
			return offerings, cigExchange.NewDatabaseError("Fetch offerings failed", db.Error)
		}
	}
	return offerings, nil
}

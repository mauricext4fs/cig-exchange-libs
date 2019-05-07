package models

import (
	cigExchange "cig-exchange-libs"
	"encoding/json"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/jinzhu/gorm/dialects/postgres"
	"github.com/lib/pq"
)

// Offering is a struct to represent an offering
type Offering struct {
	ID                     string         `json:"id" gorm:"column:id;primary_key"`
	Title                  postgres.Jsonb `json:"title" gorm:"column:title"`
	Type                   pq.StringArray `json:"type" gorm:"column:type"`
	Description            postgres.Jsonb `json:"description" gorm:"column:description"`
	Rating                 *string        `json:"rating" gorm:"column:rating"`
	Slug                   *string        `json:"slug" gorm:"column:slug"`
	Amount                 *float64       `json:"amount" gorm:"column:amount"`
	Remaining              float64        `json:"remaining" gorm:"-"`
	Interest               *float64       `json:"interest" gorm:"column:interest"`
	Period                 *int64         `json:"period" gorm:"column:period"`
	Origin                 string         `json:"origin" gorm:"column:origin"`
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
	Media                  []*Media       `json:"-" gorm:"many2many:offering_media;"`
	MediaTypes             MediaTypes     `json:"media"`
	CreatedAt              time.Time      `json:"created_at" gorm:"column:created_at"`
	UpdatedAt              time.Time      `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt              *time.Time     `json:"-" gorm:"column:deleted_at"`
}

// MediaTypes stores different media types separately
type MediaTypes struct {
	OfferingImages    []*Media `json:"offering-images"`
	OfferingDocuments []*Media `json:"offering-documents"`
}

// TableName returns table name for struct
func (*Offering) TableName() string {
	return "offering"
}

// BeforeCreate generates new unique UUIDs for new db records
func (*Offering) BeforeCreate(scope *gorm.Scope) error {

	scope.SetColumn("ID", cigExchange.RandomUUID())
	return nil
}

// GetMultilangFields returns jsonb fields
func (offering *Offering) GetMultilangFields() []string {

	return []string{"title", "description", "location", "tagline1", "tagline2", "tagline3", "current_debt_level"}
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
		return cigExchange.NewRequestDecodingError(err)
	}

	// check that all languages present
	var langsObject cigExchange.MultilangString
	if err := json.Unmarshal(value, &langsObject); err != nil {
		return cigExchange.NewRequestDecodingError(err)
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
	if len(offering.Origin) == 0 {
		missingFieldNames = append(missingFieldNames, "origin")
	}
	if len(offering.Title.RawMessage) == 0 {
		missingFieldNames = append(missingFieldNames, "title")
	}

	if len(missingFieldNames) > 0 {
		return cigExchange.NewRequiredFieldError(missingFieldNames)
	}

	apiErr := offering.checkRemaining()
	if apiErr != nil {
		return apiErr
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

	offering.processOffering()

	return nil
}

// Update existing offering object in db
func (offering *Offering) Update(update map[string]interface{}) *cigExchange.APIError {

	apiErr := offering.checkRemaining()
	if apiErr != nil {
		return apiErr
	}

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
	db := cigExchange.GetDB().Preload("Media", "offering_media.deleted_at is NULL").First(offering)
	if db.Error != nil {
		if db.RecordNotFound() {
			return nil, cigExchange.NewInvalidFieldError("offering_id", "Offering with provided id doesn't exist")
		}
		return nil, cigExchange.NewDatabaseError("Fetch offering failed", db.Error)
	}

	// fill 'remaining' field
	offering.processOffering()

	return offering, nil
}

// GetOfferings queries all offering objects from db
func GetOfferings() ([]*Offering, *cigExchange.APIError) {

	offerings := make([]*Offering, 0)
	db := cigExchange.GetDB().Preload("Organisation", "organisation.deleted_at is NULL").Preload("Media", "offering_media.deleted_at is NULL").Find(&offerings)
	if db.Error != nil {
		if !db.RecordNotFound() {
			return offerings, cigExchange.NewDatabaseError("Fetch all offerings failed", db.Error)
		}
	}

	// fill 'remaining' field
	for _, offering := range offerings {
		offering.processOffering()
	}

	return offerings, nil
}

// GetOrganisationOfferings queries all offering objects from db for a given organisation
func GetOrganisationOfferings(organisationID string) ([]*Offering, *cigExchange.APIError) {

	offerings := make([]*Offering, 0)
	db := cigExchange.GetDB().Preload("Organisation", "organisation.deleted_at is NULL").Preload("Media", "offering_media.deleted_at is NULL").Where(&Offering{OrganisationID: organisationID}).Find(&offerings)
	if db.Error != nil {
		if !db.RecordNotFound() {
			return offerings, cigExchange.NewDatabaseError("Fetch offerings failed", db.Error)
		}
	}

	// fill 'remaining' field
	for _, offering := range offerings {
		offering.processOffering()
	}

	return offerings, nil
}

func (offering *Offering) checkRemaining() *cigExchange.APIError {

	if offering.Amount == nil {
		offering.Amount = new(float64)
	}
	if offering.AmountAlreadyTaken == nil {
		offering.AmountAlreadyTaken = new(float64)
	}
	if *offering.AmountAlreadyTaken > *offering.Amount {
		return cigExchange.NewInvalidFieldError("amount, amount_already_taken", "'amount_already_taken' can't be bigger than 'amount'")
	}
	return nil
}

func (offering *Offering) processOffering() {

	// convert nil value to 0
	if offering.AmountAlreadyTaken == nil {
		offering.AmountAlreadyTaken = new(float64)
	}
	if offering.Amount == nil {
		offering.Amount = new(float64)
	}

	offering.Remaining = *offering.Amount - *offering.AmountAlreadyTaken

	// check for negative 'remaining' value
	if offering.Remaining < 0 {
		offering.Remaining = 0
	}

	offering.MediaTypes.OfferingImages = make([]*Media, 0)
	offering.MediaTypes.OfferingDocuments = make([]*Media, 0)

	// fill images and documents
	for _, m := range offering.Media {
		if m.Type == "image" {
			offering.MediaTypes.OfferingImages = append(offering.MediaTypes.OfferingImages, m)
		} else {
			offering.MediaTypes.OfferingDocuments = append(offering.MediaTypes.OfferingDocuments, m)
		}
	}
}

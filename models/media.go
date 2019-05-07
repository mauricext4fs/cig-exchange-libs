package models

import (
	cigExchange "cig-exchange-libs"
	"time"

	"github.com/jinzhu/gorm"
)

// Media types
const (
	MediaTypeDocument = "offering-document"
	MediaTypeImage    = "offering-image"
)

// Media is a struct to represent an media
type Media struct {
	ID            string     `json:"id" gorm:"column:id;primary_key"`
	Type          string     `json:"type" gorm:"column:type"`
	Subtype       *string    `json:"subtype,omitempty" gorm:"column:subtype"`
	Title         string     `json:"title" gorm:"column:title"`
	URL           string     `json:"url" gorm:"column:url"`
	MimeType      string     `json:"mime_type" gorm:"column:mime_type"`
	FileExtension string     `json:"file_extension" gorm:"column:file_extension"`
	FileSize      int        `json:"file_size" gorm:"column:file_size"`
	Description   *string    `json:"description,omitempty" gorm:"column:description"`
	CreatedAt     time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt     time.Time  `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt     *time.Time `json:"-" gorm:"column:deleted_at"`
}

// TableName returns table name for struct
func (*Media) TableName() string {
	return "media"
}

// BeforeCreate generates new unique UUIDs for new db records
func (*Media) BeforeCreate(scope *gorm.Scope) error {

	scope.SetColumn("ID", cigExchange.RandomUUID())
	return nil
}

// GetMultilangFields returns jsonb fields
func (media *Media) GetMultilangFields() []string {

	return []string{}
}

// OfferingMedia is a struct to represent an offering media link
type OfferingMedia struct {
	ID         string     `json:"id" gorm:"column:id;primary_key"`
	OfferingID string     `json:"offering_id" gorm:"column:offering_id"`
	MediaID    string     `json:"media_id" gorm:"column:media_id"`
	Index      int        `json:"index" gorm:"column:index;default:100"`
	CreatedAt  time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt  time.Time  `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt  *time.Time `json:"-" gorm:"column:deleted_at"`
}

// TableName returns table name for struct
func (*OfferingMedia) TableName() string {
	return "offering_media"
}

// BeforeCreate generates new unique UUIDs for new db records
func (*OfferingMedia) BeforeCreate(scope *gorm.Scope) error {

	scope.SetColumn("ID", cigExchange.RandomUUID())
	return nil
}

// GetMedia queries a single media from db
func GetMedia(mediaID string) (*Media, *cigExchange.APIError) {

	media := &Media{
		ID: mediaID,
	}
	db := cigExchange.GetDB().First(media)
	if db.Error != nil {
		if db.RecordNotFound() {
			return nil, cigExchange.NewInvalidFieldError("media_id", "Media with provided id doesn't exist")
		}
		return nil, cigExchange.NewDatabaseError("Fetch media failed", db.Error)
	}

	return media, nil
}

// CreateMediaForOffering creates media and offering media link
func CreateMediaForOffering(media *Media, offeringID string) *cigExchange.APIError {

	// check that UUID is set
	if len(offeringID) == 0 {
		return cigExchange.NewInvalidFieldError("offering_id", "Offering id is invalid")
	}

	// create media
	db := cigExchange.GetDB().Create(media)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Create media failed", db.Error)
	}

	// create offering media link
	mediaOffering := &OfferingMedia{
		OfferingID: offeringID,
		MediaID:    media.ID,
	}
	db = cigExchange.GetDB().Create(mediaOffering)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Create offering media failed", db.Error)
	}

	return nil
}

// Update existing media object in db
func (media *Media) Update(update map[string]interface{}) *cigExchange.APIError {

	// check that UUID is set
	if _, ok := update["id"]; !ok {
		return cigExchange.NewInvalidFieldError("id", "Invalid media id")
	}

	err := cigExchange.GetDB().Model(media).Updates(update).Error
	if err != nil {
		return cigExchange.NewDatabaseError("Failed to update media", err)
	}
	return nil
}

// GetMediaForOffering queries all offering media objects for offering
func GetMediaForOffering(offeringID string) (media []*Media, apiError *cigExchange.APIError) {

	media = make([]*Media, 0)
	// check that UUID is set
	if len(offeringID) == 0 {
		return media, cigExchange.NewInvalidFieldError("offering_id", "Offering id is invalid")
	}

	db := cigExchange.GetDB().Joins("JOIN offering_media on offering_media.media_id=media.id").Where("offering_media.offering_id=?", offeringID).Find(&media)
	if db.Error != nil {
		if !db.RecordNotFound() {
			return
		}
		apiError = cigExchange.NewDatabaseError("Fetch offering media failed", db.Error)
	}
	return
}

// DeleteOfferingMedia deletes media and offering media link
func DeleteOfferingMedia(mediaID string) *cigExchange.APIError {

	// check that UUID is set
	if len(mediaID) == 0 {
		return cigExchange.NewInvalidFieldError("media_id", "Media id is invalid")
	}

	// delete media
	db := cigExchange.GetDB().Delete(&Media{ID: mediaID})
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Failed to delete media", db.Error)
	}
	if db.RowsAffected == 0 {
		return cigExchange.NewInvalidFieldError("media_id", "Media with provided id doesn't exist")
	}

	// delete offering media link
	db = cigExchange.GetDB().Where("media_id = ?", mediaID).Delete(&OfferingMedia{})
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Failed to delete offering media link", db.Error)
	}
	if db.RowsAffected == 0 {
		return cigExchange.NewInvalidFieldError("media_id", "Offering Media link with provided id doesn't exist")
	}
	return nil
}

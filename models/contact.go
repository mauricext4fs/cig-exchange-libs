package models

import (
	cigExchange "cig-exchange-libs"
	"time"

	"github.com/jinzhu/gorm"
)

// Constants defining the contact level
const (
	ContactLevelPrimary   = "primary"
	ContactLevelSecondary = "secondary"
)

// Constants defining the contact type
const (
	ContactTypeEmail = "email"
	ContactTypePhone = "phone"
)

// Contact is a struct to represent a contact
type Contact struct {
	ID        string     `json:"id" gorm:"column:id;primary_key"`
	Level     string     `json:"level" gorm:"column:level"`
	Location  string     `json:"location" gorm:"column:location"`
	Type      string     `json:"type" gorm:"column:type"`
	Subtype   string     `json:"subtype" gorm:"column:subtype"`
	Value1    string     `json:"value1" gorm:"column:value1"`
	Value2    string     `json:"value2" gorm:"column:value2"`
	Value3    string     `json:"value3" gorm:"column:value3"`
	Value4    string     `json:"value4" gorm:"column:value4"`
	Value5    string     `json:"value5" gorm:"column:value5"`
	Value6    string     `json:"value6" gorm:"column:value6"`
	CreatedAt time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt time.Time  `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt *time.Time `json:"-" gorm:"column:deleted_at"`
}

// TableName returns table name for struct
func (*Contact) TableName() string {
	return "contact"
}

// BeforeCreate generates new unique UUIDs for new db records
func (*Contact) BeforeCreate(scope *gorm.Scope) error {

	scope.SetColumn("ID", cigExchange.RandomUUID())
	return nil
}

// GetMultilangFields returns jsonb fields
func (*Contact) GetMultilangFields() []string {

	return []string{}
}

// ContactWithIndex contains Contact struct with index from UserContact
type ContactWithIndex struct {
	*Contact
	Index int32 `json:"index" gorm:"column:index;default:100"`
}

// GetContact queries a contact from db
func GetContact(contactID string) (*Contact, *cigExchange.APIError) {

	contact := &Contact{
		ID: contactID,
	}
	db := cigExchange.GetDB().First(contact)
	if db.Error != nil {
		if db.RecordNotFound() {
			return nil, cigExchange.NewInvalidFieldError("contact_id", "Contact with provided id doesn't exist")
		}
		return nil, cigExchange.NewDatabaseError("Fetch contact failed", db.Error)
	}

	return contact, nil
}

// GetContacts queries all contact for user from db
func GetContacts(userID string) ([]*ContactWithIndex, *cigExchange.APIError) {

	contacts := make([]*ContactWithIndex, 0)

	// check that UUID is set
	if len(userID) == 0 {
		return nil, cigExchange.NewInvalidFieldError("user_id", "User id is invalid")
	}

	selectS := "SELECT contact.*, user_contact.index FROM public.contact "
	joinS := "INNER JOIN public.user_contact ON contact.id = user_contact.contact_id "
	whereS := "WHERE user_contact.user_id = '" + userID + "';"
	// query ContactWithIndex structs
	db := cigExchange.GetDB().Raw(selectS + joinS + whereS).Scan(&contacts)
	if db.Error != nil {
		if !db.RecordNotFound() {
			return nil, cigExchange.NewDatabaseError("Fetch contacts failed", db.Error)
		}
	}

	return contacts, nil
}

// Create inserts new offering contact and user_contact into db
func (contact *Contact) Create(userID string, index int32) *cigExchange.APIError {

	tx := cigExchange.GetDB().Begin()
	// invalidate the uuid
	contact.ID = ""

	err := tx.Create(contact).Error
	if err != nil {
		tx.Rollback()
		return cigExchange.NewDatabaseError("Create contact failed", err)
	}

	// create user contact link
	userContact := &UserContact{
		UserID:    userID,
		ContactID: contact.ID,
		Index:     index,
	}
	err = tx.Create(userContact).Error
	if err != nil {
		tx.Rollback()
		return cigExchange.NewDatabaseError("Create user contact link failed", err)
	}

	// commit new records
	if err = tx.Commit().Error; err != nil {
		tx.Rollback()
		return cigExchange.NewDatabaseError("Commit contact creation failed", err)
	}

	return nil
}

// Update existing contact object in db
func (contact *Contact) Update(userID string, update map[string]interface{}, index int32) *cigExchange.APIError {

	// check that contact belongs to user
	userContact := &UserContact{}
	db := cigExchange.GetDB().Where(&UserContact{UserID: userID, ContactID: contact.ID}).First(userContact)
	if db.Error != nil {
		if db.RecordNotFound() {
			return cigExchange.NewInvalidFieldError("user_id, contact_id", "Contact with provided user_id and contact_id doesn't exist")
		}
		return cigExchange.NewDatabaseError("Fetch user_contact failed", db.Error)
	}

	// check that UUID is set
	if _, ok := update["id"]; !ok {
		return cigExchange.NewInvalidFieldError("contact_id", "Contact UUID is not set")
	}

	tx := cigExchange.GetDB().Begin()

	if userContact.Index != index {
		userContact.Index = index
		err := tx.Save(userContact).Error
		if err != nil {
			tx.Rollback()
			return cigExchange.NewDatabaseError("Can't update user contact", err)
		}
	}
	err := tx.Model(contact).Updates(update).Error
	if err != nil {
		tx.Rollback()
		return cigExchange.NewDatabaseError("Failed to update contact", db.Error)
	}

	// commit new records
	if err = tx.Commit().Error; err != nil {
		tx.Rollback()
		return cigExchange.NewDatabaseError("Commit contact update failed", err)
	}

	db = cigExchange.GetDB().Where(&Contact{ID: contact.ID}).First(contact)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Fetch contact failed", db.Error)
	}

	return nil
}

// Delete existing offering object in db
func (contact *Contact) Delete(userID string) *cigExchange.APIError {

	tx := cigExchange.GetDB().Begin()

	// check that UUID is set
	if len(contact.ID) == 0 {
		return cigExchange.NewInvalidFieldError("contact_id", "Contact id is invalid")
	}

	// delete contact
	err := tx.Delete(contact).Error
	if err != nil {
		tx.Rollback()
		return cigExchange.NewDatabaseError("Failed to delete contact", err)
	}
	if tx.RowsAffected == 0 {
		tx.Rollback()
		return cigExchange.NewInvalidFieldError("contact_id", "Contact with provided id doesn't exist")
	}

	// delete user contact link
	err = tx.Delete(&UserContact{ContactID: contact.ID, UserID: userID}).Error
	if err != nil {
		tx.Rollback()
		return cigExchange.NewDatabaseError("Failed to delete user contact", err)
	}
	if tx.RowsAffected == 0 {
		tx.Rollback()
		return cigExchange.NewInvalidFieldError("contact_id", "User Contact link doesn't exist")
	}

	// commit deletion
	if err = tx.Commit().Error; err != nil {
		tx.Rollback()
		return cigExchange.NewDatabaseError("Commit contact deletion failed", err)
	}

	return nil
}

// UserContact is a struct to represent a contact
type UserContact struct {
	ID        string     `gorm:"column:id;primary_key"`
	UserID    string     `gorm:"column:user_id"`
	ContactID string     `gorm:"column:contact_id"`
	Index     int32      `gorm:"column:index;default:100"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
	DeletedAt *time.Time `gorm:"column:deleted_at"`
}

// TableName returns table name for struct
func (*UserContact) TableName() string {
	return "user_contact"
}

// BeforeCreate generates new unique UUIDs for new db records
func (*UserContact) BeforeCreate(scope *gorm.Scope) error {

	scope.SetColumn("ID", cigExchange.RandomUUID())
	return nil
}

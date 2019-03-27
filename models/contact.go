package models

import (
	"time"

	"github.com/jinzhu/gorm"
	uuid "github.com/satori/go.uuid"
)

// Contact is a struct to represent a contact
type Contact struct {
	ID        string     `gorm:"column:id;primary_key"`
	Level     string     `gorm:"column:level"`
	Location  string     `gorm:"column:location"`
	Type      string     `gorm:"column:type"`
	Subtype   string     `gorm:"column:subtype"`
	Value1    string     `gorm:"column:value1"`
	Value2    string     `gorm:"column:value2"`
	Value3    string     `gorm:"column:value3"`
	Value4    string     `gorm:"column:value4"`
	Value5    string     `gorm:"column:value5"`
	Value6    string     `gorm:"column:value6"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
	DeletedAt *time.Time `gorm:"column:deleted_at"`
}

// TableName returns table name for struct
func (*Contact) TableName() string {
	return "contact"
}

// BeforeCreate generates new unique UUIDs for new db records
func (*Contact) BeforeCreate(scope *gorm.Scope) error {

	UUID, err := uuid.NewV4()
	if err != nil {
		return err
	}
	scope.SetColumn("ID", UUID.String())

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

	UUID, err := uuid.NewV4()
	if err != nil {
		return err
	}
	scope.SetColumn("ID", UUID.String())

	return nil
}

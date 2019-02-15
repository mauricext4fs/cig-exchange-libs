package models

import (
	"time"
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
func (user *Organisation) TableName() string {
	return "organisation"
}

// OrganisationUser is a struct to represent an organisation to user link
type OrganisationUser struct {
	OrganisationID string `gorm:"column:organisation_id"`
	UserID         string `gorm:"column:user_id"`
}

// TableName returns table name for struct
func (user *OrganisationUser) TableName() string {
	return "organisation_user"
}
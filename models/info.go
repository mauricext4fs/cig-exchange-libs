package models

import (
	"time"

	"github.com/jinzhu/gorm"
	uuid "github.com/satori/go.uuid"
)

// Info is a struct to represent an info
type Info struct {
	ID        string     `json:"id" gorm:"column:id;primary_key"`
	Label     string     `json:"label" gorm:"column:label"`
	Value     string     `json:"value" gorm:"column:value"`
	CreatedAt time.Time  `json:"created_at" gorm:"column:created_at"`
	UpdatedAt time.Time  `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt *time.Time `json:"-" gorm:"column:deleted_at"`
}

// TableName returns table name for struct
func (*Info) TableName() string {
	return "info"
}

// BeforeCreate generates new unique UUIDs for new db records
func (*Info) BeforeCreate(scope *gorm.Scope) error {

	scope.SetColumn("ID", cigExchange.RandomUUID())
	return nil
}

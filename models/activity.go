package models

import (
	"time"

	"github.com/jinzhu/gorm"
	"github.com/jinzhu/gorm/dialects/postgres"
	uuid "github.com/satori/go.uuid"
)

// UserActivity types
const (
	ActivityTypeSignUp             = "sign_up"
	ActivityTypeSignIn             = "sign_in"
	ActivityTypeSendOtp            = "send_otp"
	ActivityTypeVerifyOtp          = "verify_otp"
	ActivityTypeOrganisationSignUp = "org_sign_up"
	ActivityTypeAllOfferings       = "get_offerings"
	ActivityTypeContactUs          = "contact_us"
	ActivityTypeSwitchOrganisation = "switch"
	ActivityTypeUpdateUser         = "update_user"
	ActivityTypeGetUser            = "get_user"
	ActivityTypeCreateOrganisation = "create_org"
	ActivityTypeGetOrganisations   = "get_orgs"
	ActivityTypeGetOrganisation    = "get_org"
	ActivityTypeUpdateOrganisation = "update_org"
	ActivityTypeDeleteOrganisation = "delete_org"
	ActivityTypeCreateOffering     = "create_offering"
	ActivityTypeGetOfferings       = "get_offerings"
	ActivityTypeGetOffering        = "get_offering"
	ActivityTypeUpdateOffering     = "update_offering"
	ActivityTypeDeleteOffering     = "delete_offering"
	ActivityTypeGetUsers           = "get_users"
	ActivityTypeDeleteUser         = "delete_user"
	ActivityTypeCreateInvitation   = "create_invitation"
	ActivityTypeGetInvitations     = "get_invitations"
	ActivityTypeDeleteInvitation   = "delete_invitation"
	ActivityTypeSessionLength      = "user_session"
	ActivityTypeCreateUserActivity = "create_user_activity"
)

// UnknownUser user for trading api calls
const UnknownUser = "00000000-0000-0000-0000-000000000000"

// UserActivity is a struct to represent an user activity
type UserActivity struct {
	ID        string         `json:"id" gorm:"column:id;primary_key"`
	UserID    string         `json:"user_id" gorm:"column:user_id"`
	Type      string         `json:"type" gorm:"column:type"`
	Info      *string        `json:"info" gorm:"column:info"`
	JWT       postgres.Jsonb `json:"jwt" gorm:"column:jwt"`
	CreatedAt time.Time      `json:"created_at" gorm:"column:created_at"`
	UpdatedAt time.Time      `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt *time.Time     `json:"-" gorm:"column:deleted_at"`
}

// TableName returns table name for struct
func (*UserActivity) TableName() string {
	return "user_activity"
}

// BeforeCreate generates new unique UUIDs for new db records
func (*UserActivity) BeforeCreate(scope *gorm.Scope) error {

	UUID, err := uuid.NewV4()
	if err != nil {
		return err
	}
	scope.SetColumn("ID", UUID.String())

	return nil
}

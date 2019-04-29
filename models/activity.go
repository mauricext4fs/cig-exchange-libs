package models

import (
	cigExchange "cig-exchange-libs"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/jinzhu/gorm/dialects/postgres"
)

// UserActivity types
const (
	ActivityTypeSignUp                = "sign_up"
	ActivityTypeSignIn                = "sign_in"
	ActivityTypeSendOtp               = "send_otp"
	ActivityTypeVerifyOtp             = "verify_otp"
	ActivityTypeOrganisationSignUp    = "org_sign_up"
	ActivityTypeAllOfferings          = "get_all_offerings"
	ActivityTypeContactUs             = "contact_us"
	ActivityTypeSwitchOrganisation    = "switch"
	ActivityTypeUpdateUser            = "update_user"
	ActivityTypeGetUser               = "get_user"
	ActivityTypeCreateOrganisation    = "create_org"
	ActivityTypeGetOrganisations      = "get_orgs"
	ActivityTypeGetOrganisation       = "get_org"
	ActivityTypeUpdateOrganisation    = "update_org"
	ActivityTypeDeleteOrganisation    = "delete_org"
	ActivityTypeCreateOffering        = "create_offering"
	ActivityTypeGetOfferings          = "get_offerings"
	ActivityTypeGetOffering           = "get_offering"
	ActivityTypeUpdateOffering        = "update_offering"
	ActivityTypeDeleteOffering        = "delete_offering"
	ActivityTypeGetUsers              = "get_users"
	ActivityTypeAddUser               = "add_user"
	ActivityTypeDeleteUser            = "delete_user"
	ActivityTypeCreateInvitation      = "create_invitation"
	ActivityTypeGetInvitations        = "get_invitations"
	ActivityTypeDeleteInvitation      = "delete_invitation"
	ActivityTypeAcceptInvitation      = "accept_invitation"
	ActivityTypeSessionLength         = "user_session"
	ActivityTypeCreateUserActivity    = "create_user_activity"
	ActivityTypeUserInfo              = "get_user_info"
	ActivityTypeGetUserActivities     = "get_user_activities"
	ActivityTypeGetDashboard          = "get_dashboard"
	ActivityTypeGetDashboardUsers     = "get_dashboard_users"
	ActivityTypeGetDashboardBreakdown = "get_dashboard_breakdown"
	ActivityTypeGetDashboardClick     = "get_dashboard_click"
	ActivityTypeGetOfferingsMedia     = "get_offerings_media"
	ActivityTypeUploadMedia           = "upload_media"
	ActivityTypeUpdateOfferingsMedia  = "update_offerings_media"
	ActivityTypeDeleteOfferingsMedia  = "delete_offerings_media"
)

// UnknownUser user for trading api calls
const UnknownUser = "00000000-0000-0000-0000-000000000000"

// UserActivity is a struct to represent an user activity
type UserActivity struct {
	ID         string         `json:"id" gorm:"column:id;primary_key"`
	UserID     string         `json:"user_id" gorm:"column:user_id"`
	RemoteAddr string         `json:"remote_addr" gorm:"remote_addr"`
	Type       string         `json:"type" gorm:"column:type"`
	Info       *string        `json:"info" gorm:"column:info"`
	JWT        postgres.Jsonb `json:"jwt" gorm:"column:jwt"`
	CreatedAt  time.Time      `json:"created_at" gorm:"column:created_at"`
	UpdatedAt  time.Time      `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt  *time.Time     `json:"-" gorm:"column:deleted_at"`
}

// TableName returns table name for struct
func (*UserActivity) TableName() string {
	return "user_activity"
}

// BeforeCreate generates new unique UUIDs for new db records
func (*UserActivity) BeforeCreate(scope *gorm.Scope) error {
	scope.SetColumn("ID", cigExchange.RandomUUID())
	return nil
}

// GetActivitiesForUser queries all user activities for user from db
func GetActivitiesForUser(userID string) (userActs []*UserActivity, apiErr *cigExchange.APIError) {

	userActs = make([]*UserActivity, 0)
	// find all userActivities objects for organisation
	db := cigExchange.GetDB().Where(&UserActivity{UserID: userID}).Find(&userActs)
	if db.Error != nil {
		if !db.RecordNotFound() {
			apiErr = cigExchange.NewDatabaseError("UserActivity lookup failed", db.Error)
		}
	}
	return
}

// FindSessionActivity queries session user activity for user from db
func (activity *UserActivity) FindSessionActivity() (activityResp *UserActivity, apiErr *cigExchange.APIError) {

	sType := ActivityTypeSessionLength
	activityResp = &UserActivity{}
	now := time.Now()
	// session wait time 10 minutes
	limit := now.Add(time.Duration(-10) * time.Minute)
	db := cigExchange.GetDB().Where("updated_at > ? and user_id = ? and jwt = ? and type = ?", limit, activity.UserID, activity.JWT, sType).Order("updated_at desc").First(activityResp)
	if db.Error != nil {
		if db.RecordNotFound() {
			activityResp = activity
			return
		}
		return nil, cigExchange.NewDatabaseError("UserActivity lookup failed", db.Error)
	}
	return
}

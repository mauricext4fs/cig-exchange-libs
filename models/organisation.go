package models

import (
	cigExchange "cig-exchange-libs"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/jinzhu/gorm/dialects/postgres"
	uuid "github.com/satori/go.uuid"
)

// Constants defining the user role in organisation
const (
	OrganisationRoleAdmin = "admin"
	OrganisationRoleUser  = "user"
)

// Constants defining the organisation status
const (
	OrganisationStatusVerified   = "verified"
	OrganisationStatusUnverified = "unverified"
)

// Organisation is a struct to represent an organisation
type Organisation struct {
	ID                        string         `json:"id" gorm:"column:id;primary_key"`
	Type                      string         `json:"type" gorm:"column:type"`
	Name                      string         `json:"name" gorm:"column:name"`
	Website                   string         `json:"website" gorm:"column:website"`
	ReferenceKey              string         `json:"reference_key" gorm:"column:reference_key"`
	OfferingRatingDescription postgres.Jsonb `json:"offering_rating_description" gorm:"column:offering_rating_description"`
	Status                    string         `json:"status" gorm:"column:status;default:'unverified'"`
	Verified                  int64          `json:"-" gorm:"column:verified"`
	CreatedAt                 time.Time      `json:"created_at" gorm:"column:created_at"`
	UpdatedAt                 time.Time      `json:"updated_at" gorm:"column:updated_at"`
	DeletedAt                 *time.Time     `json:"-" gorm:"column:deleted_at"`
}

// TableName returns table name for struct
func (*Organisation) TableName() string {
	return "organisation"
}

// BeforeCreate generates new unique UUIDs for new db records
func (*Organisation) BeforeCreate(scope *gorm.Scope) error {

	UUID, err := uuid.NewV4()
	if err != nil {
		return err
	}
	scope.SetColumn("ID", UUID.String())

	return nil
}

// GetMultilangFields returns jsonb fields
func (*Organisation) GetMultilangFields() []string {

	return []string{"offering_rating_description"}
}

// Create inserts new organisation object into db
func (organisation *Organisation) Create() *cigExchange.APIError {

	// invalidate the uuid
	organisation.ID = ""

	// create unverified organisation
	organisation.Status = OrganisationStatusUnverified

	if apiErr := organisation.trimFieldsAndValidate(); apiErr != nil {
		return apiErr
	}

	db := cigExchange.GetDB().Create(organisation)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Failed to create organisation", db.Error)
	}
	return nil
}

// Update existing organisation object in db
func (organisation *Organisation) Update(update map[string]interface{}) *cigExchange.APIError {

	// check that UUID is set
	if _, ok := update["id"]; !ok || len(organisation.ID) == 0 {
		return cigExchange.NewInvalidFieldError("organisation_id", "Invalid organisation id")
	}

	err := cigExchange.GetDB().Model(organisation).Updates(update).Error
	if err != nil {
		return cigExchange.NewDatabaseError("Failed to update organisation ", err)
	}
	return nil
}

// Delete existing organisation object in db
func (organisation *Organisation) Delete() *cigExchange.APIError {

	// check that UUID is set
	if len(organisation.ID) == 0 {
		return cigExchange.NewInvalidFieldError("organisation_id", "Invalid organisation id")
	}

	db := cigExchange.GetDB().Delete(organisation)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Failed to delete organisation", db.Error)
	}
	if db.RowsAffected == 0 {
		return cigExchange.NewInvalidFieldError("organisation_id", "Organisation with provided id doesn't exist")
	}
	return nil
}

// GetOrganisation queries a single organisation from db
func GetOrganisation(UUID string) (*Organisation, *cigExchange.APIError) {

	// check that UUID is set
	if len(UUID) == 0 {
		return nil, cigExchange.NewInvalidFieldError("organisation_id", "Invalid organisation id")
	}

	organisation := &Organisation{
		ID: UUID,
	}
	db := cigExchange.GetDB().First(organisation)

	if db.Error != nil {
		if !db.RecordNotFound() {
			return nil, cigExchange.NewDatabaseError("Organisation lookup failed", db.Error)
		}
		return nil, cigExchange.NewOrganisationDoesntExistError("Organisation with provided uuid doesn't exist")
	}

	return organisation, nil
}

// GetOrganisations queries all organisations for user from db
func GetOrganisations(userUUID string) ([]*Organisation, *cigExchange.APIError) {

	// check that UUID is set
	if len(userUUID) == 0 {
		return nil, cigExchange.NewInvalidFieldError("user_id", "Invalid user id")
	}

	organisations := make([]*Organisation, 0)

	var orgUsers []OrganisationUser

	// find all organisationUser objects for user
	db := cigExchange.GetDB().Where(&OrganisationUser{UserID: userUUID}).Find(&orgUsers)
	if db.Error != nil {
		if !db.RecordNotFound() {
			return nil, cigExchange.NewDatabaseError("OrganisationUser lookup failed", db.Error)
		}
	}

	for _, orgUser := range orgUsers {
		organisation := &Organisation{
			ID: orgUser.OrganisationID,
		}
		db := cigExchange.GetDB().First(organisation)
		if db.Error != nil {
			if !db.RecordNotFound() {
				return nil, cigExchange.NewDatabaseError("Organisation lookup failed", db.Error)
			}
		}
		organisations = append(organisations, organisation)
	}

	return organisations, nil
}

// GetAllOrganisations queries all organisations from db
func GetAllOrganisations() ([]*Organisation, *cigExchange.APIError) {

	orgs := make([]*Organisation, 0)
	// find all userActivities objects for organisation
	db := cigExchange.GetDB().Find(&orgs)
	if db.Error != nil {
		if !db.RecordNotFound() {
			apiErr := cigExchange.NewDatabaseError("UserActivity lookup failed", db.Error)
			return orgs, apiErr
		}
	}
	return orgs, nil
}

func (organisation *Organisation) trimFieldsAndValidate() *cigExchange.APIError {

	organisation.Name = strings.TrimSpace(organisation.Name)
	organisation.Type = strings.TrimSpace(organisation.Type)
	organisation.ReferenceKey = strings.TrimSpace(organisation.ReferenceKey)

	missingFieldNames := make([]string, 0)
	if len(organisation.Name) == 0 {
		missingFieldNames = append(missingFieldNames, "name")
	}
	if len(organisation.ReferenceKey) == 0 {
		missingFieldNames = append(missingFieldNames, "reference_key")
	}

	if len(missingFieldNames) > 0 {
		return cigExchange.NewRequiredFieldError(missingFieldNames)
	}
	return nil
}

// OrganisationInfo is a struct to store dashboard values
type OrganisationInfo struct {
	TotalOfferings  int     `json:"total_offerings"`
	TotalUsers      int     `json:"total_users"`
	TotalAmount     float32 `json:"total_amount"`
	RemainingAmount float32 `json:"remaining_amount"`
}

// GetOrganisationInfo returns values for organisation dashboard
func GetOrganisationInfo(organisationID string) (*OrganisationInfo, *cigExchange.APIError) {

	organisationInfo := &OrganisationInfo{}

	// get total offerings
	var count int
	db := cigExchange.GetDB().Model(&Offering{}).Where("organisation_id = ?", organisationID).Count(&count)
	if db.Error != nil {
		return nil, cigExchange.NewDatabaseError("Get total offerings for organisation failed", db.Error)
	}
	organisationInfo.TotalOfferings = count

	// get total users
	db = cigExchange.GetDB().Model(&OrganisationUser{}).Where("organisation_id = ? and status = ?", organisationID, OrganisationUserStatusActive).Count(&count)
	if db.Error != nil {
		return nil, cigExchange.NewDatabaseError("Get total users for organisation failed", db.Error)
	}
	organisationInfo.TotalUsers = count

	// get offerings amount and amount already taken
	row := cigExchange.GetDB().Model(&Offering{}).Select("sum(amount), sum(amount_already_taken)").Where("organisation_id = ?", organisationID).Row()

	var amount float32
	var taken float32
	err := row.Scan(&amount, &taken)
	if err != nil {
		fmt.Println(cigExchange.NewDatabaseError("Get total and remaininig amount for organisation failed", err).ToString())
		return organisationInfo, nil
	}
	organisationInfo.TotalAmount = amount
	organisationInfo.RemainingAmount = amount - taken

	return organisationInfo, nil
}

// OrganisationUserInfo is a struct to store dashboard values
type OrganisationUserInfo struct {
	Name        string  `json:"name"`
	LastName    string  `json:"lastname"`
	UserID      string  `json:"user_id"`
	Count       float32 `json:"count"`
	AverageTime int     `json:"average"`
}

// GetOrganisationUsersInfo returns values for organisation users dashboard
func GetOrganisationUsersInfo(organisationID string) ([]*OrganisationUserInfo, *cigExchange.APIError) {

	organisationUsersInfo := make([]*OrganisationUserInfo, 0)

	selectS := "SELECT \"user\".name, \"user\".lastname, user_id, COUNT(user_id) as c, extract(epoch from sum(\"user_activity\".updated_at - \"user_activity\".created_at)) / count(*) as average FROM public.user_activity "
	joinS := "INNER JOIN public.user ON public.user_activity.user_id = public.user.id "
	whereS := "WHERE type = 'user_session' and jwt @> '{\"organisation_id\": \"" + organisationID + "\"}' "
	groupS := "GROUP BY user_id, \"user\".name, \"user\".lastname;"
	// get user sessions
	rows, err := cigExchange.GetDB().Raw(selectS + joinS + whereS + groupS).Rows()
	if err != nil {
		return nil, cigExchange.NewDatabaseError("Get user sessions for organisation failed", err)
	}
	defer rows.Close()
	for rows.Next() {
		var average float64
		orgUserInfo := &OrganisationUserInfo{}
		err = rows.Scan(&orgUserInfo.Name, &orgUserInfo.LastName, &orgUserInfo.UserID, &orgUserInfo.Count, &average)
		if err == nil {
			orgUserInfo.AverageTime = int(average)
			organisationUsersInfo = append(organisationUsersInfo, orgUserInfo)
		} else {
			fmt.Printf("GetOrganisationUsersInfo error: %v\n", err.Error())
		}
	}

	return organisationUsersInfo, nil
}

// OrganisationOfferingsTypeBreakdown is a struct to store dashboard values
type OrganisationOfferingsTypeBreakdown struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// GetOfferingsTypeBreakdown returns values for organisation offerings breakdown
func GetOfferingsTypeBreakdown(organisationID string) ([]*OrganisationOfferingsTypeBreakdown, *cigExchange.APIError) {

	organisationOfferings := make([]*OrganisationOfferingsTypeBreakdown, 0)

	selectS := "SELECT count(x.offering_type) as total, x.offering_type FROM public.offering o , LATERAL "
	lateralS := "(SELECT unnest(o.type) AS offering_type) x "
	whereS := "WHERE o.organisation_id = '" + organisationID + "' "
	groupS := "GROUP BY x.offering_type ORDER BY total DESC;"
	// get organisation offerings breakdown
	rows, err := cigExchange.GetDB().Raw(selectS + lateralS + whereS + groupS).Rows()
	if err != nil {
		return nil, cigExchange.NewDatabaseError("Get offerings type breakdown for organisation failed", err)
	}
	defer rows.Close()
	for rows.Next() {
		// fill one offering type
		orgOffering := &OrganisationOfferingsTypeBreakdown{}
		err = rows.Scan(&orgOffering.Count, &orgOffering.Type)
		if err == nil {
			organisationOfferings = append(organisationOfferings, orgOffering)
		}
	}

	return organisationOfferings, nil
}

// OrganisationOfferingClicks is a struct to store dashboard values
type OrganisationOfferingClicks struct {
	OfferingID       string         `json:"offering_id"`
	OfferingTitle    string         `json:"title"`
	OfferingTitleMap postgres.Jsonb `json:"title_map"`
	Count            int            `json:"count"`
}

// GetOfferingsClicks returns values for offering clicks
func GetOfferingsClicks(organisationID string) ([]*OrganisationOfferingClicks, *cigExchange.APIError) {

	offerings := make([]*Offering, 0)
	offeringsClicks := make([]*OrganisationOfferingClicks, 0)

	// get all offerings for organisation
	db := cigExchange.GetDB().Where(&Offering{OrganisationID: organisationID}).Find(&offerings)
	if db.Error != nil {
		if !db.RecordNotFound() {
			return offeringsClicks, cigExchange.NewDatabaseError("Offerings lookup failed", db.Error)
		}
	}

	for _, offering := range offerings {
		clicks := &OrganisationOfferingClicks{
			OfferingID:       offering.ID,
			OfferingTitleMap: offering.Title,
		}

		offeringMap := make(map[string]interface{})
		// marshal to json
		offeringBytes, err := json.Marshal(offering)
		if err != nil {
			return offeringsClicks, cigExchange.NewJSONEncodingError(err)
		}

		// fill map
		err = json.Unmarshal(offeringBytes, &offeringMap)
		if err != nil {
			return offeringsClicks, cigExchange.NewJSONDecodingError(err)
		}

		val, ok := offeringMap["title"]
		if !ok {
			continue
		}
		if val != nil {
			mapLang, ok := val.(map[string]interface{})
			if ok {
				if v, ok := mapLang["en"]; ok {
					valStr, ok := v.(string)
					if ok {
						clicks.OfferingTitle = valStr
					}
				} else if v, ok := mapLang["fr"]; ok {
					valStr, ok := v.(string)
					if ok {
						clicks.OfferingTitle = valStr
					}
				} else if v, ok := mapLang["it"]; ok {
					valStr, ok := v.(string)
					if ok {
						clicks.OfferingTitle = valStr
					}
				} else if v, ok := mapLang["de"]; ok {
					valStr, ok := v.(string)
					if ok {
						clicks.OfferingTitle = valStr
					}
				}
			}
		}
		selectS := "SELECT count(*) as total FROM public.user_activity WHERE type = 'offering_click' and info ~ '" + offering.ID + "';"
		// get organisation offerings breakdown
		row := cigExchange.GetDB().Raw(selectS).Row()
		var amount int
		err = row.Scan(&amount)
		if err == nil {
			clicks.Count = amount
		}
		offeringsClicks = append(offeringsClicks, clicks)
	}

	sort.Slice(offeringsClicks, func(i, j int) bool {
		return offeringsClicks[i].Count > offeringsClicks[j].Count
	})

	return offeringsClicks, nil
}

// Constants defining the organisation user status
const (
	OrganisationUserStatusInvited    = "invited"
	OrganisationUserStatusUnverified = "unverified"
	OrganisationUserStatusActive     = "active"
)

// OrganisationUser is a struct to represent an organisation to user link
type OrganisationUser struct {
	ID               string     `gorm:"column:id;primary_key"`
	OrganisationID   string     `gorm:"column:organisation_id"`
	UserID           string     `gorm:"column:user_id"`
	OrganisationRole string     `gorm:"column:organisation_role"`
	IsHome           bool       `gorm:"column:is_home"`
	Status           string     `gorm:"column:status;default:'invited'"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
	DeletedAt        *time.Time `gorm:"column:deleted_at"`
}

// TableName returns table name for struct
func (*OrganisationUser) TableName() string {
	return "organisation_user"
}

// BeforeCreate generates new unique UUIDs for new db records
func (*OrganisationUser) BeforeCreate(scope *gorm.Scope) error {

	UUID, err := uuid.NewV4()
	if err != nil {
		return err
	}
	scope.SetColumn("ID", UUID.String())

	return nil
}

// Create inserts new organisation user object into db
func (orgUser *OrganisationUser) Create() *cigExchange.APIError {

	// invalidate the uuid
	orgUser.ID = ""

	// check that both ID's are set
	if len(orgUser.UserID) == 0 {
		return cigExchange.NewInvalidFieldError("user_id", "UserID is invalid")
	}
	if len(orgUser.OrganisationID) == 0 {
		return cigExchange.NewInvalidFieldError("organization_id", "OrganisationID is invalid")
	}

	db := cigExchange.GetDB().Create(orgUser)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Create organization user link call failed", db.Error)
	}
	return nil
}

// Update existing organisation user object in db
func (orgUser *OrganisationUser) Update() *cigExchange.APIError {

	// check that both ID's are set
	if len(orgUser.ID) == 0 {
		return cigExchange.NewInvalidFieldError("id", "ID is invalid")
	}
	if len(orgUser.UserID) == 0 {
		return cigExchange.NewInvalidFieldError("user_id", "UserID is invalid")
	}
	if len(orgUser.OrganisationID) == 0 {
		return cigExchange.NewInvalidFieldError("organization_id", "OrganisationID is invalid")
	}

	// update OrganisationUser
	err := cigExchange.GetDB().Save(orgUser).Error
	if err != nil {
		return cigExchange.NewDatabaseError("Failed to update organisation user ", err)
	}
	return nil
}

// Find queries organisation user from db
func (orgUser *OrganisationUser) Find() (organisationUser *OrganisationUser, apiError *cigExchange.APIError) {

	organisationUser = &OrganisationUser{}
	db := cigExchange.GetDB().Where(orgUser).First(organisationUser)
	if db.Error != nil {
		if db.RecordNotFound() {
			return nil, cigExchange.NewOrganisationUserDoesntExistError("Organisation User with provided parameters doesn't exist")
		}
		return nil, cigExchange.NewDatabaseError("Organisation Users lookup failed", db.Error)
	}
	return organisationUser, nil
}

// Delete existing user organisation object in db
func (orgUser *OrganisationUser) Delete() *cigExchange.APIError {

	// check that UUID is set
	if len(orgUser.ID) == 0 {
		return cigExchange.NewInvalidFieldError("id", "Invalid organisation user id")
	}

	// remove previous token from redis
	redisKey := orgUser.UserID + "|" + orgUser.OrganisationID

	intRedisCmd := cigExchange.GetRedis().Del(redisKey)
	if intRedisCmd.Err() != nil {
		return cigExchange.NewRedisError("Del token failure", intRedisCmd.Err())
	}

	db := cigExchange.GetDB().Delete(orgUser)
	if db.Error != nil {
		return cigExchange.NewDatabaseError("Error deleting organisation user", db.Error)
	}
	if db.RowsAffected == 0 {
		return cigExchange.NewInvalidFieldError("organisation_id, user_id", "Organisation User doesn't exist")
	}
	return nil
}

// GetOrganisationUsersForOrganisation queries all organisation users for organisation from db
func GetOrganisationUsersForOrganisation(organisationID string) (orgUsers []*OrganisationUser, apiErr *cigExchange.APIError) {

	orgUsers = make([]*OrganisationUser, 0)
	// find all organisationUser objects for organisation
	db := cigExchange.GetDB().Where(&OrganisationUser{OrganisationID: organisationID}).Find(&orgUsers)
	if db.Error != nil {
		if !db.RecordNotFound() {
			apiErr = cigExchange.NewDatabaseError("Organisation Users lookup failed", db.Error)
		}
	}
	return
}

// OrganisationUserResponse used in response for organisation/{organisation_id}/users api call
type OrganisationUserResponse struct {
	*User
	UserEmail string     `json:"email"`
	LastLogin *time.Time `json:"last_login,omitempty"`
}

// GetUsersForOrganisation queries all users for organisation from db
func GetUsersForOrganisation(organisationID string, invitedUsers bool) (usersResponse []*OrganisationUserResponse, apiErr *cigExchange.APIError) {

	usersResponse = make([]*OrganisationUserResponse, 0)
	var orgUsers []OrganisationUser

	// find all organisationUser objects for organisation
	db := cigExchange.GetDB().Where(&OrganisationUser{OrganisationID: organisationID}).Find(&orgUsers)
	if db.Error != nil {
		if !db.RecordNotFound() {
			apiErr = cigExchange.NewDatabaseError("Organisation Users lookup failed", db.Error)
		}
		return
	}

	for _, orgUser := range orgUsers {
		if invitedUsers {
			// return only list of invited users
			if orgUser.Status != OrganisationUserStatusInvited {
				continue
			}
		} else {
			// return only list of active users
			if orgUser.Status != OrganisationUserStatusActive {
				continue
			}
		}
		// get user with login email
		var user User
		db = cigExchange.GetDB().Preload("LoginEmail").Where(&User{ID: orgUser.UserID}).First(&user)
		if db.Error != nil {
			if db.RecordNotFound() {
				continue
			}
			apiErr = cigExchange.NewDatabaseError("User lookup failed", db.Error)
			return
		}
		if user.LoginEmail == nil || len(user.LoginEmail.Value1) == 0 {
			apiErr = cigExchange.NewDatabaseError("Invalid login email", db.Error)
			return
		}

		var lastLogin time.Time

		// get last login for user
		row := cigExchange.GetDB().Model(&UserActivity{}).Select("updated_at").Where("user_id = ? and type = ?", user.ID, ActivityTypeSessionLength).Row()

		err := row.Scan(&lastLogin)
		if err != nil {
			if err != sql.ErrNoRows {
				fmt.Println(cigExchange.NewDatabaseError("Last login error: ", err).ToString())
				return
			}
		}
		var lastLoginP *time.Time
		if !lastLogin.IsZero() {
			lastLoginP = &lastLogin
		}

		// fill response struct
		userResponse := &OrganisationUserResponse{&user, user.LoginEmail.Value1, lastLoginP}
		// add user to response
		usersResponse = append(usersResponse, userResponse)
	}

	return
}

// DeleteExpiredInvitations deletes expired invitations
func DeleteExpiredInvitations() {

	// delete invited user with updated_at < now() - interval '30 days'
	db := cigExchange.GetDB().Where("status = ? and updated_at < now() - interval '30 days'", OrganisationUserStatusInvited).Delete(&OrganisationUser{})
	if db.Error != nil {
		log.Printf("Failed to delete invited users with error: %v\n", db.Error.Error())
		return
	}
	log.Printf("%d invitations deleted\n", db.RowsAffected)
}

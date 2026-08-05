package model

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const UserNameMaxLength = 20

// User if you add sensitive fields, don't forget to clean them in setupLogin function.
// Otherwise, the sensitive information will be saved on local storage in plain text!
type User struct {
	Id               int            `json:"id"`
	Username         string         `json:"username" gorm:"unique;index" validate:"max=20"`
	Password         string         `json:"password" gorm:"not null;" validate:"min=8,max=20"`
	OriginalPassword string         `json:"original_password" gorm:"-:all"` // this field is only for Password change verification, don't save it to database!
	DisplayName      string         `json:"display_name" gorm:"index" validate:"max=20"`
	Role             int            `json:"role" gorm:"type:int;default:1"`   // admin, common
	Status           int            `json:"status" gorm:"type:int;default:1"` // enabled, disabled
	Email            string         `json:"email" gorm:"index" validate:"max=50"`
	GitHubId         string         `json:"github_id" gorm:"column:github_id;index"`
	DiscordId        string         `json:"discord_id" gorm:"column:discord_id;index"`
	OidcId           string         `json:"oidc_id" gorm:"column:oidc_id;index"`
	WeChatId         string         `json:"wechat_id" gorm:"column:wechat_id;index"`
	TelegramId       string         `json:"telegram_id" gorm:"column:telegram_id;index"`
	VerificationCode string         `json:"verification_code" gorm:"-:all"`                         // this field is only for Email verification, don't save it to database!
	AccessToken      *string        `json:"-" gorm:"type:char(32);column:access_token;uniqueIndex"` // this token is for system management
	Quota            int            `json:"quota" gorm:"type:int;default:0"`
	UsedQuota        int            `json:"used_quota" gorm:"type:int;default:0;column:used_quota"` // used quota
	RequestCount     int            `json:"request_count" gorm:"type:int;default:0;"`               // request number
	Group            string         `json:"group" gorm:"type:varchar(64);default:'default'"`
	GroupId          int            `json:"group_id" gorm:"index;default:0"`
	GroupName        string         `json:"group_name" gorm:"-"`
	AffCode          string         `json:"aff_code" gorm:"type:varchar(32);column:aff_code;uniqueIndex"`
	AffCount         int            `json:"aff_count" gorm:"type:int;default:0;column:aff_count"`
	AffQuota         int            `json:"aff_quota" gorm:"type:int;default:0;column:aff_quota"`           // 邀请剩余额度
	AffHistoryQuota  int            `json:"aff_history_quota" gorm:"type:int;default:0;column:aff_history"` // 邀请历史额度
	InviterId        int            `json:"inviter_id" gorm:"type:int;column:inviter_id;index"`
	DeletedAt        gorm.DeletedAt `gorm:"index"`
	LinuxDOId        string         `json:"linux_do_id" gorm:"column:linux_do_id;index"`
	Setting          string         `json:"setting" gorm:"type:text;column:setting"`
	Remark           string         `json:"remark,omitempty" gorm:"type:varchar(255)" validate:"max=255"`
	StripeCustomer   string         `json:"stripe_customer" gorm:"type:varchar(64);column:stripe_customer;index"`
	CreatedAt        int64          `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	LastLoginAt      int64          `json:"last_login_at" gorm:"default:0;column:last_login_at"`
	// CyberPolicyCountResetEventId 记录最近一次自动封禁时的累计重置事件。
	// 审计事件继续保留，恢复账号后只统计该事件之后的新事件。
	CyberPolicyCountResetEventId int64 `json:"-" gorm:"default:0;column:cyber_policy_count_reset_event_id"`
}

func applyUserGroupNames(users []*User, groupNames map[string]string) {
	for _, user := range users {
		if user == nil {
			continue
		}
		group := strings.TrimSpace(user.Group)
		if group == "" {
			continue
		}
		user.GroupName = group
		if name := strings.TrimSpace(groupNames[group]); name != "" {
			user.GroupName = name
		}
	}
}

func FillUserGroupNames(users ...*User) {
	groupNames, err := GetGroupDisplayNameMap()
	if err != nil {
		groupNames = map[string]string{}
	}
	applyUserGroupNames(users, groupNames)
}

func (user *User) ToBaseUser() *UserBase {
	cache := &UserBase{
		Id:       user.Id,
		Group:    user.Group,
		GroupId:  user.GroupId,
		Quota:    user.Quota,
		Status:   user.Status,
		Role:     user.Role,
		Username: user.Username,
		Setting:  user.Setting,
		Email:    user.Email,
	}
	return cache
}

func (user *User) GetAccessToken() string {
	if user.AccessToken == nil {
		return ""
	}
	return *user.AccessToken
}

func (user *User) SetAccessToken(token string) {
	user.AccessToken = &token
}

func (user *User) GetSetting() dto.UserSetting {
	setting := dto.UserSetting{}
	if user.Setting != "" {
		err := common.Unmarshal([]byte(user.Setting), &setting)
		if err != nil {
			common.SysLog("failed to unmarshal setting: " + err.Error())
		}
	}
	return setting
}

func (user *User) SetSetting(setting dto.UserSetting) {
	settingBytes, err := common.Marshal(setting)
	if err != nil {
		common.SysLog("failed to marshal setting: " + err.Error())
		return
	}
	user.Setting = string(settingBytes)
}

// whereExactText 使用字节精确语义比较受并发控制保护的文本列。
// MySQL 常见的默认排序规则不区分大小写，普通等号不能作为可靠的 CAS 条件。
func whereExactText(query *gorm.DB, column string, value string) *gorm.DB {
	if common.UsingMySQL {
		return query.Where("BINARY "+column+" = BINARY ?", value)
	}
	return query.Where(column+" = ?", value)
}

func whereExactTextOrNull(query *gorm.DB, column string, value string) *gorm.DB {
	if common.UsingMySQL {
		return query.Where("(BINARY "+column+" = BINARY ? OR "+column+" IS NULL)", value)
	}
	return query.Where("("+column+" = ? OR "+column+" IS NULL)", value)
}

func updateExistingUserColumn(userId int, column string, value any) error {
	if userId == 0 {
		return errors.New("id 为空！")
	}
	result := DB.Model(&User{}).Where("id = ?", userId).Update(column, value)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 0 {
		return nil
	}

	// MySQL 更新为相同值时可能报告 0 行；复核目标仍存在后按成功处理。
	var count int64
	if err := DB.Model(&User{}).Where("id = ?", userId).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateUserSettingColumn 只更新用户设置列，避免资料快照覆盖并发计费字段。
func UpdateUserSettingColumn(userId int, setting dto.UserSetting) error {
	settingBytes, err := common.Marshal(setting)
	if err != nil {
		return err
	}
	settingValue := string(settingBytes)
	if err = updateExistingUserColumn(userId, "setting", settingValue); err != nil {
		return err
	}
	return invalidateUserCachePreservingQuota(userId)
}

// MutateUserSetting 使用乐观并发控制修改设置，回调可能因冲突被重复调用。
func MutateUserSetting(userId int, mutate func(*dto.UserSetting) error) error {
	return mutateUserSettingWithInvalidation(userId, mutate, invalidateUserCachePreservingQuota)
}

func mutateUserSettingWithInvalidation(
	userId int,
	mutate func(*dto.UserSetting) error,
	invalidate func(userId int) error,
) error {
	if userId == 0 {
		return errors.New("id 为空！")
	}
	if mutate == nil {
		return errors.New("设置修改函数为空")
	}
	if invalidate == nil {
		return errors.New("设置缓存失效函数为空")
	}

	const maxAttempts = 8
	for attempt := 0; attempt < maxAttempts; attempt++ {
		var current User
		if err := DB.Select("id", "setting").First(&current, userId).Error; err != nil {
			return err
		}

		settings := dto.UserSetting{}
		if current.Setting != "" {
			if err := common.Unmarshal([]byte(current.Setting), &settings); err != nil {
				return err
			}
		}
		if err := mutate(&settings); err != nil {
			return err
		}
		settingBytes, err := common.Marshal(settings)
		if err != nil {
			return err
		}
		settingValue := string(settingBytes)
		if settingValue == current.Setting {
			// 上一次写库可能已成功、但缓存失效失败。相同请求重试时
			// 仍需再次失效，否则旧设置会一直保留到 Redis TTL 结束。
			return invalidate(userId)
		}

		query := DB.Model(&User{}).Where("id = ?", userId)
		if current.Setting == "" {
			query = whereExactTextOrNull(query, "setting", "")
		} else {
			query = whereExactText(query, "setting", current.Setting)
		}
		result := query.Update("setting", settingValue)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			return invalidate(userId)
		}
	}

	return errors.New("用户设置并发更新冲突，请重试")
}

// UpdateUserAccessTokenColumn 只更新管理访问令牌，不写回完整用户快照。
func UpdateUserAccessTokenColumn(userId int, accessToken string) error {
	return updateExistingUserColumn(userId, "access_token", accessToken)
}

// UpdateUserAffiliateCodeColumn 只更新邀请码，避免邀请资料写回旧用户快照。
func UpdateUserAffiliateCodeColumn(userId int, affiliateCode string) error {
	return updateExistingUserColumn(userId, "aff_code", affiliateCode)
}

// UpdateUserEmailColumn 只更新邮箱，并使包含旧邮箱的资料缓存失效。
func UpdateUserEmailColumn(userId int, email string) error {
	if err := updateExistingUserColumn(userId, "email", email); err != nil {
		return err
	}
	return invalidateUserCachePreservingQuota(userId)
}

// UpdateUserBuiltinOAuthBindingColumn 只允许更新内置 OAuth 绑定列。
func UpdateUserBuiltinOAuthBindingColumn(userId int, bindingType string, providerUserId string) error {
	bindingColumnMap := map[string]string{
		"github":   "github_id",
		"discord":  "discord_id",
		"oidc":     "oidc_id",
		"wechat":   "wechat_id",
		"telegram": "telegram_id",
		"linuxdo":  "linux_do_id",
	}
	column, ok := bindingColumnMap[bindingType]
	if !ok {
		return errors.New("invalid binding type")
	}
	return updateExistingUserColumn(userId, column, providerUserId)
}

// UpdateUserStatusColumn 只更新账号状态，并使旧资料缓存失效。
func UpdateUserStatusColumn(userId int, status int) error {
	if err := updateExistingUserColumn(userId, "status", status); err != nil {
		return err
	}
	return invalidateUserCachePreservingQuota(userId)
}

// UpdateUserRoleColumn 只更新账号角色，并使旧资料缓存失效。
func UpdateUserRoleColumn(userId int, role int) error {
	if err := updateExistingUserColumn(userId, "role", role); err != nil {
		return err
	}
	return invalidateUserCachePreservingQuota(userId)
}

// 根据用户角色生成默认的边栏配置
func generateDefaultSidebarConfigForRole(userRole int) string {
	defaultConfig := map[string]any{}

	// 聊天区域 - 所有用户都可以访问
	defaultConfig["chat"] = map[string]any{
		"enabled":    true,
		"playground": true,
		"canvas":     true,
		"chat":       true,
	}

	// 控制台区域 - 所有用户都可以访问
	defaultConfig["console"] = map[string]any{
		"enabled":     true,
		"detail":      true,
		"token":       true,
		"log":         true,
		"midjourney":  true,
		"task":        true,
		"game_center": true,
	}

	// 个人中心区域 - 所有用户都可以访问
	defaultConfig["personal"] = map[string]any{
		"enabled":   true,
		"topup":     true,
		"affiliate": true,
		"invoice":   true,
		"personal":  true,
	}

	// 管理员区域 - 根据角色决定
	if userRole == common.RoleAdminUser {
		// 管理员可以访问管理员区域，但不能访问系统设置
		defaultConfig["admin"] = map[string]any{
			"enabled":         true,
			"channel":         true,
			"models":          true,
			"deployment":      true,
			"redemption":      true,
			"subscription":    true,
			"game_management": true,
			"user":            true,
			"invoice_admin":   true,
			"affiliate_admin": false,
			"extension_admin": false,
			"setting":         false, // 管理员不能访问系统设置
		}
	} else if userRole == common.RoleRootUser {
		// 超级管理员可以访问所有功能
		defaultConfig["admin"] = map[string]any{
			"enabled":         true,
			"channel":         true,
			"models":          true,
			"deployment":      true,
			"redemption":      true,
			"subscription":    true,
			"game_management": true,
			"user":            true,
			"invoice_admin":   true,
			"affiliate_admin": true,
			"extension_admin": true,
			"setting":         true,
		}
	}
	// 普通用户不包含admin区域

	// 转换为JSON字符串
	configBytes, err := common.Marshal(defaultConfig)
	if err != nil {
		common.SysLog("生成默认边栏配置失败: " + err.Error())
		return ""
	}

	return string(configBytes)
}

// CheckUserExistOrDeleted check if user exist or deleted, if not exist, return false, nil, if deleted or exist, return true, nil
func CheckUserExistOrDeleted(username string, email string) (bool, error) {
	var user User

	// err := DB.Unscoped().First(&user, "username = ? or email = ?", username, email).Error
	// check email if empty
	var err error
	if email == "" {
		err = DB.Unscoped().First(&user, "username = ?", username).Error
	} else {
		err = DB.Unscoped().First(&user, "username = ? or email = ?", username, email).Error
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// not exist, return false, nil
			return false, nil
		}
		// other error, return false, err
		return false, err
	}
	// exist, return true, nil
	return true, nil
}

func GetMaxUserId() int {
	var user User
	DB.Unscoped().Last(&user)
	return user.Id
}

func GetAllUsers(pageInfo *common.PageInfo) (users []*User, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Get total count within transaction
	err = tx.Unscoped().Model(&User{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated users within same transaction
	err = tx.Unscoped().Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Omit("password", "access_token").Find(&users).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	FillUserGroupNames(users...)

	return users, total, nil
}

func SearchUsers(keyword string, group string, role *int, status *int, startIdx int, num int, searchTypes ...string) ([]*User, int64, error) {
	var users []*User
	var total int64
	var err error

	// 开始事务
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 构建基础查询
	query := tx.Unscoped().Model(&User{})

	keyword = strings.TrimSpace(keyword)
	searchType := "all"
	if len(searchTypes) > 0 {
		searchType = strings.ToLower(strings.TrimSpace(searchTypes[0]))
	}
	likeKeyword := "%" + keyword + "%"
	exactUserID := 0
	hasExactUserID := false
	if keyword != "" {
		switch searchType {
		case "id":
			keywordInt, parseErr := strconv.Atoi(keyword)
			if parseErr != nil || keywordInt <= 0 {
				query = query.Where("1 = 0")
			} else {
				query = query.Where("id = ?", keywordInt)
			}
		case "username":
			query = query.Where("username LIKE ?", likeKeyword)
		default:
			likeCondition := "username LIKE ? OR email LIKE ? OR display_name LIKE ?"
			likeArgs := []interface{}{likeKeyword, likeKeyword, likeKeyword}
			if keywordInt, parseErr := strconv.Atoi(keyword); parseErr == nil && keywordInt > 0 {
				likeCondition = "id = ? OR " + likeCondition
				likeArgs = append([]interface{}{keywordInt}, likeArgs...)
				exactUserID = keywordInt
				hasExactUserID = true
			}
			query = query.Where("("+likeCondition+")", likeArgs...)
		}
	}
	if group != "" {
		query = query.Where(commonGroupCol+" = ?", group)
	}
	if role != nil {
		query = query.Where("role = ?", *role)
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 数字搜索保留原有匹配范围，但把精确 ID 放在最前，避免目标用户被挤出当前页。
	if hasExactUserID {
		query = query.Order(clause.OrderBy{Expression: clause.Expr{
			SQL:                "CASE WHEN id = ? THEN 0 ELSE 1 END, id DESC",
			Vars:               []interface{}{exactUserID},
			WithoutParentheses: true,
		}})
	} else {
		query = query.Order("id desc")
	}

	// 获取分页数据
	err = query.Omit("password", "access_token").Limit(num).Offset(startIdx).Find(&users).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	FillUserGroupNames(users...)

	return users, total, nil
}

func GetUserById(id int, selectAll bool) (*User, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	user := User{Id: id}
	var err error = nil
	if selectAll {
		err = DB.First(&user, "id = ?", id).Error
	} else {
		err = DB.Omit("password", "access_token").First(&user, "id = ?", id).Error
	}
	return &user, err
}

func GetUserIdByAffCode(affCode string) (int, error) {
	if affCode == "" {
		return 0, errors.New("affCode 为空！")
	}
	var user User
	err := DB.Select("id").First(&user, "aff_code = ?", affCode).Error
	if err == nil && IsAffiliateUserInviteCodeBlocked(user.Id) {
		return 0, errors.New("该邀请码已失效")
	}
	return user.Id, err
}

// getActiveInviterIdByAffCodeWithDB 用于邀请制注册准入，要求邀请人仍启用，且风控状态可确认。
func getActiveInviterIdByAffCodeWithDB(db *gorm.DB, affCode string, forUpdate bool) (int, error) {
	if db == nil {
		return 0, errors.New("database is nil")
	}
	affCode = strings.TrimSpace(affCode)
	if affCode == "" {
		return 0, errors.New("affCode 为空！")
	}
	var user User
	query := db.Select("id")
	if forUpdate {
		query = lockForUpdate(query)
	}
	if err := query.
		Where("aff_code = ? AND status = ?", affCode, common.UserStatusEnabled).
		First(&user).Error; err != nil {
		return 0, err
	}
	blocked, err := queryAffiliateUserInviteCodeBlockedWithDB(db, user.Id)
	if err != nil {
		return 0, err
	}
	if blocked {
		return 0, errors.New("该邀请码已失效")
	}
	return user.Id, nil
}

func GetActiveInviterIdByAffCodeWithDB(db *gorm.DB, affCode string) (int, error) {
	return getActiveInviterIdByAffCodeWithDB(db, affCode, false)
}

// GetActiveInviterIdByAffCodeForUpdateWithDB 锁定邀请人，串行化注册与邀请码撤销。
func GetActiveInviterIdByAffCodeForUpdateWithDB(db *gorm.DB, affCode string) (int, error) {
	return getActiveInviterIdByAffCodeWithDB(db, affCode, true)
}

func GetActiveInviterIdByAffCode(affCode string) (int, error) {
	return GetActiveInviterIdByAffCodeWithDB(DB, affCode)
}

func DeleteUserById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	user := User{Id: id}
	return user.Delete()
}

func HardDeleteUserById(id int) error {
	if id == 0 {
		return errors.New("id 为空！")
	}
	user := User{Id: id}
	return user.HardDelete()
}

func inviteUser(inviterId int) (err error) {
	return DB.Model(&User{}).Where("id = ?", inviterId).
		Update("aff_count", gorm.Expr("aff_count + ?", 1)).Error
}

func (user *User) TransferAffQuotaToQuota(quota int) error {
	// 检查quota是否小于最小额度
	if float64(quota) < common.QuotaPerUnit {
		return fmt.Errorf("转移额度最小为%s！", logger.LogQuota(int(common.QuotaPerUnit)))
	}

	// 开始数据库事务
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback() // 确保在函数退出时事务能回滚

	// 加锁查询用户以确保数据一致性
	err := lockForUpdate(tx).First(&user, user.Id).Error
	if err != nil {
		return err
	}

	// 再次检查用户的AffQuota是否足够
	if user.AffQuota < quota {
		return errors.New("邀请额度不足！")
	}

	// 更新用户额度
	user.AffQuota -= quota
	user.Quota += quota

	// 保存用户状态
	if err := tx.Save(user).Error; err != nil {
		return err
	}

	// 提交事务
	return tx.Commit().Error
}

func (user *User) Insert(inviterId int) error {
	var err error
	if user.Password != "" {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}
	user.Quota = common.QuotaForNewUser
	//user.SetAccessToken(common.GetUUID())
	user.AffCode = common.GetRandomString(4)
	user.InviterId = inviterId
	if groupID, groupErr := ResolveGroupIDByCode(user.Group); groupErr == nil {
		user.GroupId = groupID
	}

	// 初始化用户设置，包括默认的边栏配置
	if user.Setting == "" {
		defaultSetting := dto.UserSetting{}
		// 这里暂时不设置SidebarModules，因为需要在用户创建后根据角色设置
		user.SetSetting(defaultSetting)
	}

	result := DB.Create(user)
	if result.Error != nil {
		return result.Error
	}

	// 用户创建成功后，根据角色初始化边栏配置
	// 需要重新获取用户以确保有正确的ID和Role
	var createdUser User
	if err := DB.Where("username = ?", user.Username).First(&createdUser).Error; err == nil {
		// 生成基于角色的默认边栏配置
		defaultSidebarConfig := generateDefaultSidebarConfigForRole(createdUser.Role)
		if defaultSidebarConfig != "" {
			currentSetting := createdUser.GetSetting()
			currentSetting.SidebarModules = defaultSidebarConfig
			if err := UpdateUserSettingColumn(createdUser.Id, currentSetting); err != nil {
				common.SysLog(fmt.Sprintf("为新用户 %s 初始化边栏配置失败: %v", createdUser.Username, err))
			} else {
				common.SysLog(fmt.Sprintf("为新用户 %s (角色: %d) 初始化边栏配置", createdUser.Username, createdUser.Role))
			}
		}
	}

	if common.QuotaForNewUser > 0 {
		RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("新用户注册赠送 %s", logger.LogQuota(common.QuotaForNewUser)))
	}
	if inviterId != 0 && operation_setting.IsPaymentComplianceConfirmed() {
		if common.QuotaForInvitee > 0 {
			_ = IncreaseUserQuota(user.Id, common.QuotaForInvitee, true)
			RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("使用邀请码赠送 %s", logger.LogQuota(common.QuotaForInvitee)))
		}
		_ = inviteUser(inviterId)
	}
	return nil
}

// InsertWithTx inserts a new user within an existing transaction.
// This is used for OAuth registration where user creation and binding need to be atomic.
// Post-creation tasks (sidebar config, logs, inviter rewards) are handled after the transaction commits.
func (user *User) InsertWithTx(tx *gorm.DB, inviterId int) error {
	var err error
	if user.Password != "" {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
	}
	user.Quota = common.QuotaForNewUser
	user.AffCode = common.GetRandomString(4)
	user.InviterId = inviterId
	if groupID, groupErr := ResolveGroupIDByCodeWithDB(tx, user.Group); groupErr == nil {
		user.GroupId = groupID
	}

	// 初始化用户设置
	if user.Setting == "" {
		defaultSetting := dto.UserSetting{}
		user.SetSetting(defaultSetting)
	}

	result := tx.Create(user)
	if result.Error != nil {
		return result.Error
	}

	return nil
}

// FinalizeOAuthUserCreation performs post-transaction tasks for OAuth user creation.
// This should be called after the transaction commits successfully.
func (user *User) FinalizeOAuthUserCreation(inviterId int) {
	// 用户创建成功后，根据角色初始化边栏配置
	var createdUser User
	if err := DB.Where("id = ?", user.Id).First(&createdUser).Error; err == nil {
		defaultSidebarConfig := generateDefaultSidebarConfigForRole(createdUser.Role)
		if defaultSidebarConfig != "" {
			currentSetting := createdUser.GetSetting()
			currentSetting.SidebarModules = defaultSidebarConfig
			if err := UpdateUserSettingColumn(createdUser.Id, currentSetting); err != nil {
				common.SysLog(fmt.Sprintf("为新用户 %s 初始化边栏配置失败: %v", createdUser.Username, err))
			} else {
				common.SysLog(fmt.Sprintf("为新用户 %s (角色: %d) 初始化边栏配置", createdUser.Username, createdUser.Role))
			}
		}
	}

	if common.QuotaForNewUser > 0 {
		RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("新用户注册赠送 %s", logger.LogQuota(common.QuotaForNewUser)))
	}
	if inviterId != 0 && operation_setting.IsPaymentComplianceConfirmed() {
		if common.QuotaForInvitee > 0 {
			_ = IncreaseUserQuota(user.Id, common.QuotaForInvitee, true)
			RecordLog(user.Id, LogTypeSystem, fmt.Sprintf("使用邀请码赠送 %s", logger.LogQuota(common.QuotaForInvitee)))
		}
		_ = inviteUser(inviterId)
	}
}

func (user *User) Update(updatePassword bool) error {
	var err error
	updates := map[string]interface{}{}
	if updatePassword {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
		updates["password"] = user.Password
	}
	if user.Username != "" {
		updates["username"] = user.Username
	}
	if user.DisplayName != "" {
		updates["display_name"] = user.DisplayName
	}

	current := User{}
	if err = DB.First(&current, user.Id).Error; err != nil {
		return err
	}
	// 通用资料更新只允许修改用户名、显示名和显式验证后的密码。
	if len(updates) > 0 {
		err = DB.Model(&current).Updates(updates).Error
	}
	if err != nil {
		return err
	}
	if err = DB.First(user, user.Id).Error; err != nil {
		return err
	}

	return invalidateUserCachePreservingQuota(user.Id)
}

type UserEditOptions struct {
	Username      bool
	DisplayName   bool
	Role          bool
	GuardRole     bool
	Group         bool
	Remark        bool
	ExpectedRole  int
	ExpectedGroup string
}

func (user *User) Edit(updatePassword bool, options UserEditOptions) error {
	var err error
	updates := map[string]interface{}{}
	if updatePassword {
		user.Password, err = common.Password2Hash(user.Password)
		if err != nil {
			return err
		}
		updates["password"] = user.Password
	}
	if options.Username {
		updates["username"] = user.Username
	}
	if options.DisplayName {
		updates["display_name"] = user.DisplayName
	}
	if options.Role {
		updates["role"] = user.Role
	}
	if options.Group {
		groupID, groupErr := ResolveGroupIDByCode(user.Group)
		if groupErr != nil {
			return groupErr
		}
		updates["group"] = user.Group
		updates["group_id"] = groupID
	}
	if options.Remark {
		updates["remark"] = user.Remark
	}
	if len(updates) == 0 {
		return nil
	}

	query := DB.Model(&User{}).Where("id = ?", user.Id)
	guardRole := options.GuardRole || options.Role
	if guardRole {
		query = query.Where("role = ?", options.ExpectedRole)
	}
	if options.Group {
		query = whereExactText(query, commonGroupCol, options.ExpectedGroup)
	}
	result := query.Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 && (guardRole || options.Group) {
		var current User
		if err = DB.Select("role", commonGroupCol).First(&current, user.Id).Error; err != nil {
			return err
		}
		if (guardRole && current.Role != options.ExpectedRole) ||
			(options.Group && current.Group != options.ExpectedGroup) {
			return errors.New("用户角色或分组已被并发修改，请刷新后重试")
		}
		if (options.Role && current.Role != user.Role) ||
			(options.Group && current.Group != user.Group) {
			return errors.New("用户角色或分组更新未生效，请重试")
		}
	}

	return invalidateUserCachePreservingQuota(user.Id)
}

func (user *User) ClearBinding(bindingType string) error {
	if user.Id == 0 {
		return errors.New("user id is empty")
	}

	bindingColumnMap := map[string]string{
		"email":    "email",
		"github":   "github_id",
		"discord":  "discord_id",
		"oidc":     "oidc_id",
		"wechat":   "wechat_id",
		"telegram": "telegram_id",
		"linuxdo":  "linux_do_id",
	}

	column, ok := bindingColumnMap[bindingType]
	if !ok {
		return errors.New("invalid binding type")
	}

	if err := DB.Model(&User{}).Where("id = ?", user.Id).Update(column, "").Error; err != nil {
		return err
	}

	if err := DB.Where("id = ?", user.Id).First(user).Error; err != nil {
		return err
	}

	return invalidateUserCachePreservingQuota(user.Id)
}

func (user *User) Delete() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	if err := DB.Delete(user).Error; err != nil {
		return err
	}

	// 清除缓存
	return invalidateUserCache(user.Id)
}

func (user *User) HardDelete() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	var tokens []Token
	err := DB.Transaction(func(tx *gorm.DB) error {
		if common.RedisEnabled {
			if err := tx.Unscoped().
				Select("id", commonKeyCol).
				Where("user_id = ?", user.Id).
				Find(&tokens).Error; err != nil {
				return err
			}
		}
		if err := deleteUserAuthenticationData(tx, user.Id); err != nil {
			return err
		}
		return tx.Unscoped().Delete(user).Error
	})
	if err != nil {
		return err
	}
	if err := invalidateTokensCache(tokens); err != nil {
		common.SysError(fmt.Sprintf("failed to invalidate token cache after hard deleting user %d: %v", user.Id, err))
	}
	if err := invalidateUserCache(user.Id); err != nil {
		common.SysError(fmt.Sprintf("failed to invalidate user cache after hard deleting user %d: %v", user.Id, err))
	}
	return nil
}

func deleteUserAuthenticationData(tx *gorm.DB, userId int) error {
	for _, authenticationData := range []any{
		&TwoFABackupCode{},
		&TwoFA{},
		&PasskeyCredential{},
		&Token{},
		&UserOAuthBinding{},
	} {
		if err := tx.Unscoped().Where("user_id = ?", userId).Delete(authenticationData).Error; err != nil {
			return err
		}
	}
	return nil
}

// ValidateAndFill check password & user status
func (user *User) ValidateAndFill() (err error) {
	// When querying with struct, GORM will only query with non-zero fields,
	// that means if your field's value is 0, '', false or other zero values,
	// it won't be used to build query conditions
	password := user.Password
	username := strings.TrimSpace(user.Username)
	if username == "" || password == "" {
		return ErrUserEmptyCredentials
	}
	// find by username or email
	err = DB.Where("username = ? OR email = ?", username, username).First(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	okay := common.ValidatePasswordAndHash(password, user.Password)
	if !okay || user.Status != common.UserStatusEnabled {
		return ErrInvalidCredentials
	}
	return nil
}

func (user *User) FillUserById() error {
	if user.Id == 0 {
		return errors.New("id 为空！")
	}
	return DB.Where(User{Id: user.Id}).First(user).Error
}

func (user *User) FillUserByEmail() error {
	if user.Email == "" {
		return errors.New("email 为空！")
	}
	DB.Where(User{Email: user.Email}).First(user)
	return nil
}

func (user *User) FillUserByGitHubId() error {
	if user.GitHubId == "" {
		return errors.New("GitHub id 为空！")
	}
	DB.Where(User{GitHubId: user.GitHubId}).First(user)
	return nil
}

// UpdateGitHubId updates the user's GitHub ID (used for migration from login to numeric ID)
func (user *User) UpdateGitHubId(newGitHubId string) error {
	if user.Id == 0 {
		return errors.New("user id is empty")
	}
	return DB.Model(user).Update("github_id", newGitHubId).Error
}

func (user *User) FillUserByDiscordId() error {
	if user.DiscordId == "" {
		return errors.New("discord id 为空！")
	}
	DB.Where(User{DiscordId: user.DiscordId}).First(user)
	return nil
}

func (user *User) FillUserByOidcId() error {
	if user.OidcId == "" {
		return errors.New("oidc id 为空！")
	}
	DB.Where(User{OidcId: user.OidcId}).First(user)
	return nil
}

func (user *User) FillUserByWeChatId() error {
	if user.WeChatId == "" {
		return errors.New("WeChat id 为空！")
	}
	DB.Where(User{WeChatId: user.WeChatId}).First(user)
	return nil
}

func (user *User) FillUserByTelegramId() error {
	if user.TelegramId == "" {
		return errors.New("Telegram id 为空！")
	}
	err := DB.Where(User{TelegramId: user.TelegramId}).First(user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("该 Telegram 账户未绑定")
	}
	return err
}

func IsEmailAlreadyTaken(email string) bool {
	return DB.Unscoped().Where("email = ?", email).Find(&User{}).RowsAffected == 1
}

func IsWeChatIdAlreadyTaken(wechatId string) bool {
	return DB.Unscoped().Where("wechat_id = ?", wechatId).Find(&User{}).RowsAffected == 1
}

func IsGitHubIdAlreadyTaken(githubId string) bool {
	return DB.Unscoped().Where("github_id = ?", githubId).Find(&User{}).RowsAffected == 1
}

func IsDiscordIdAlreadyTaken(discordId string) bool {
	return DB.Unscoped().Where("discord_id = ?", discordId).Find(&User{}).RowsAffected == 1
}

func IsOidcIdAlreadyTaken(oidcId string) bool {
	return DB.Where("oidc_id = ?", oidcId).Find(&User{}).RowsAffected == 1
}

func IsTelegramIdAlreadyTaken(telegramId string) bool {
	return DB.Unscoped().Where("telegram_id = ?", telegramId).Find(&User{}).RowsAffected == 1
}

func ResetUserPasswordByEmail(email string, password string) error {
	if email == "" || password == "" {
		return errors.New("邮箱地址或密码为空！")
	}
	hashedPassword, err := common.Password2Hash(password)
	if err != nil {
		return err
	}
	err = DB.Model(&User{}).Where("email = ?", email).Update("password", hashedPassword).Error
	return err
}

func IsAdmin(userId int) bool {
	if userId == 0 {
		return false
	}
	var user User
	err := DB.Where("id = ?", userId).Select("role").Find(&user).Error
	if err != nil {
		common.SysLog("no such user " + err.Error())
		return false
	}
	return user.Role >= common.RoleAdminUser
}

//// IsUserEnabled checks user status from Redis first, falls back to DB if needed
//func IsUserEnabled(id int, fromDB bool) (status bool, err error) {
//	defer func() {
//		// Update Redis cache asynchronously on successful DB read
//		if shouldUpdateRedis(fromDB, err) {
//			gopool.Go(func() {
//				if err := updateUserStatusCache(id, status); err != nil {
//					common.SysError("failed to update user status cache: " + err.Error())
//				}
//			})
//		}
//	}()
//	if !fromDB && common.RedisEnabled {
//		// Try Redis first
//		status, err := getUserStatusCache(id)
//		if err == nil {
//			return status == common.UserStatusEnabled, nil
//		}
//		// Don't return error - fall through to DB
//	}
//	fromDB = true
//	var user User
//	err = DB.Where("id = ?", id).Select("status").Find(&user).Error
//	if err != nil {
//		return false, err
//	}
//
//	return user.Status == common.UserStatusEnabled, nil
//}

func ValidateAccessToken(token string) (*User, error) {
	if token == "" {
		return nil, nil
	}
	token = strings.Replace(token, "Bearer ", "", 1)
	user := &User{}
	err := DB.Where("access_token = ?", token).First(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	return user, nil
}

// GetUserQuota gets quota from Redis first, falls back to DB if needed
func GetUserQuota(id int, fromDB bool) (quota int, err error) {
	if hasUserQuotaDeferredFallback(id) {
		return 0, fmt.Errorf("%w: 本实例仍有待落库额度", ErrUserQuotaCacheSync)
	}
	if fromDB && userQuotaDatabaseSnapshotRequiresFence() {
		return 0, fmt.Errorf("%w: 批量计费启用时不能绕过缓存栅栏", ErrUserQuotaCacheSync)
	}
	if !fromDB && common.RedisEnabled {
		quota, err := getUserQuotaCache(id)
		if err == nil {
			if userQuotaCacheReadBlocked(id) {
				return 0, fmt.Errorf("%w: 缓存读取期间正在持久化额度", ErrUserQuotaCacheSync)
			}
			return quota, nil
		}
		if errors.Is(err, ErrUserQuotaCacheSync) {
			return 0, err
		}
		// Don't return error - fall through to DB
	}
	if hasPendingUserQuotaDelta(id) || hasUserQuotaDeferredFallback(id) {
		return 0, fmt.Errorf("%w: 本实例仍有未落库额度", ErrUserQuotaCacheSync)
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select("quota").Find(&quota).Error
	if err != nil {
		return 0, err
	}
	if hasPendingUserQuotaDelta(id) || hasUserQuotaDeferredFallback(id) {
		return 0, fmt.Errorf("%w: 数据库读取期间出现待落库额度", ErrUserQuotaCacheSync)
	}

	return quota, nil
}

func GetUserUsedQuota(id int) (quota int, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("used_quota").Find(&quota).Error
	return quota, err
}

func GetUserEmail(id int) (email string, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("email").Find(&email).Error
	return email, err
}

// GetUserGroup gets group from Redis first, falls back to DB if needed
func GetUserGroup(id int, fromDB bool) (group string, err error) {
	if !fromDB && common.RedisEnabled {
		group, err := getUserGroupCache(id)
		if err == nil {
			return group, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select(commonGroupCol).Find(&group).Error
	if err != nil {
		return "", err
	}

	return group, nil
}

// GetUserSetting gets setting from Redis first, falls back to DB if needed
func GetUserSetting(id int, fromDB bool) (settingMap dto.UserSetting, err error) {
	if !fromDB && common.RedisEnabled {
		var userCache *UserBase
		userCache, err = GetUserCache(id)
		if err == nil {
			return userCache.GetSetting(), nil
		}
		// Don't return error - fall through to DB
	}
	// can be nil setting
	var safeSetting sql.NullString
	err = DB.Model(&User{}).Where("id = ?", id).Select("setting").Find(&safeSetting).Error
	if err != nil {
		return settingMap, err
	}
	if safeSetting.Valid {
		userBase := &UserBase{Setting: safeSetting.String}
		return userBase.GetSetting(), nil
	}
	return settingMap, nil
}

func IncreaseUserQuota(id int, quota int, db bool) (err error) {
	return increaseUserQuotaWithDurability(id, quota, db, userQuotaDeltaAbortable)
}

// IncreaseUserQuotaCommitted 接受已经发生的退款或结算调整。
// 批量模式下即时落库失败时会保留到受保护队列，避免调用方重复退款。
func IncreaseUserQuotaCommitted(id int, quota int) error {
	return increaseUserQuotaWithDurability(id, quota, false, userQuotaDeltaCommitted)
}

func increaseUserQuotaWithDurability(id int, quota int, db bool, durability userQuotaDeltaDurability) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if !db && common.BatchUpdateEnabled {
		applyDelta := applyUserQuotaDeltaWithBatch
		if durability == userQuotaDeltaCommitted {
			applyDelta = applyCommittedUserQuotaDeltaWithBatch
		}
		return applyDelta(
			id,
			quota,
			func() (userQuotaCacheUpdate, error) {
				return tryAdjustUserQuotaCache(id, int64(quota))
			},
			func(delta int) error {
				return increaseUserQuota(id, delta)
			},
			func(lockToken string) error {
				return finishUserQuotaCacheFallback(id, lockToken)
			},
			func(lockToken string) error {
				return ensureUserQuotaCacheFallback(id, lockToken)
			},
			func() error {
				return invalidateUserCache(id)
			},
		)
	}
	if err := increaseUserQuota(id, quota); err != nil {
		return err
	}
	gopool.Go(func() {
		err := cacheIncrUserQuota(id, int64(quota))
		if err != nil {
			common.SysLog("failed to increase user quota: " + err.Error())
		}
	})
	return nil
}

func increaseUserQuota(id int, quota int) (err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota + ?", quota)).Error
	if err != nil {
		return err
	}
	return err
}

func DecreaseUserQuota(id int, quota int, db bool) (err error) {
	return decreaseUserQuotaWithDurability(id, quota, db, userQuotaDeltaAbortable)
}

// DecreaseUserQuotaCommitted 接受已经发生的消费或结算调整。
// 批量模式下即时落库失败时会保留到受保护队列，避免漏扣已完成消费。
func DecreaseUserQuotaCommitted(id int, quota int) error {
	return decreaseUserQuotaWithDurability(id, quota, false, userQuotaDeltaCommitted)
}

func decreaseUserQuotaWithDurability(id int, quota int, db bool, durability userQuotaDeltaDurability) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if !db && common.BatchUpdateEnabled {
		applyDelta := applyUserQuotaDeltaWithBatch
		if durability == userQuotaDeltaCommitted {
			applyDelta = applyCommittedUserQuotaDeltaWithBatch
		}
		return applyDelta(
			id,
			-quota,
			func() (userQuotaCacheUpdate, error) {
				return tryAdjustUserQuotaCache(id, -int64(quota))
			},
			func(delta int) error {
				return increaseUserQuota(id, delta)
			},
			func(lockToken string) error {
				return finishUserQuotaCacheFallback(id, lockToken)
			},
			func(lockToken string) error {
				return ensureUserQuotaCacheFallback(id, lockToken)
			},
			func() error {
				return invalidateUserCache(id)
			},
		)
	}
	if err := decreaseUserQuota(id, quota); err != nil {
		return err
	}
	gopool.Go(func() {
		err := cacheDecrUserQuota(id, int64(quota))
		if err != nil {
			common.SysLog("failed to decrease user quota: " + err.Error())
		}
	})
	return nil
}

func decreaseUserQuota(id int, quota int) (err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota - ?", quota)).Error
	if err != nil {
		return err
	}
	return err
}

func DeltaUpdateUserQuota(id int, delta int) (err error) {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		return IncreaseUserQuota(id, delta, false)
	} else {
		return DecreaseUserQuota(id, -delta, false)
	}
}

//func GetRootUserEmail() (email string) {
//	DB.Model(&User{}).Where("role = ?", common.RoleRootUser).Select("email").Find(&email)
//	return email
//}

func GetRootUser() (user *User) {
	DB.Where("role = ?", common.RoleRootUser).First(&user)
	return user
}

func UpdateUserLastLoginAt(id int) {
	if err := DB.Model(&User{}).Where("id = ?", id).Update("last_login_at", common.GetTimestamp()).Error; err != nil {
		common.SysLog("failed to update user last_login_at: " + err.Error())
	}
}

func UpdateUserUsedQuotaAndRequestCount(id int, quota int) {
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUsedQuota, id, quota)
		addNewRecord(BatchUpdateTypeRequestCount, id, 1)
		return
	}
	updateUserUsedQuotaAndRequestCount(id, quota, 1)
}

func updateUserUsedQuotaAndRequestCount(id int, quota int, count int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"request_count": gorm.Expr("request_count + ?", count),
		},
	).Error
	if err != nil {
		common.SysLog("failed to update user used quota and request count: " + err.Error())
		return
	}

	//// 更新缓存
	//if err := invalidateUserCache(id); err != nil {
	//	common.SysError("failed to invalidate user cache: " + err.Error())
	//}
}

func updateUserUsedQuota(id int, quota int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"used_quota": gorm.Expr("used_quota + ?", quota),
		},
	).Error
	if err != nil {
		common.SysLog("failed to update user used quota: " + err.Error())
	}
}

func updateUserRequestCount(id int, count int) {
	err := DB.Model(&User{}).Where("id = ?", id).Update("request_count", gorm.Expr("request_count + ?", count)).Error
	if err != nil {
		common.SysLog("failed to update user request count: " + err.Error())
	}
}

// GetUsernameById gets username from Redis first, falls back to DB if needed
func GetUsernameById(id int, fromDB bool) (username string, err error) {
	if !fromDB && common.RedisEnabled {
		username, err := getUserNameCache(id)
		if err == nil {
			return username, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select("username").Find(&username).Error
	if err != nil {
		return "", err
	}

	return username, nil
}

func IsLinuxDOIdAlreadyTaken(linuxDOId string) bool {
	var user User
	err := DB.Unscoped().Where("linux_do_id = ?", linuxDOId).First(&user).Error
	return !errors.Is(err, gorm.ErrRecordNotFound)
}

func (user *User) FillUserByLinuxDOId() error {
	if user.LinuxDOId == "" {
		return errors.New("linux do id is empty")
	}
	err := DB.Where("linux_do_id = ?", user.LinuxDOId).First(user).Error
	return err
}

func RootUserExists() bool {
	var user User
	err := DB.Where("role = ?", common.RoleRootUser).First(&user).Error
	if err != nil {
		return false
	}
	return true
}

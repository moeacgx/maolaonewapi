package model

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

type Token struct {
	Id                 int              `json:"id"`
	UserId             int              `json:"user_id" gorm:"index"`
	Key                string           `json:"key" gorm:"type:varchar(128);uniqueIndex"`
	Status             int              `json:"status" gorm:"default:1"`
	Name               string           `json:"name" gorm:"index" `
	CreatedTime        int64            `json:"created_time" gorm:"bigint"`
	AccessedTime       int64            `json:"accessed_time" gorm:"bigint"`
	ExpiredTime        int64            `json:"expired_time" gorm:"bigint;default:-1"` // -1 means never expired
	RemainQuota        int              `json:"remain_quota" gorm:"default:0"`
	UnlimitedQuota     bool             `json:"unlimited_quota"`
	ModelLimitsEnabled bool             `json:"model_limits_enabled"`
	ModelLimits        string           `json:"model_limits" gorm:"type:text"`
	AllowIps           *string          `json:"allow_ips" gorm:"default:''"`
	UsedQuota          int              `json:"used_quota" gorm:"default:0"` // used quota
	Group              string           `json:"group" gorm:"default:''"`
	GroupMode          string           `json:"group_mode" gorm:"type:varchar(16);default:'inherit'"`
	GroupIds           []int            `json:"group_ids,omitempty" gorm:"-"`
	GroupDetails       []GroupReference `json:"group_details,omitempty" gorm:"-"`
	GroupRatioLimits   string           `json:"group_ratio_limits" gorm:"type:text;default:''"`
	CrossGroupRetry    bool             `json:"cross_group_retry"` // 跨分组重试，仅auto分组有效
	DeletedAt          gorm.DeletedAt   `gorm:"index"`
}

// TokenQuotaReservation 把令牌扣减与随后可能发生的资金预留绑定在一起。
// 批量模式下会持有该令牌的刷盘锁，资金失败时可在落库前原地抵消扣减。
type TokenQuotaReservation struct {
	tokenId   int
	tokenKey  string
	quota     int
	batch     bool
	applyLock *sync.Mutex
	closed    bool
	mu        sync.Mutex
}

type TokenQuotaInsufficientError struct {
	RemainQuota int
	NeedQuota   int
}

func (e *TokenQuotaInsufficientError) Error() string {
	return fmt.Sprintf(
		"token quota is not enough, token remain quota: %d, need quota: %d",
		e.RemainQuota,
		e.NeedQuota,
	)
}

func (token *Token) Clean() {
	token.Key = ""
}

func MaskTokenKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 4 {
		return strings.Repeat("*", len(key))
	}
	if len(key) <= 8 {
		return key[:2] + "****" + key[len(key)-2:]
	}
	return key[:4] + "**********" + key[len(key)-4:]
}

func (token *Token) GetFullKey() string {
	return token.Key
}

func (token *Token) GetMaskedKey() string {
	return MaskTokenKey(token.Key)
}

func (token *Token) GetIpLimits() []string {
	// delete empty spaces
	//split with \n
	ipLimits := make([]string, 0)
	if token.AllowIps == nil {
		return ipLimits
	}
	cleanIps := strings.ReplaceAll(*token.AllowIps, " ", "")
	if cleanIps == "" {
		return ipLimits
	}
	ips := strings.Split(cleanIps, "\n")
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		ip = strings.ReplaceAll(ip, ",", "")
		if ip != "" {
			ipLimits = append(ipLimits, ip)
		}
	}
	return ipLimits
}

func GetAllUserTokens(userId int, startIdx int, num int) ([]*Token, error) {
	var tokens []*Token
	var err error
	err = DB.Where("user_id = ?", userId).Order("id desc").Limit(num).Offset(startIdx).Find(&tokens).Error
	if err == nil {
		err = HydrateTokenGroupBindings(DB, tokens)
	}
	return tokens, err
}

// sanitizeLikePattern 校验并清洗用户输入的 LIKE 搜索模式。
// 规则：
//  1. 转义 ! 和 _（使用 ! 作为 ESCAPE 字符，兼容 MySQL/PostgreSQL/SQLite）
//  2. 连续的 % 合并为单个 %
//  3. 最多允许 2 个 %
//  4. 含 % 时（模糊搜索），去掉 % 后关键词长度必须 >= 2
//  5. 不含 % 时按精确匹配
func sanitizeLikePattern(input string) (string, error) {
	// 1. 先转义 ESCAPE 字符 ! 自身，再转义 _
	//    使用 ! 而非 \ 作为 ESCAPE 字符，避免 MySQL 中反斜杠的字符串转义问题
	input = strings.ReplaceAll(input, "!", "!!")
	input = strings.ReplaceAll(input, `_`, `!_`)

	// 2. 连续的 % 直接拒绝
	if strings.Contains(input, "%%") {
		return "", errors.New("搜索模式中不允许包含连续的 % 通配符")
	}

	// 3. 统计 % 数量，不得超过 2
	count := strings.Count(input, "%")
	if count > 2 {
		return "", errors.New("搜索模式中最多允许包含 2 个 % 通配符")
	}

	// 4. 含 % 时，去掉 % 后关键词长度必须 >= 2
	if count > 0 {
		stripped := strings.ReplaceAll(input, "%", "")
		if len(stripped) < 2 {
			return "", errors.New("使用模糊搜索时，关键词长度至少为 2 个字符")
		}
		return input, nil
	}

	// 5. 无 % 时，精确全匹配
	return input, nil
}

const searchHardLimit = 100

func SearchUserTokens(userId int, keyword string, token string, offset int, limit int) (tokens []*Token, total int64, err error) {
	// model 层强制截断
	if limit <= 0 || limit > searchHardLimit {
		limit = searchHardLimit
	}
	if offset < 0 {
		offset = 0
	}

	if token != "" {
		token = strings.TrimPrefix(token, "sk-")
	}

	// 超量用户（令牌数超过上限）只允许精确搜索，禁止模糊搜索
	maxTokens := operation_setting.GetMaxUserTokens()
	hasFuzzy := strings.Contains(keyword, "%") || strings.Contains(token, "%")
	if hasFuzzy {
		count, err := CountUserTokens(userId)
		if err != nil {
			common.SysLog("failed to count user tokens: " + err.Error())
			return nil, 0, errors.New("获取令牌数量失败")
		}
		if int(count) > maxTokens {
			return nil, 0, errors.New("令牌数量超过上限，仅允许精确搜索，请勿使用 % 通配符")
		}
	}

	baseQuery := DB.Model(&Token{}).Where("user_id = ?", userId)

	// 非空才加 LIKE 条件，空则跳过（不过滤该字段）
	if keyword != "" {
		keywordPattern, err := sanitizeLikePattern(keyword)
		if err != nil {
			return nil, 0, err
		}
		baseQuery = baseQuery.Where("name LIKE ? ESCAPE '!'", keywordPattern)
	}
	if token != "" {
		tokenPattern, err := sanitizeLikePattern(token)
		if err != nil {
			return nil, 0, err
		}
		baseQuery = baseQuery.Where(commonKeyCol+" LIKE ? ESCAPE '!'", tokenPattern)
	}

	// 先查匹配总数（用于分页，受 maxTokens 上限保护，避免全表 COUNT）
	err = baseQuery.Limit(maxTokens).Count(&total).Error
	if err != nil {
		common.SysError("failed to count search tokens: " + err.Error())
		return nil, 0, errors.New("搜索令牌失败")
	}

	// 再分页查数据
	err = baseQuery.Order("id desc").Offset(offset).Limit(limit).Find(&tokens).Error
	if err != nil {
		common.SysError("failed to search tokens: " + err.Error())
		return nil, 0, errors.New("搜索令牌失败")
	}
	if err = HydrateTokenGroupBindings(DB, tokens); err != nil {
		return nil, 0, err
	}
	return tokens, total, nil
}

func ValidateUserToken(key string) (token *Token, err error) {
	if key == "" {
		return nil, ErrTokenNotProvided
	}
	token, err = GetTokenByKey(key, false)
	if err == nil {
		if token.Status == common.TokenStatusExhausted ||
			token.Status == common.TokenStatusExpired ||
			token.Status != common.TokenStatusEnabled {
			return token, ErrTokenInvalid
		}
		if token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp() {
			if !common.RedisEnabled {
				token.Status = common.TokenStatusExpired
				err := token.SelectUpdate()
				if err != nil {
					common.SysLog("failed to update token status" + err.Error())
				}
			}
			return token, ErrTokenInvalid
		}
		if !token.UnlimitedQuota && token.RemainQuota <= 0 {
			if !common.RedisEnabled {
				token.Status = common.TokenStatusExhausted
				err := token.SelectUpdate()
				if err != nil {
					common.SysLog("failed to update token status" + err.Error())
				}
			}
			return token, ErrTokenInvalid
		}
		return token, nil
	}
	common.SysLog("ValidateUserToken: failed to get token: " + err.Error())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTokenInvalid
	}
	return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
}

func GetTokenByIds(id int, userId int) (*Token, error) {
	if id == 0 || userId == 0 {
		return nil, errors.New("id 或 userId 为空！")
	}
	token := Token{Id: id, UserId: userId}
	var err error = nil
	err = DB.First(&token, "id = ? and user_id = ?", id, userId).Error
	if err == nil {
		err = HydrateTokenGroupBindings(DB, []*Token{&token})
	}
	return &token, err
}

func GetTokenById(id int) (*Token, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	token := Token{Id: id}
	var err error = nil
	err = DB.First(&token, "id = ?", id).Error
	if err == nil {
		err = HydrateTokenGroupBindings(DB, []*Token{&token})
	}
	if shouldUpdateRedis(true, err) {
		tokenID, tokenKey := token.Id, token.Key
		gopool.Go(func() {
			if err := cacheRefreshToken(tokenID, tokenKey, true); err != nil {
				common.SysLog("failed to update user status cache: " + err.Error())
			}
		})
	}
	return &token, err
}

func GetTokenByKey(key string, fromDB bool) (token *Token, err error) {
	if key == "" {
		return nil, gorm.ErrRecordNotFound
	}
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) && token != nil {
			tokenID, tokenKey := token.Id, token.Key
			gopool.Go(func() {
				if err := cacheRefreshToken(tokenID, tokenKey, true); err != nil {
					common.SysLog("failed to update user status cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		// Try Redis first
		token, err := cacheGetTokenByKey(key)
		if err == nil {
			if len(token.GroupIds) == 0 {
				if err = HydrateTokenGroupBindings(DB, []*Token{token}); err != nil {
					return nil, err
				}
			}
			return token, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	token = &Token{}
	err = DB.Where(&Token{Key: key}, "Key").First(token).Error
	if err == nil {
		err = HydrateTokenGroupBindings(DB, []*Token{token})
	}
	return token, err
}

func (token *Token) Insert() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := PrepareTokenGroupBindings(tx, token); err != nil {
			return err
		}
		if err := lockTokenGroupBindingGroups(tx, token); err != nil {
			return err
		}
		if err := ValidateTokenExclusiveGroupBinding(tx, token); err != nil {
			return err
		}
		if err := tx.Create(token).Error; err != nil {
			return err
		}
		return writeTokenGroupBindings(tx, token)
	})
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (token *Token) Update() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			tokenID, tokenKey := token.Id, token.Key
			gopool.Go(func() {
				err := cacheRefreshToken(tokenID, tokenKey, false)
				if err != nil {
					common.SysLog("failed to update token cache: " + err.Error())
				}
			})
		}
	}()
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := PrepareTokenGroupBindingsForUpdate(tx, token); err != nil {
			return err
		}
		if err := lockTokenGroupBindingGroups(tx, token); err != nil {
			return err
		}
		if err := ValidateTokenExclusiveGroupBinding(tx, token); err != nil {
			return err
		}
		var locked Token
		if err := lockForUpdate(tx).Select("id").First(&locked, "id = ?", token.Id).Error; err != nil {
			return err
		}
		if err := tx.Model(token).Select("name", "status", "expired_time", "remain_quota", "unlimited_quota",
			"model_limits_enabled", "model_limits", "allow_ips", "group", "group_mode", "group_ratio_limits", "cross_group_retry").Updates(token).Error; err != nil {
			return err
		}
		return writeTokenGroupBindings(tx, token)
	})
	return err
}

func (token *Token) SelectUpdate() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			tokenID, tokenKey := token.Id, token.Key
			gopool.Go(func() {
				err := cacheRefreshToken(tokenID, tokenKey, true)
				if err != nil {
					common.SysLog("failed to update token cache: " + err.Error())
				}
			})
		}
	}()
	// This can update zero values
	return DB.Model(token).Select("accessed_time", "status").Updates(token).Error
}

func (token *Token) Delete() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				err := cacheDeleteToken(token.Key)
				if err != nil {
					common.SysLog("failed to delete token cache: " + err.Error())
				}
			})
		}
	}()
	err = DB.Transaction(func(tx *gorm.DB) error {
		var locked Token
		if err := lockForUpdate(tx).Select("id").First(&locked, "id = ?", token.Id).Error; err != nil {
			return err
		}
		if err := deleteTokenGroupBindings(tx, []int{token.Id}); err != nil {
			return err
		}
		return tx.Delete(token).Error
	})
	return err
}

func (token *Token) IsModelLimitsEnabled() bool {
	return token.ModelLimitsEnabled
}

func (token *Token) GetModelLimits() []string {
	if token.ModelLimits == "" {
		return []string{}
	}
	return strings.Split(token.ModelLimits, ",")
}

// GetGroups 解析逗号分隔的分组字段，返回有序分组列表。
// 单分组返回 1 个元素的切片，空串返回 nil。
func (token *Token) GetGroups() []string {
	if token.Group == "" {
		return nil
	}
	parts := strings.Split(token.Group, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// IsMultiGroup 判断令牌是否配置了多个分组（逗号分隔）。
func (token *Token) IsMultiGroup() bool {
	return strings.Contains(token.Group, ",")
}

// IsAutoGroup 判断令牌是否为自动分组。
func (token *Token) IsAutoGroup() bool {
	return token.Group == "auto"
}

func (token *Token) GetGroupRatioLimitsMap() map[string]float64 {
	limits := make(map[string]float64)
	if token == nil || strings.TrimSpace(token.GroupRatioLimits) == "" {
		return limits
	}
	if err := common.UnmarshalJsonStr(token.GroupRatioLimits, &limits); err != nil {
		common.SysLog("failed to unmarshal token group ratio limits: " + err.Error())
		return map[string]float64{}
	}
	for group, ratio := range limits {
		trimmedGroup := strings.TrimSpace(group)
		if trimmedGroup == "" || ratio <= 0 {
			delete(limits, group)
			continue
		}
		if trimmedGroup != group {
			delete(limits, group)
			limits[trimmedGroup] = ratio
		}
	}
	return limits
}

func (token *Token) GetModelLimitsMap() map[string]bool {
	limits := token.GetModelLimits()
	limitsMap := make(map[string]bool)
	for _, limit := range limits {
		limitsMap[limit] = true
	}
	return limitsMap
}

func DisableModelLimits(tokenId int) error {
	token, err := GetTokenById(tokenId)
	if err != nil {
		return err
	}
	token.ModelLimitsEnabled = false
	token.ModelLimits = ""
	return token.Update()
}

func DeleteTokenById(id int, userId int) (err error) {
	// Why we need userId here? In case user want to delete other's token.
	if id == 0 || userId == 0 {
		return errors.New("id 或 userId 为空！")
	}
	token := Token{Id: id, UserId: userId}
	err = DB.Where(token).First(&token).Error
	if err != nil {
		return err
	}
	return token.Delete()
}

func updateTokenQuotaCache(key string, delta int) {
	if !common.RedisEnabled {
		return
	}
	if err := cacheIncrTokenQuota(key, int64(delta)); err != nil {
		common.SysLog("failed to update token quota cache: " + err.Error())
	}
}

func updateTokenQuotaCacheAsync(key string, delta int) {
	if !common.RedisEnabled {
		return
	}
	gopool.Go(func() {
		updateTokenQuotaCache(key, delta)
	})
}

// BeginTokenQuotaReservation 预留一次可提交或可取消的令牌扣减。
// 批量模式会阻止该令牌在资金结果明确前刷盘；非批量模式立即落库。
func BeginTokenQuotaReservation(tokenId int, key string, quota int, unlimited bool) (*TokenQuotaReservation, error) {
	if tokenId <= 0 {
		return nil, errors.New("token id is invalid")
	}
	if quota < 0 {
		return nil, errors.New("quota 不能为负数！")
	}

	reservation := &TokenQuotaReservation{
		tokenId:  tokenId,
		tokenKey: key,
		quota:    quota,
		batch:    common.BatchUpdateEnabled,
	}
	if reservation.batch {
		reservation.applyLock = tokenQuotaBatchApplyLockFor(tokenId)
		reservation.applyLock.Lock()
	}

	token, err := GetTokenByKey(key, false)
	if err != nil {
		reservation.releaseBatchLock()
		return nil, err
	}
	if token.Id != tokenId {
		reservation.releaseBatchLock()
		return nil, fmt.Errorf("token id mismatch: expected=%d actual=%d", tokenId, token.Id)
	}
	remainQuota := token.RemainQuota
	if reservation.batch && !common.RedisEnabled {
		remainQuota += pendingTokenQuotaDeltaLocked(tokenId)
	}
	if !unlimited && !token.UnlimitedQuota && remainQuota < quota {
		reservation.releaseBatchLock()
		return nil, &TokenQuotaInsufficientError{RemainQuota: remainQuota, NeedQuota: quota}
	}
	if quota == 0 {
		reservation.closed = true
		reservation.releaseBatchLock()
		return reservation, nil
	}

	if reservation.batch {
		addNewRecord(BatchUpdateTypeTokenQuota, tokenId, -quota)
		return reservation, nil
	}
	if err := decreaseTokenQuota(tokenId, quota); err != nil {
		return nil, err
	}
	updateTokenQuotaCache(key, -quota)
	return reservation, nil
}

func (r *TokenQuotaReservation) releaseBatchLock() {
	if r.applyLock == nil {
		return
	}
	r.applyLock.Unlock()
	r.applyLock = nil
}

// Commit 确认资金预留成功，使批量令牌扣减可刷盘。
func (r *TokenQuotaReservation) Commit() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	if r.batch {
		// 在释放刷盘锁前同步缓存，下一次同令牌预留才能看到最新额度。
		updateTokenQuotaCache(r.tokenKey, -r.quota)
		r.releaseBatchLock()
	}
	r.closed = true
}

// Compensate 取消令牌预留。批量模式在同一队列中抵消；非批量模式使用持久幂等账本。
func (r *TokenQuotaReservation) Compensate(operationKey string) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	if r.batch {
		addNewRecord(BatchUpdateTypeTokenQuota, r.tokenId, r.quota)
		r.releaseBatchLock()
		r.closed = true
		return nil
	}
	if err := ApplyTokenQuotaCompensation(operationKey, r.tokenId, r.tokenKey, r.quota); err != nil {
		return err
	}
	r.closed = true
	return nil
}

func IncreaseTokenQuota(tokenId int, key string, quota int) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if common.BatchUpdateEnabled {
		applyLock := tokenQuotaBatchApplyLockFor(tokenId)
		applyLock.Lock()
		defer applyLock.Unlock()
		addNewRecord(BatchUpdateTypeTokenQuota, tokenId, quota)
		updateTokenQuotaCache(key, quota)
		return nil
	}
	if err := increaseTokenQuota(tokenId, quota); err != nil {
		return err
	}
	updateTokenQuotaCacheAsync(key, quota)
	return nil
}

func increaseTokenQuota(id int, quota int) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota + ?", quota),
			"used_quota":    gorm.Expr("used_quota - ?", quota),
			"accessed_time": common.GetTimestamp(),
		},
	).Error
	return err
}

func DecreaseTokenQuota(id int, key string, quota int) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if common.BatchUpdateEnabled {
		applyLock := tokenQuotaBatchApplyLockFor(id)
		applyLock.Lock()
		defer applyLock.Unlock()
		addNewRecord(BatchUpdateTypeTokenQuota, id, -quota)
		updateTokenQuotaCache(key, -quota)
		return nil
	}
	if err := decreaseTokenQuota(id, quota); err != nil {
		return err
	}
	updateTokenQuotaCacheAsync(key, -quota)
	return nil
}

func decreaseTokenQuota(id int, quota int) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota - ?", quota),
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"accessed_time": common.GetTimestamp(),
		},
	).Error
	return err
}

// CountUserTokens returns total number of tokens for the given user, used for pagination
func CountUserTokens(userId int) (int64, error) {
	var total int64
	err := DB.Model(&Token{}).Where("user_id = ?", userId).Count(&total).Error
	return total, err
}

// BatchDeleteTokens 删除指定用户的一组令牌，返回成功删除数量
func BatchDeleteTokens(ids []int, userId int) (int, error) {
	if len(ids) == 0 {
		return 0, errors.New("ids 不能为空！")
	}

	tx := DB.Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}

	var tokens []Token
	if err := lockForUpdate(tx).
		Where("user_id = ? AND id IN (?)", userId, ids).
		Order("id ASC").
		Find(&tokens).Error; err != nil {
		tx.Rollback()
		return 0, err
	}
	tokenIDs := make([]int, 0, len(tokens))
	for _, token := range tokens {
		tokenIDs = append(tokenIDs, token.Id)
	}
	if err := deleteTokenGroupBindings(tx, tokenIDs); err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Where("user_id = ? AND id IN (?)", userId, ids).Delete(&Token{}).Error; err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Commit().Error; err != nil {
		return 0, err
	}

	if common.RedisEnabled {
		gopool.Go(func() {
			for _, t := range tokens {
				_ = cacheDeleteToken(t.Key)
			}
		})
	}

	return len(tokens), nil
}

func GetTokenKeysByIds(ids []int, userId int) ([]Token, error) {
	var tokens []Token
	err := DB.Select("id", commonKeyCol).
		Where("user_id = ? AND id IN (?)", userId, ids).
		Find(&tokens).Error
	return tokens, err
}

// InvalidateUserTokensCache 清理指定用户所有令牌在 Redis 中的缓存，
// 配合 InvalidateUserCache 使用，可在用户被禁用/删除时立即阻断其令牌的请求。
// 下一次请求将从数据库重新加载令牌及用户状态，从而立即识别出被禁用的用户。
func InvalidateUserTokensCache(userId int) error {
	if !common.RedisEnabled {
		return nil
	}
	if userId <= 0 {
		return errors.New("userId 无效")
	}
	var tokens []Token
	if err := DB.Unscoped().
		Select("id", commonKeyCol).
		Where("user_id = ?", userId).
		Find(&tokens).Error; err != nil {
		return err
	}
	return invalidateTokensCache(tokens)
}

func invalidateTokensCache(tokens []Token) error {
	if !common.RedisEnabled {
		return nil
	}
	var firstErr error
	for _, t := range tokens {
		if t.Key == "" {
			continue
		}
		if err := cacheDeleteToken(t.Key); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

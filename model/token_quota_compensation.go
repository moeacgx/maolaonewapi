package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrTokenQuotaCompensationConflict = errors.New("token quota compensation operation conflict")

// TokenQuotaCompensation 记录已经原子应用的令牌补偿，保证同一操作键只能增额一次。
type TokenQuotaCompensation struct {
	Id               int    `json:"id"`
	OperationKey     string `json:"operation_key" gorm:"type:varchar(64);uniqueIndex"`
	TokenId          int    `json:"token_id" gorm:"index;not null"`
	Quota            int    `json:"quota" gorm:"not null"`
	Status           string `json:"status" gorm:"type:varchar(16);index;not null;default:pending"`
	CacheInvalidated bool   `json:"cache_invalidated" gorm:"index;not null;default:false"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt        int64  `json:"updated_at" gorm:"bigint;index"`
}

func (r *TokenQuotaCompensation) BeforeCreate(tx *gorm.DB) error {
	r.CreatedAt = common.GetTimestamp()
	r.UpdatedAt = r.CreatedAt
	if r.Status == "" {
		r.Status = "pending"
	}
	return nil
}

func (r *TokenQuotaCompensation) BeforeUpdate(tx *gorm.DB) error {
	r.UpdatedAt = common.GetTimestamp()
	return nil
}

func normalizeTokenQuotaCompensationKey(operationKey string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(operationKey)))
}

func tokenQuotaCompensationMatches(record *TokenQuotaCompensation, tokenId int, quota int) bool {
	return record != nil && record.TokenId == tokenId && record.Quota == quota
}

func findTokenQuotaCompensation(operationKey string, tokenId int, quota int) (*TokenQuotaCompensation, error) {
	var record TokenQuotaCompensation
	result := DB.Where("operation_key = ?", operationKey).Limit(1).Find(&record)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	if !tokenQuotaCompensationMatches(&record, tokenId, quota) {
		return nil, fmt.Errorf("%w: operation_key=%s", ErrTokenQuotaCompensationConflict, operationKey)
	}
	return &record, nil
}

func ensureTokenQuotaCompensation(operationKey string, tokenId int, quota int) error {
	record := &TokenQuotaCompensation{
		OperationKey: operationKey,
		TokenId:      tokenId,
		Quota:        quota,
		Status:       "pending",
	}
	result := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "operation_key"}},
		DoNothing: true,
	}).Create(record)
	if result.Error != nil {
		existing, confirmErr := findTokenQuotaCompensation(operationKey, tokenId, quota)
		if confirmErr != nil {
			return errors.Join(result.Error, confirmErr)
		}
		if existing == nil {
			return result.Error
		}
		return nil
	}
	if result.RowsAffected > 0 {
		return nil
	}
	existing, err := findTokenQuotaCompensation(operationKey, tokenId, quota)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("token quota compensation %s disappeared after conflict", operationKey)
	}
	return nil
}

func applyTokenQuotaCompensation(operationKey string, tokenId int, quota int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		claim := tx.Model(&TokenQuotaCompensation{}).
			Where("operation_key = ? AND status = ?", operationKey, "pending").
			Update("status", "applying")
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected == 0 {
			var existing TokenQuotaCompensation
			if err := tx.Where("operation_key = ?", operationKey).First(&existing).Error; err != nil {
				return err
			}
			if !tokenQuotaCompensationMatches(&existing, tokenId, quota) {
				return fmt.Errorf("%w: operation_key=%s", ErrTokenQuotaCompensationConflict, operationKey)
			}
			if existing.Status == "applied" {
				return nil
			}
			return fmt.Errorf("token quota compensation %s has unexpected status %s", operationKey, existing.Status)
		}

		update := tx.Model(&Token{}).Where("id = ?", tokenId).Updates(map[string]interface{}{
			"remain_quota":  gorm.Expr("remain_quota + ?", quota),
			"used_quota":    gorm.Expr("used_quota - ?", quota),
			"accessed_time": common.GetTimestamp(),
		})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return fmt.Errorf("token %d not found while applying quota compensation", tokenId)
		}
		return tx.Model(&TokenQuotaCompensation{}).
			Where("operation_key = ? AND status = ?", operationKey, "applying").
			Updates(map[string]interface{}{"status": "applied", "cache_invalidated": false}).Error
	})
}

func invalidateTokenQuotaCompensationCache(operationKey string, tokenKey string) error {
	if common.RedisEnabled {
		if tokenKey == "" {
			var record TokenQuotaCompensation
			if err := DB.Select("token_id").Where("operation_key = ?", operationKey).First(&record).Error; err != nil {
				return err
			}
			var token Token
			if err := DB.Select("key").Where("id = ?", record.TokenId).First(&token).Error; err != nil {
				return err
			}
			tokenKey = token.Key
		}
		if err := cacheDeleteToken(tokenKey); err != nil {
			return fmt.Errorf("invalidate token cache after quota compensation: %w", err)
		}
	}
	return DB.Model(&TokenQuotaCompensation{}).
		Where("operation_key = ? AND status = ?", operationKey, "applied").
		Updates(map[string]interface{}{"cache_invalidated": true, "updated_at": common.GetTimestamp()}).Error
}

// ApplyTokenQuotaCompensation 幂等退还一次已经落库、但没有资金来源支撑的令牌额度。
// 批量预留在刷盘前由 TokenQuotaReservation 原地抵消，不会进入这里；历史待办记录始终直接落库。
func ApplyTokenQuotaCompensation(operationKey string, tokenId int, tokenKey string, quota int) error {
	if operationKey == "" {
		return errors.New("token quota compensation operation key is empty")
	}
	if tokenId <= 0 {
		return errors.New("token quota compensation token id is invalid")
	}
	if quota <= 0 {
		return errors.New("token quota compensation quota must be positive")
	}
	operationKey = normalizeTokenQuotaCompensationKey(operationKey)
	if err := ensureTokenQuotaCompensation(operationKey, tokenId, quota); err != nil {
		return err
	}
	err := applyTokenQuotaCompensation(operationKey, tokenId, quota)
	if err != nil {
		// COMMIT 返回错误时结果可能未知。必须在新事务确认同一操作键，
		// 查到完全一致的记录才可视为成功，不能直接重复 quota += N。
		record, confirmErr := findTokenQuotaCompensation(operationKey, tokenId, quota)
		if confirmErr != nil {
			return errors.Join(err, fmt.Errorf("confirm token quota compensation: %w", confirmErr))
		}
		if record == nil || record.Status != "applied" {
			return err
		}
	}
	return invalidateTokenQuotaCompensationCache(operationKey, tokenKey)
}

// ProcessPendingTokenQuotaCompensations 处理未完成或尚未确认缓存失效的补偿记录。
func ProcessPendingTokenQuotaCompensations(limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	var records []TokenQuotaCompensation
	if err := DB.Where("status = ? OR cache_invalidated = ?", "pending", false).
		Order("id asc").Limit(limit).Find(&records).Error; err != nil {
		return 0, err
	}
	processed := 0
	var joined error
	for _, record := range records {
		if record.Status != "applied" {
			if err := applyTokenQuotaCompensation(record.OperationKey, record.TokenId, record.Quota); err != nil {
				joined = errors.Join(joined, err)
				continue
			}
		}
		if err := invalidateTokenQuotaCompensationCache(record.OperationKey, ""); err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		processed++
	}
	return processed, joined
}

func CleanupTokenQuotaCompensations(olderThanSeconds int64) (int64, error) {
	if olderThanSeconds <= 0 {
		olderThanSeconds = 7 * 24 * 3600
	}
	cutoff := common.GetTimestamp() - olderThanSeconds
	result := DB.Where(
		"status = ? AND cache_invalidated = ? AND updated_at < ?",
		"applied",
		true,
		cutoff,
	).Delete(&TokenQuotaCompensation{})
	return result.RowsAffected, result.Error
}

// InitTokenQuotaCompensationWorker 周期恢复因临时数据库或缓存故障中断的令牌补偿。
func InitTokenQuotaCompensationWorker() {
	go func() {
		cycle := 0
		for {
			if _, err := ProcessPendingTokenQuotaCompensations(100); err != nil {
				common.SysError("failed to process token quota compensations: " + err.Error())
			}
			cycle++
			if cycle%120 == 0 {
				if _, err := CleanupTokenQuotaCompensations(7 * 24 * 3600); err != nil {
					common.SysError("failed to clean token quota compensations: " + err.Error())
				}
			}
			time.Sleep(30 * time.Second)
		}
	}()
}

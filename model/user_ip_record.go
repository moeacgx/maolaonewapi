package model

import (
	"net"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
)

type UserIPRecord struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	UserId    int    `json:"user_id" gorm:"index:idx_user_ip,priority:1;index"`
	Ip        string `json:"ip" gorm:"type:varchar(45);index:idx_user_ip,priority:2;index"`
	Action    string `json:"action" gorm:"type:varchar(32);index"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

const (
	UserIPActionLogin    = "login"
	UserIPActionRegister = "register"
)

func (UserIPRecord) TableName() string {
	return "user_ip_records"
}

func RecordUserIP(userId int, ip string, action string) {
	if userId <= 0 || ip == "" || ip == "127.0.0.1" || ip == "::1" {
		return
	}
	gopool.Go(func() {
		recordUserIPSync(userId, ip, action)
	})
}

func recordUserIPSync(userId int, ip string, action string) {
	now := common.GetTimestamp()
	oneHourAgo := now - 3600

	var count int64
	DB.Model(&UserIPRecord{}).
		Where("user_id = ? AND ip = ? AND action = ? AND created_at > ?", userId, ip, action, oneHourAgo).
		Count(&count)
	if count > 0 {
		return
	}

	record := &UserIPRecord{
		UserId: userId,
		Ip:     ip,
		Action: action,
	}
	DB.Create(record)
}

func GetDistinctIPsByUserId(userId int) ([]string, error) {
	var ips []string
	err := DB.Model(&UserIPRecord{}).
		Where("user_id = ? AND ip != ''", userId).
		Distinct("ip").
		Pluck("ip", &ips).Error
	return ips, err
}

func normalizeAffiliateFraudIP(raw string) (string, bool) {
	parsed := net.ParseIP(raw)
	if parsed == nil || parsed.IsUnspecified() || parsed.IsLoopback() ||
		parsed.IsLinkLocalUnicast() || parsed.IsLinkLocalMulticast() ||
		parsed.IsInterfaceLocalMulticast() || parsed.IsMulticast() ||
		parsed.IsPrivate() || !parsed.IsGlobalUnicast() {
		return "", false
	}
	if ipv4 := parsed.To4(); ipv4 != nil {
		return ipv4.String(), true
	}
	return parsed.String(), true
}

func filterAffiliateFraudIPs(ips []string) []string {
	seen := make(map[string]bool, len(ips))
	filtered := make([]string, 0, len(ips))
	for _, rawIP := range ips {
		ip, ok := normalizeAffiliateFraudIP(rawIP)
		if !ok || seen[ip] {
			continue
		}
		seen[ip] = true
		filtered = append(filtered, ip)
	}
	return filtered
}

func GetIPOverlap(userIdA, userIdB int, sinceTimestamp int64) ([]string, error) {
	var ipsA []string
	inviterQuery := DB.Model(&UserIPRecord{}).
		Where("user_id = ? AND ip != '' AND action IN ?", userIdA, []string{UserIPActionLogin, UserIPActionRegister}).
		Distinct("ip")
	if sinceTimestamp > 0 {
		inviterQuery = inviterQuery.Where("created_at >= ?", sinceTimestamp)
	}
	if err := inviterQuery.Pluck("ip", &ipsA).Error; err != nil {
		return nil, err
	}
	inviterSet := make(map[string]struct{}, len(ipsA))
	for _, raw := range ipsA {
		if ip, ok := normalizeAffiliateFraudIP(raw); ok {
			inviterSet[ip] = struct{}{}
		}
	}
	if len(inviterSet) == 0 {
		return nil, nil
	}

	var ipsB []string
	inviteeQuery := DB.Model(&UserIPRecord{}).
		Where("user_id = ? AND ip != '' AND action IN ?", userIdB, []string{UserIPActionLogin, UserIPActionRegister}).
		Distinct("ip")
	if sinceTimestamp > 0 {
		inviteeQuery = inviteeQuery.Where("created_at >= ?", sinceTimestamp)
	}
	if err := inviteeQuery.Pluck("ip", &ipsB).Error; err != nil {
		return nil, err
	}
	sharedSet := make(map[string]struct{})
	for _, raw := range ipsB {
		ip, ok := normalizeAffiliateFraudIP(raw)
		if !ok {
			continue
		}
		if _, shared := inviterSet[ip]; shared {
			sharedSet[ip] = struct{}{}
		}
	}
	shared := make([]string, 0, len(sharedSet))
	for ip := range sharedSet {
		shared = append(shared, ip)
	}
	sort.Strings(shared)
	return shared, nil
}

func GetIPOverlapBatch(inviterId int, inviteeIds []int, sinceTimestamp int64) (map[int][]string, error) {
	if len(inviteeIds) == 0 {
		return nil, nil
	}

	var inviterIPs []string
	inviterQuery := DB.Model(&UserIPRecord{}).
		Where("user_id = ? AND ip != '' AND action IN ?", inviterId, []string{UserIPActionLogin, UserIPActionRegister}).
		Distinct("ip")
	if sinceTimestamp > 0 {
		inviterQuery = inviterQuery.Where("created_at >= ?", sinceTimestamp)
	}
	if err := inviterQuery.Pluck("ip", &inviterIPs).Error; err != nil {
		return nil, err
	}
	inviterSet := make(map[string]struct{}, len(inviterIPs))
	for _, raw := range inviterIPs {
		if ip, ok := normalizeAffiliateFraudIP(raw); ok {
			inviterSet[ip] = struct{}{}
		}
	}
	if len(inviterSet) == 0 {
		return nil, nil
	}

	type ipUserRow struct {
		UserId int
		Ip     string
	}
	var rows []ipUserRow
	inviteeQuery := DB.Model(&UserIPRecord{}).
		Select("DISTINCT user_id, ip").
		Where("user_id IN ? AND ip != '' AND action IN ?", inviteeIds, []string{UserIPActionLogin, UserIPActionRegister})
	if sinceTimestamp > 0 {
		inviteeQuery = inviteeQuery.Where("created_at >= ?", sinceTimestamp)
	}
	if err := inviteeQuery.Find(&rows).Error; err != nil {
		return nil, err
	}

	resultSets := make(map[int]map[string]struct{})
	for _, row := range rows {
		ip, ok := normalizeAffiliateFraudIP(row.Ip)
		if !ok {
			continue
		}
		if _, shared := inviterSet[ip]; !shared {
			continue
		}
		if resultSets[row.UserId] == nil {
			resultSets[row.UserId] = make(map[string]struct{})
		}
		resultSets[row.UserId][ip] = struct{}{}
	}
	result := make(map[int][]string, len(resultSets))
	for userId, ips := range resultSets {
		for ip := range ips {
			result[userId] = append(result[userId], ip)
		}
		sort.Strings(result[userId])
	}
	return result, nil
}

func CleanOldIPRecords(beforeTimestamp int64) (int64, error) {
	result := DB.Where("created_at < ?", beforeTimestamp).Delete(&UserIPRecord{})
	return result.RowsAffected, result.Error
}

func GetUserIPRecordCount(userId int) (int64, error) {
	var count int64
	err := DB.Model(&UserIPRecord{}).Where("user_id = ?", userId).Count(&count).Error
	return count, err
}

func GetRecentIPsByUserId(userId int, limit int) ([]UserIPRecord, error) {
	var records []UserIPRecord
	err := DB.Where("user_id = ?", userId).
		Order("created_at DESC").
		Limit(limit).
		Find(&records).Error
	return records, err
}

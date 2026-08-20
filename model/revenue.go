package model

import (
	"sort"
)

// RevenueDataPoint 表示一个时间桶内的收入数据。
type RevenueDataPoint struct {
	Timestamp       int64   `json:"timestamp"`
	OnlineMoney     float64 `json:"online_money"`
	RedemptionQuota int64   `json:"redemption_quota"`
	OnlineCount     int     `json:"online_count"`
	RedemptionCount int     `json:"redemption_count"`
}

// RevenueSummary 表示查询时间范围内的收入汇总。
type RevenueSummary struct {
	TotalOnlineMoney     float64 `json:"total_online_money"`
	TotalRedemptionQuota int64   `json:"total_redemption_quota"`
	TotalOnlineCount     int     `json:"total_online_count"`
	TotalRedemptionCount int     `json:"total_redemption_count"`
}

// RevenueStatsResponse 表示收入统计接口的完整响应。
type RevenueStatsResponse struct {
	Summary    RevenueSummary     `json:"summary"`
	DataPoints []RevenueDataPoint `json:"data_points"`
}

// revenueRow 用于接收原始 SQL 聚合结果。
type revenueRow struct {
	Bucket int64   `gorm:"column:bucket"`
	Money  float64 `gorm:"column:money"`
	Quota  int64   `gorm:"column:quota"`
	Cnt    int     `gorm:"column:cnt"`
}

// GetRevenueStats 聚合指定时间范围内的在线充值与兑换码收入。
// tzOffset 是客户端相对 UTC 的秒偏移量，用于把每日时间桶对齐到客户端本地零点。
func GetRevenueStats(startTime, endTime int64, granularity string, tzOffset int64) (*RevenueStatsResponse, error) {
	bucketSeconds := int64(86400) // 默认按天聚合。
	if granularity == "hour" {
		bucketSeconds = 3600
	}

	dataMap := make(map[int64]*RevenueDataPoint)

	// 查询在线充值收入。
	if err := queryTopUpRevenue(dataMap, bucketSeconds, startTime, endTime, tzOffset); err != nil {
		return nil, err
	}

	// 订阅订单完成后已经写入 top_ups，不再单独查询，避免重复统计。

	// 查询兑换码收入。
	if err := queryRedemptionRevenue(dataMap, bucketSeconds, startTime, endTime, tzOffset); err != nil {
		return nil, err
	}

	// 汇总并按时间排序。
	summary := RevenueSummary{}
	dataPoints := make([]RevenueDataPoint, 0, len(dataMap))
	for _, dp := range dataMap {
		summary.TotalOnlineMoney += dp.OnlineMoney
		summary.TotalRedemptionQuota += dp.RedemptionQuota
		summary.TotalOnlineCount += dp.OnlineCount
		summary.TotalRedemptionCount += dp.RedemptionCount
		dataPoints = append(dataPoints, *dp)
	}

	sort.Slice(dataPoints, func(i, j int) bool {
		return dataPoints[i].Timestamp < dataPoints[j].Timestamp
	})

	return &RevenueStatsResponse{
		Summary:    summary,
		DataPoints: dataPoints,
	}, nil
}

func getOrCreateDataPoint(dataMap map[int64]*RevenueDataPoint, bucket int64) *RevenueDataPoint {
	dp, ok := dataMap[bucket]
	if !ok {
		dp = &RevenueDataPoint{Timestamp: bucket}
		dataMap[bucket] = dp
	}
	return dp
}

func queryTopUpRevenue(dataMap map[int64]*RevenueDataPoint, bucketSeconds, startTime, endTime, tzOffset int64) error {
	var rows []revenueRow
	// 先按客户端时区偏移，再取整时间桶，最后还原为 UTC 时间戳。
	err := DB.Raw(
		"SELECT ((complete_time + ?) / ? * ? - ?) AS bucket, SUM(actual_money) AS money, COUNT(*) AS cnt "+
			"FROM top_ups "+
			"WHERE status = 'success' AND payment_provider != 'balance' "+
			"AND complete_time >= ? AND complete_time <= ? "+
			"GROUP BY bucket ORDER BY bucket",
		tzOffset, bucketSeconds, bucketSeconds, tzOffset, startTime, endTime,
	).Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, r := range rows {
		dp := getOrCreateDataPoint(dataMap, r.Bucket)
		dp.OnlineMoney += r.Money
		dp.OnlineCount += r.Cnt
	}
	return nil
}

func queryRedemptionRevenue(dataMap map[int64]*RevenueDataPoint, bucketSeconds, startTime, endTime, tzOffset int64) error {
	var rows []revenueRow
	err := DB.Raw(
		"SELECT ((ru.created_time + ?) / ? * ? - ?) AS bucket, SUM(r.quota) AS quota, COUNT(*) AS cnt "+
			"FROM redemption_usages ru "+
			"LEFT JOIN redemptions r ON r.id = ru.redemption_id "+
			"WHERE ru.created_time >= ? AND ru.created_time <= ? "+
			"GROUP BY bucket ORDER BY bucket",
		tzOffset, bucketSeconds, bucketSeconds, tzOffset, startTime, endTime,
	).Scan(&rows).Error
	if err != nil {
		return err
	}
	for _, r := range rows {
		dp := getOrCreateDataPoint(dataMap, r.Bucket)
		dp.RedemptionQuota += r.Quota
		dp.RedemptionCount += r.Cnt
	}
	return nil
}

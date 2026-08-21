package model

import "testing"

func TestGetPerfMetricsMergesCurrentCodeAndHistoricalAliasIdentity(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	if err := db.AutoMigrate(&Group{}, &GroupAlias{}, &PerfMetric{}); err != nil {
		t.Fatalf("迁移性能指标测试表失败: %v", err)
	}
	defaultGroup := Group{Code: "default", Name: "默认", Ratio: 1, Status: GroupStatusActive}
	if err := db.Create(&defaultGroup).Error; err != nil {
		t.Fatal(err)
	}
	group := Group{Code: "2", Name: "特价", Ratio: 1, Status: GroupStatusActive}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&GroupAlias{Alias: "group_2", GroupId: group.Id}).Error; err != nil {
		t.Fatal(err)
	}
	metrics := []PerfMetric{
		{ModelName: "gpt-test", Group: "group_2", BucketTs: 100, RequestCount: 2},
		{ModelName: "gpt-test", Group: group.Code, BucketTs: 100, RequestCount: 3},
	}
	if err := db.Create(&metrics).Error; err != nil {
		t.Fatal(err)
	}

	queryRows, err := GetPerfMetrics("gpt-test", group.Code, 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(queryRows) != 2 {
		t.Fatalf("性能明细行数 = %d，期望历史 alias 与当前 code 均被查询", len(queryRows))
	}
	totalRequests := int64(0)
	for _, row := range queryRows {
		if row.Group != group.Code {
			t.Fatalf("性能明细分组 = %q，期望规范化为当前 code %q", row.Group, group.Code)
		}
		totalRequests += row.RequestCount
	}
	if totalRequests != 5 {
		t.Fatalf("性能明细请求数 = %d，期望包含历史 alias 与当前 code 的 5 次请求", totalRequests)
	}
}

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

	identifiers, err := ResolveGroupLogIdentifiers(group.Code)
	if err != nil {
		t.Fatal(err)
	}
	summaryRows, err := GetPerfMetricsSummaryBucketsAll(0, 200, identifiers)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaryRows) != 1 {
		t.Fatalf("性能摘要行数 = %d，期望历史 alias 与当前 code 聚合为 1 行", len(summaryRows))
	}
	if summaryRows[0].RequestCount != 5 {
		t.Fatalf("性能摘要请求数 = %d，期望包含历史 alias 与当前 code 的 5 次请求", summaryRows[0].RequestCount)
	}
}

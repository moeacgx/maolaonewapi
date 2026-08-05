package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestChannelRetryGroupIsolation(t *testing.T) {
	t.Run("memory cache", func(t *testing.T) {
		oldMemoryCacheEnabled := common.MemoryCacheEnabled
		common.MemoryCacheEnabled = true

		priority := int64(10)
		weight := uint(100)
		shared := &Channel{Id: 9101, Name: "shared", Priority: &priority, Weight: &weight}
		regularOnly := &Channel{Id: 9102, Name: "regular-only", Priority: &priority, Weight: &weight}

		channelSyncLock.Lock()
		oldGroups := group2model2channels
		oldChannels := channelsIDM
		group2model2channels = map[string]map[string][]int{
			"special": {"gpt-test": {shared.Id}},
			"regular": {"gpt-test": {shared.Id, regularOnly.Id}},
		}
		channelsIDM = map[int]*Channel{shared.Id: shared, regularOnly.Id: regularOnly}
		channelSyncLock.Unlock()

		t.Cleanup(func() {
			channelSyncLock.Lock()
			group2model2channels = oldGroups
			channelsIDM = oldChannels
			channelSyncLock.Unlock()
			common.MemoryCacheEnabled = oldMemoryCacheEnabled
			resetChannelConcurrencyForTest()
		})

		excluded := map[int]struct{}{shared.Id: {}}
		channel, err := GetRandomSatisfiedChannelWithExclusions("special", "gpt-test", 1, excluded)
		if err != nil {
			t.Fatalf("特价分组重试失败: %v", err)
		}
		if channel != nil {
			t.Fatalf("特价分组候选耗尽后错误选择渠道 #%d", channel.Id)
		}

		channel, err = GetRandomSatisfiedChannelWithExclusions("regular", "gpt-test", 1, excluded)
		if err != nil {
			t.Fatalf("正价分组选渠失败: %v", err)
		}
		if channel == nil || channel.Id != regularOnly.Id {
			t.Fatalf("正价分组应能选择仅属于正价的渠道，实际 %#v", channel)
		}
	})

	t.Run("database", func(t *testing.T) {
		db := openGroupIdentityTestDB(t)
		if err := db.AutoMigrate(&Channel{}, &Ability{}); err != nil {
			t.Fatalf("迁移选渠测试表失败: %v", err)
		}
		oldMemoryCacheEnabled := common.MemoryCacheEnabled
		common.MemoryCacheEnabled = false
		t.Cleanup(func() { common.MemoryCacheEnabled = oldMemoryCacheEnabled })

		priority := int64(10)
		weight := uint(100)
		shared := &Channel{Id: 9201, Name: "shared", Key: "shared-key", Group: "special,regular", Priority: &priority, Weight: &weight}
		regularOnly := &Channel{Id: 9202, Name: "regular-only", Key: "regular-key", Group: "regular", Priority: &priority, Weight: &weight}
		staleGroup := &Channel{Id: 9203, Name: "stale-group", Key: "stale-key", Group: "regular", Priority: &priority, Weight: &weight}
		if err := db.Create([]*Channel{shared, regularOnly, staleGroup}).Error; err != nil {
			t.Fatalf("创建测试渠道失败: %v", err)
		}
		abilities := []Ability{
			{Group: "special", Model: "gpt-test", ChannelId: shared.Id, Enabled: true, Priority: &priority, Weight: weight},
			{Group: "regular", Model: "gpt-test", ChannelId: shared.Id, Enabled: true, Priority: &priority, Weight: weight},
			{Group: "regular", Model: "gpt-test", ChannelId: regularOnly.Id, Enabled: true, Priority: &priority, Weight: weight},
			// 模拟渠道从 special 改到 regular 后遗留的旧能力记录。
			{Group: "special", Model: "gpt-test", ChannelId: staleGroup.Id, Enabled: true, Priority: &priority, Weight: weight},
		}
		if err := db.Create(&abilities).Error; err != nil {
			t.Fatalf("创建测试能力失败: %v", err)
		}
		excluded := map[int]struct{}{shared.Id: {}}
		channel, err := GetChannelWithExclusions("special", "gpt-test", 1, excluded)
		if err != nil {
			t.Fatalf("特价分组数据库重试失败: %v", err)
		}
		if channel != nil {
			t.Fatalf("特价分组候选耗尽后错误选择数据库渠道 #%d", channel.Id)
		}

		channel, err = GetChannelWithExclusions("regular", "gpt-test", 1, excluded)
		if err != nil {
			t.Fatalf("正价分组数据库选渠失败: %v", err)
		}
		if channel == nil || channel.Id != regularOnly.Id {
			t.Fatalf("正价分组应能选择仅属于正价的数据库渠道，实际 %#v", channel)
		}
		if isChannelEnabledForGroupModelDB("special", "gpt-test", staleGroup.Id) {
			t.Fatalf("渠道亲和校验不应接受已移出 special 分组的渠道 #%d", staleGroup.Id)
		}

		channel, err = GetChannelWithExclusions("special", "gpt-test", 0, nil)
		if err != nil {
			t.Fatalf("旧能力记录校验失败: %v", err)
		}
		if channel != nil && channel.Id == staleGroup.Id {
			t.Fatalf("已移出 special 分组的渠道 #%d 不应被重新选中", staleGroup.Id)
		}

		caseMismatch := &Channel{Id: 9204, Name: "case-mismatch", Key: "case-key", Group: "VIP", Priority: &priority, Weight: &weight}
		emptyGroup := &Channel{Id: 9205, Name: "empty-group", Key: "empty-key", Group: "", Priority: &priority, Weight: &weight}
		if err := db.Create([]*Channel{caseMismatch, emptyGroup}).Error; err != nil {
			t.Fatalf("创建边界测试渠道失败: %v", err)
		}
		if err := db.Create([]Ability{
			{Group: "vip", Model: "gpt-case", ChannelId: caseMismatch.Id, Enabled: true, Priority: &priority, Weight: weight},
			{Group: "special", Model: "gpt-empty", ChannelId: emptyGroup.Id, Enabled: true, Priority: &priority, Weight: weight},
		}).Error; err != nil {
			t.Fatalf("创建边界测试能力失败: %v", err)
		}
		channel, err = GetChannel("vip", "gpt-case", 0)
		if err != nil {
			t.Fatalf("大小写分组隔离校验失败: %v", err)
		}
		if channel != nil {
			t.Fatalf("分组编码 VIP 与 vip 必须严格隔离，实际选择渠道 #%d", channel.Id)
		}
		channel, err = GetChannel("special", "gpt-empty", 0)
		if err != nil {
			t.Fatalf("空分组渠道隔离校验失败: %v", err)
		}
		if channel != nil {
			t.Fatalf("没有当前分组的渠道不应通过历史能力记录被选中，实际渠道 #%d", channel.Id)
		}
	})

	t.Run("structured bindings override legacy mirror", func(t *testing.T) {
		db := openGroupIdentityTestDB(t)
		if err := db.AutoMigrate(&Group{}, &Channel{}, &Ability{}, &ChannelGroupBinding{}); err != nil {
			t.Fatalf("迁移结构化选渠测试表失败: %v", err)
		}
		oldMemoryCacheEnabled := common.MemoryCacheEnabled
		common.MemoryCacheEnabled = false
		channelSyncLock.Lock()
		oldGroups := group2model2channels
		oldChannels := channelsIDM
		channelSyncLock.Unlock()
		t.Cleanup(func() {
			channelSyncLock.Lock()
			group2model2channels = oldGroups
			channelsIDM = oldChannels
			channelSyncLock.Unlock()
			common.MemoryCacheEnabled = oldMemoryCacheEnabled
		})

		special := &Group{Id: 9301, Code: "special", Name: "特价", Status: GroupStatusActive}
		regular := &Group{Id: 9302, Code: "regular", Name: "正价", Status: GroupStatusActive}
		legacy := &Group{Id: 9304, Code: "legacy", Name: "旧分组", Status: GroupStatusActive}
		if err := db.Create([]*Group{special, regular, legacy}).Error; err != nil {
			t.Fatalf("创建结构化测试分组失败: %v", err)
		}
		priority := int64(10)
		weight := uint(100)
		channel := &Channel{
			Id: 9303, Name: "structured-current", Key: "structured-key",
			// 模拟兼容镜像仍残留 special，但管理端当前只绑定 regular。
			Group: "special", Models: "gpt-test", Priority: &priority, Weight: &weight,
		}
		if err := db.Create(channel).Error; err != nil {
			t.Fatalf("创建结构化测试渠道失败: %v", err)
		}
		if err := db.Create(&ChannelGroupBinding{ChannelId: channel.Id, GroupId: regular.Id, Position: 0}).Error; err != nil {
			t.Fatalf("创建结构化渠道绑定失败: %v", err)
		}
		if err := db.Create([]Ability{
			{Group: "special", Model: "gpt-test", ChannelId: channel.Id, Enabled: true, Priority: &priority, Weight: weight},
			{Group: "regular", Model: "gpt-test", ChannelId: channel.Id, Enabled: true, Priority: &priority, Weight: weight},
		}).Error; err != nil {
			t.Fatalf("创建结构化测试能力失败: %v", err)
		}

		selected, err := GetChannel("special", "gpt-test", 0)
		if err != nil {
			t.Fatalf("结构化分组隔离校验失败: %v", err)
		}
		if selected != nil {
			t.Fatalf("结构化绑定未包含 special 时不应信任旧镜像，实际选择渠道 #%d", selected.Id)
		}
		if isChannelEnabledForGroupModelDB("special", "gpt-test", channel.Id) {
			t.Fatalf("渠道亲和校验不应绕过结构化分组绑定")
		}
		selected, err = GetChannel("regular", "gpt-test", 0)
		if err != nil {
			t.Fatalf("结构化当前分组选渠失败: %v", err)
		}
		if selected == nil || selected.Id != channel.Id {
			t.Fatalf("结构化当前分组应能选择渠道 #%d，实际 %#v", channel.Id, selected)
		}
		if !selected.GroupsHydrated || len(selected.GroupDetails) != 1 || selected.GroupDetails[0].Code != "regular" {
			t.Fatalf("数据库选渠应直接返回已校验的结构化分组快照，实际 %#v", selected.GroupDetails)
		}

		hydrated := &Channel{Id: channel.Id, Group: "special"}
		if err := HydrateChannelGroupBindings(db, []*Channel{hydrated}); err != nil {
			t.Fatalf("加载结构化渠道分组失败: %v", err)
		}
		if hydrated.Group != "regular" {
			t.Fatalf("结构化绑定应覆盖旧分组镜像，实际 %q", hydrated.Group)
		}

		legacyChannel := &Channel{Id: 9305, Name: "legacy-channel", Key: "legacy-key", Group: "legacy", Models: "gpt-legacy", Priority: &priority, Weight: &weight}
		if err := db.Create(legacyChannel).Error; err != nil {
			t.Fatalf("创建未回填结构化绑定的旧渠道失败: %v", err)
		}
		if err := db.Create(&Ability{Group: "legacy", Model: "gpt-legacy", ChannelId: legacyChannel.Id, Enabled: true, Priority: &priority, Weight: weight}).Error; err != nil {
			t.Fatalf("创建旧渠道能力失败: %v", err)
		}
		selected, err = GetChannel("legacy", "gpt-legacy", 0)
		if err != nil {
			t.Fatalf("旧渠道字符串镜像回退失败: %v", err)
		}
		if selected == nil || selected.Id != legacyChannel.Id {
			t.Fatalf("尚未建立结构化绑定的旧渠道应继续可用，实际 %#v", selected)
		}

		markGroupBindingsBackfilled(db)
		selected, err = GetChannel("legacy", "gpt-legacy", 0)
		if err != nil {
			t.Fatalf("回填后缺失绑定校验失败: %v", err)
		}
		if selected != nil {
			t.Fatalf("回填完成后缺失结构化绑定必须拒绝旧镜像，实际渠道 #%d", selected.Id)
		}

		common.MemoryCacheEnabled = true
		InitChannelCache()
		selected, err = GetRandomSatisfiedChannel("special", "gpt-test", 0)
		if err != nil {
			t.Fatalf("内存缓存结构化分组隔离失败: %v", err)
		}
		if selected != nil {
			t.Fatalf("内存缓存不应使用结构化绑定已覆盖的 special 旧镜像，实际渠道 #%d", selected.Id)
		}
		selected, err = GetRandomSatisfiedChannel("regular", "gpt-test", 0)
		if err != nil {
			t.Fatalf("内存缓存当前分组选渠失败: %v", err)
		}
		if selected == nil || selected.Id != channel.Id {
			t.Fatalf("内存缓存应从结构化当前分组选择渠道 #%d，实际 %#v", channel.Id, selected)
		}
	})

	t.Run("memory cache creates indexes for authoritative groups", func(t *testing.T) {
		db := openGroupIdentityTestDB(t)
		if err := db.AutoMigrate(&Group{}, &Channel{}, &Ability{}, &ChannelGroupBinding{}); err != nil {
			t.Fatalf("迁移缓存边界测试表失败: %v", err)
		}
		oldMemoryCacheEnabled := common.MemoryCacheEnabled
		common.MemoryCacheEnabled = true
		channelSyncLock.Lock()
		oldGroups := group2model2channels
		oldChannels := channelsIDM
		channelSyncLock.Unlock()
		t.Cleanup(func() {
			channelSyncLock.Lock()
			group2model2channels = oldGroups
			channelsIDM = oldChannels
			channelSyncLock.Unlock()
			common.MemoryCacheEnabled = oldMemoryCacheEnabled
		})

		currentGroup := &Group{Id: 9401, Code: "cache-current", Name: "缓存当前分组", Status: GroupStatusActive}
		staleGroup := &Group{Id: 9402, Code: "cache-stale", Name: "缓存旧分组", Status: GroupStatusActive}
		if err := db.Create([]*Group{currentGroup, staleGroup}).Error; err != nil {
			t.Fatalf("创建缓存边界分组失败: %v", err)
		}
		priority := int64(10)
		weight := uint(100)
		channel := &Channel{Id: 9403, Name: "cache-current", Key: "cache-key", Group: "cache-stale", Models: "gpt-cache", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight}
		if err := db.Create(channel).Error; err != nil {
			t.Fatalf("创建缓存边界渠道失败: %v", err)
		}
		if err := db.Create(&ChannelGroupBinding{ChannelId: channel.Id, GroupId: currentGroup.Id, Position: 0}).Error; err != nil {
			t.Fatalf("创建缓存当前绑定失败: %v", err)
		}
		// 只保留旧分组能力，确保初始化不能依赖 abilities 预先创建当前分组 Map。
		if err := db.Create(&Ability{Group: staleGroup.Code, Model: "gpt-cache", ChannelId: channel.Id, Enabled: true, Priority: &priority, Weight: weight}).Error; err != nil {
			t.Fatalf("创建缓存旧能力失败: %v", err)
		}
		markGroupBindingsBackfilled(db)
		InitChannelCache()

		selected, err := GetRandomSatisfiedChannel(currentGroup.Code, "gpt-cache", 0)
		if err != nil {
			t.Fatalf("当前结构化分组缓存选渠失败: %v", err)
		}
		if selected == nil || selected.Id != channel.Id {
			t.Fatalf("缓存应为当前结构化分组动态建索引，实际 %#v", selected)
		}
		selected, err = GetRandomSatisfiedChannel(staleGroup.Code, "gpt-cache", 0)
		if err != nil {
			t.Fatalf("旧分组缓存隔离失败: %v", err)
		}
		if selected != nil {
			t.Fatalf("旧能力分组不应保留已迁移渠道，实际渠道 #%d", selected.Id)
		}
	})
}

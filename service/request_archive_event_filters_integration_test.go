package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func configureRequestArchiveEventFilters(
	t *testing.T,
	root string,
	scope string,
	channelIds []int,
	groupCodes []string,
	sources []string,
) *RequestArchiveConfig {
	t.Helper()
	config, err := GetRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	updated, err := SaveRequestArchiveConfig(context.Background(), RequestArchiveUpdateRequest{
		ExpectedConfigVersion: config.ConfigVersion,
		Enabled:               true,
		ArchiveScope:          scope,
		EventChannelIds:       channelIds,
		EventGroupCodes:       groupCodes,
		EventSources:          sources,
		ActiveTargetId:        "local-archive",
		RetentionDays:         RequestArchiveDefaultRetentionDays,
		WorkerCount:           RequestArchiveDefaultWorkerCount,
		QueueCapacity:         RequestArchiveDefaultQueueCapacity,
		MaxBodyBytes:          model.RequestArchiveDefaultMaxBodyBytes,
		QueueMaxBytes:         model.RequestArchiveDefaultQueueMaxBytes,
		Targets: []RequestArchiveUpdateTarget{{
			Id: "local-archive", Name: "本地归档", Type: model.RequestArchiveTargetLocal,
			Enabled: true, LocalPath: root,
		}},
	}, 1)
	require.NoError(t, err)
	return updated
}

func TestRequestArchiveEventFilterConfigRoundTrip(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	config, err := GetRequestArchiveConfig(context.Background())
	require.NoError(t, err)

	updated, err := SaveRequestArchiveConfig(context.Background(), RequestArchiveUpdateRequest{
		ExpectedConfigVersion: config.ConfigVersion,
		Enabled:               false,
		ArchiveScope:          model.RequestArchiveScopeAllRequests,
		EventChannelIds:       []int{31, 7, 31},
		EventGroupCodes:       []string{" vip ", "default", "vip"},
		EventSources: []string{
			PromptAuditSourceUpstreamPolicy,
			" PROMPT_GUARD ",
			PromptAuditSourceUpstreamPolicy,
		},
		RetentionDays: RequestArchiveDefaultRetentionDays,
		WorkerCount:   RequestArchiveDefaultWorkerCount,
		QueueCapacity: RequestArchiveDefaultQueueCapacity,
		MaxBodyBytes:  model.RequestArchiveDefaultMaxBodyBytes,
		QueueMaxBytes: model.RequestArchiveDefaultQueueMaxBytes,
	}, 1)
	require.NoError(t, err)
	require.Equal(t, []int{7, 31}, updated.EventChannelIds)
	require.Equal(t, []string{"default", "vip"}, updated.EventGroupCodes)
	require.Equal(t, []string{PromptAuditSourceGuard, PromptAuditSourceUpstreamPolicy}, updated.EventSources)

	var stored model.RequestArchiveConfig
	require.NoError(t, db.First(&stored, model.RequestArchiveConfigID).Error)
	require.Equal(t, "[7,31]", stored.EventChannelIds)
	require.Equal(t, `["default","vip"]`, stored.EventGroupCodes)
	require.Equal(t, `["prompt_guard","upstream_policy"]`, stored.EventSources)
}

func TestQueuePendingRequestArchiveRetainsCandidateUntilEventMatchesFilters(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	configureRequestArchiveEventFilters(
		t,
		requestArchiveTestLocalPath(t, "event-filter-archive"),
		model.RequestArchiveScopeAuditEvents,
		[]int{31},
		[]string{"vip"},
		[]string{PromptAuditSourceUpstreamPolicy},
	)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	SetPendingRequestArchive(c, RequestArchiveRequest{
		Body: []byte(`{"input":"unsafe"}`), Method: http.MethodPost,
		Path: "/v1/responses", RequestId: "req-event-filter",
	})

	QueuePendingRequestArchiveForAuditEvent(c, &model.PromptAuditEvent{
		Id: 101, ChannelId: 31, GroupCode: "vip", Source: PromptAuditSourceSensitiveWord,
	})
	var count int64
	require.NoError(t, db.Model(&model.RequestArchiveJob{}).Count(&count).Error)
	require.Zero(t, count)
	require.NotNil(t, pendingRequestArchive(c), "不匹配事件不能清除原始请求候选")

	QueuePendingRequestArchiveForAuditEvent(c, &model.PromptAuditEvent{
		Id: 102, ChannelId: 31, GroupCode: "vip", Source: PromptAuditSourceUpstreamPolicy,
	})
	var job model.RequestArchiveJob
	require.NoError(t, db.First(&job).Error)
	require.EqualValues(t, 102, job.AuditEventId)
	require.Nil(t, pendingRequestArchive(c))
}

func TestQueuePendingRequestArchiveKeepsCandidateWhenArchiveIsDisabledBeforeEvent(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	config := configureRequestArchiveEventFilters(
		t,
		requestArchiveTestLocalPath(t, "event-filter-disabled"),
		model.RequestArchiveScopeAuditEvents,
		nil,
		[]string{"vip"},
		[]string{PromptAuditSourceUpstreamPolicy},
	)
	require.Len(t, config.Targets, 1)
	target := config.Targets[0]

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	SetPendingRequestArchive(c, RequestArchiveRequest{
		Body: []byte(`{"input":"unsafe"}`), Method: http.MethodPost,
		Path: "/v1/responses", RequestId: "req-event-filter-disabled",
	})

	_, err := SaveRequestArchiveConfig(context.Background(), RequestArchiveUpdateRequest{
		ExpectedConfigVersion: config.ConfigVersion,
		Enabled:               false,
		ArchiveScope:          config.ArchiveScope,
		EventChannelIds:       config.EventChannelIds,
		EventGroupCodes:       config.EventGroupCodes,
		EventSources:          config.EventSources,
		ActiveTargetId:        config.ActiveTargetId,
		RetentionDays:         config.RetentionDays,
		WorkerCount:           config.WorkerCount,
		QueueCapacity:         config.QueueCapacity,
		MaxBodyBytes:          config.MaxBodyBytes,
		QueueMaxBytes:         config.QueueMaxBytes,
		Targets: []RequestArchiveUpdateTarget{{
			Id: target.Id, Name: target.Name, Type: target.Type,
			Enabled: target.Enabled, LocalPath: target.LocalPath,
		}},
	}, 1)
	require.NoError(t, err)

	QueuePendingRequestArchiveForAuditEvent(c, &model.PromptAuditEvent{
		Id: 103, GroupCode: "vip", Source: PromptAuditSourceUpstreamPolicy,
	})
	var count int64
	require.NoError(t, db.Model(&model.RequestArchiveJob{}).Count(&count).Error)
	require.Zero(t, count)
	require.NotNil(t, pendingRequestArchive(c), "禁用导致的无操作不能清除候选")
}

func TestRequestArchiveAllRequestsIgnoresStoredEventFilters(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	configureRequestArchiveEventFilters(
		t,
		requestArchiveTestLocalPath(t, "all-request-filter-archive"),
		model.RequestArchiveScopeAllRequests,
		[]int{999},
		[]string{"vip"},
		[]string{PromptAuditSourceUpstreamPolicy},
	)

	result, err := QueueRequestArchive(context.Background(), RequestArchiveRequest{
		Body: []byte(`{"input":"ordinary"}`), Method: http.MethodPost,
		Path: "/v1/responses", RequestId: "req-all-requests",
	})
	require.NoError(t, err)
	require.True(t, result.Enqueued)

	var count int64
	require.NoError(t, db.Model(&model.RequestArchiveJob{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
func TestRecoverExpiredPromptAuditJobQueuesFilteredRequestArchive(t *testing.T) {
	db := setupPromptAuditServiceTest(t, false, false, nil)
	InvalidateRequestArchiveConfig()
	require.NoError(t, model.MigrateRequestArchive())
	require.NoError(t, model.EnsureRequestArchiveDefaults())
	require.NoError(t, db.AutoMigrate(&model.Group{}, &model.GroupAlias{}))
	t.Cleanup(InvalidateRequestArchiveConfig)

	group := model.Group{Code: "lease-guard-group", Name: "租约终态分组", Ratio: 1, Status: model.GroupStatusActive}
	require.NoError(t, db.Create(&group).Error)
	configureRequestArchiveEventFilters(
		t,
		requestArchiveTestLocalPath(t, "lease-terminal-filtered-archive"),
		model.RequestArchiveScopeAuditEvents,
		nil,
		[]string{group.Code},
		[]string{PromptAuditSourceGuard},
	)

	rawBody := []byte("{\"model\":\"gpt-test\",\"messages\":[{\"role\":\"user\",\"content\":\"lease terminal prompt\"}]}")
	snapshot, err := BuildPromptAuditTextSnapshot(PromptAuditRequest{
		RequestId: "req-lease-terminal-filtered", Stage: "http",
		GroupId: group.Id, GroupName: group.Name,
		RequestArchive: &RequestArchiveRequest{
			Body: rawBody, ArchiveId: "archive-lease-terminal", DedupeKey: "archive-lease-terminal",
			Method: http.MethodPost, Path: "/v1/chat/completions", RequestId: "req-lease-terminal-filtered",
		},
	}, "lease terminal prompt")
	require.NoError(t, err)
	require.Empty(t, snapshot.GroupCode)

	cfg, err := GetPromptAuditConfig(context.Background())
	require.NoError(t, err)
	require.NoError(t, EnqueuePromptAuditSnapshot(snapshot, cfg))
	claimed, err := model.ClaimPromptAuditJob("lease-terminal-worker", time.Second)
	require.NoError(t, err)
	expiredAt := time.Now().Unix() - 1
	require.NoError(t, db.Model(&model.PromptAuditJob{}).Where("id = ?", claimed.Id).Updates(map[string]interface{}{
		"attempts": model.PromptAuditJobMaxAttempts, "lease_until": expiredAt,
	}).Error)

	recovered, err := recoverExpiredPromptAuditJobs(context.Background(), time.Now().Unix())
	require.NoError(t, err)
	require.EqualValues(t, 1, recovered)

	var event model.PromptAuditEvent
	require.NoError(t, db.First(&event, "job_id = ?", claimed.Id).Error)
	require.Equal(t, PromptAuditSourceGuard, event.Source)
	require.Equal(t, group.Code, event.GroupCode)

	var archives []model.RequestArchiveJob
	require.NoError(t, db.Where("audit_event_id = ?", event.Id).Find(&archives).Error)
	require.Len(t, archives, 1)
	require.Equal(t, rawBody, mustDecryptRequestArchivePayload(t, &archives[0]))

	recovered, err = recoverExpiredPromptAuditJobs(context.Background(), time.Now().Unix()+1)
	require.NoError(t, err)
	require.Zero(t, recovered)
	var archiveCount int64
	require.NoError(t, db.Model(&model.RequestArchiveJob{}).Where("audit_event_id = ?", event.Id).Count(&archiveCount).Error)
	require.EqualValues(t, 1, archiveCount)
}

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupRequestArchiveServiceTest(t *testing.T) *gorm.DB {
	t.Helper()
	InvalidateRequestArchiveConfig()
	oldDB := model.DB
	oldSecret := common.CryptoSecret
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "request-archive-service.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, model.MigrateRequestArchive())
	t.Setenv("CRYPTO_SECRET", "request-archive-stable-test-secret")
	common.CryptoSecret = "request-archive-stable-test-secret"
	require.NoError(t, model.EnsureRequestArchiveDefaults())
	t.Cleanup(func() {
		InvalidateRequestArchiveConfig()
		common.CryptoSecret = oldSecret
		model.DB = oldDB
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func requestArchiveTestLocalPath(t *testing.T, components ...string) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return filepath.Join(append([]string{base}, components...)...)
}

func configureRequestArchiveLocalTarget(t *testing.T, root string) {
	t.Helper()
	config, err := GetRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	_, err = SaveRequestArchiveConfig(context.Background(), RequestArchiveUpdateRequest{
		ExpectedConfigVersion: config.ConfigVersion,
		Enabled:               true,
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
}

func configureRequestArchiveS3TestTarget(t *testing.T, db *gorm.DB, targetID, endpoint string) model.RequestArchiveTarget {
	t.Helper()
	accessCiphertext, err := EncryptRequestArchiveSecret("access", targetID, requestArchiveAccessKeyPurpose)
	require.NoError(t, err)
	secretCiphertext, err := EncryptRequestArchiveSecret("secret", targetID, requestArchiveSecretKeyPurpose)
	require.NoError(t, err)
	target := model.RequestArchiveTarget{
		Id: targetID, Name: "测试 S3", Type: model.RequestArchiveTargetS3, Enabled: true,
		Endpoint: endpoint, Bucket: "archive-bucket", Region: "us-east-1", PathStyle: true,
		AccessKeyCiphertext: accessCiphertext, SecretKeyCiphertext: secretCiphertext,
	}
	require.NoError(t, NormalizeRequestArchiveTarget(&target))
	require.NoError(t, db.Create(&target).Error)
	return target
}

func TestQueueRequestArchiveEncryptsAndWritesLocalObject(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	root := requestArchiveTestLocalPath(t, "archive")
	configureRequestArchiveLocalTarget(t, root)
	secretBody := []byte(`{"model":"gpt-test","input":"only this encrypted archive may contain it"}`)
	result, err := QueueRequestArchive(context.Background(), RequestArchiveRequest{
		Body: secretBody, ContentType: "application/json", Method: "POST",
		Path: "/v1/responses?api_key=must-not-be-stored", RequestId: "req-archive-1",
		UserId: 12, Username: "alice", UserEmail: "alice@example.com", TokenId: 34, TokenName: "test-key",
	})
	require.NoError(t, err)
	require.True(t, result.Enqueued)

	var queued model.RequestArchiveJob
	require.NoError(t, db.First(&queued, result.JobId).Error)
	require.NotEmpty(t, queued.RequestCiphertext)
	require.NotContains(t, string(queued.RequestCiphertext), "only this encrypted")
	require.NotContains(t, queued.Path, "api_key")
	plainDigest := sha256.Sum256(secretBody)
	require.NotEqual(t, hex.EncodeToString(plainDigest[:]), queued.SHA256)
	plain, err := DecryptRequestArchivePayload(&queued)
	require.NoError(t, err)
	require.Equal(t, secretBody, plain)

	processed, err := ProcessNextRequestArchiveJob(context.Background(), "local-test-worker")
	require.NoError(t, err)
	require.True(t, processed)
	var completed model.RequestArchiveJob
	require.NoError(t, db.First(&completed, result.JobId).Error)
	require.Equal(t, model.RequestArchiveJobDone, completed.Status)
	require.Empty(t, completed.RequestCiphertext)
	require.NotEmpty(t, completed.ObjectKey)
	stored, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(completed.ObjectKey)))
	require.NoError(t, err)
	require.NotContains(t, string(stored), "only this encrypted")
	completed.RequestCiphertext = model.RequestArchiveLargeText(stored)
	plain, err = DecryptRequestArchivePayload(&completed)
	require.NoError(t, err)
	require.Equal(t, secretBody, plain)
}

func TestQueueRequestArchiveWorksWithoutCryptoSecretForLocalTarget(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	t.Setenv("CRYPTO_SECRET", "")
	common.CryptoSecret = ""
	root := requestArchiveTestLocalPath(t, "plain-archive")
	configureRequestArchiveLocalTarget(t, root)
	body := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"无需密钥也要保留"}]}`)
	result, err := QueueRequestArchive(context.Background(), RequestArchiveRequest{
		Body: body, ContentType: "application/json", Method: "POST", Path: "/v1/chat/completions",
	})
	require.NoError(t, err)
	require.True(t, result.Enqueued)

	var queued model.RequestArchiveJob
	require.NoError(t, db.First(&queued, result.JobId).Error)
	require.Equal(t, requestArchivePlaintextVersion, queued.RequestCipherFormat)
	require.True(t, strings.HasPrefix(string(queued.RequestCiphertext), requestArchivePlaintextPrefix))
	plain, err := DecryptRequestArchivePayload(&queued)
	require.NoError(t, err)
	require.Equal(t, body, plain)

	processed, err := ProcessNextRequestArchiveJob(context.Background(), "plain-worker")
	require.NoError(t, err)
	require.True(t, processed)
	var completed model.RequestArchiveJob
	require.NoError(t, db.First(&completed, result.JobId).Error)
	require.Equal(t, model.RequestArchiveJobDone, completed.Status)
	stored, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(completed.ObjectKey)))
	require.NoError(t, err)
	require.Equal(t, body, stored[len(requestArchivePlaintextPrefix):])
}

func TestRequestArchiveTargetSwitchKeepsOldQueuedTargetAndCleansExactObject(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	root := requestArchiveTestLocalPath(t, "archive")
	configureRequestArchiveLocalTarget(t, root)
	result, err := QueueRequestArchive(context.Background(), RequestArchiveRequest{Body: []byte("archive body"), Method: "POST"})
	require.NoError(t, err)

	config, err := GetRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	_, err = SaveRequestArchiveConfig(context.Background(), RequestArchiveUpdateRequest{
		ExpectedConfigVersion: config.ConfigVersion, Enabled: false, RetentionDays: config.RetentionDays,
		WorkerCount: config.WorkerCount, QueueCapacity: config.QueueCapacity,
		MaxBodyBytes: config.MaxBodyBytes, QueueMaxBytes: config.QueueMaxBytes,
		Targets: []RequestArchiveUpdateTarget{{
			Id: "local-archive", Name: "本地归档", Type: model.RequestArchiveTargetLocal,
			Enabled: false, LocalPath: root,
		}},
	}, 1)
	require.NoError(t, err)

	processed, err := ProcessNextRequestArchiveJob(context.Background(), "disabled-target-worker")
	require.NoError(t, err)
	require.True(t, processed)
	var completed model.RequestArchiveJob
	require.NoError(t, db.First(&completed, result.JobId).Error)
	require.Equal(t, model.RequestArchiveJobDone, completed.Status)
	objectPath := filepath.Join(root, filepath.FromSlash(completed.ObjectKey))
	require.FileExists(t, objectPath)

	// 已有保留期对象时，不能复用同一个目标 ID 改写物理位置；否则后续
	// 清理会指向新位置而遗留旧对象。
	config, err = GetRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	_, err = SaveRequestArchiveConfig(context.Background(), RequestArchiveUpdateRequest{
		ExpectedConfigVersion: config.ConfigVersion, Enabled: false, RetentionDays: config.RetentionDays,
		WorkerCount: config.WorkerCount, QueueCapacity: config.QueueCapacity,
		MaxBodyBytes: config.MaxBodyBytes, QueueMaxBytes: config.QueueMaxBytes,
		Targets: []RequestArchiveUpdateTarget{{
			Id: "local-archive", Name: "本地归档", Type: model.RequestArchiveTargetLocal,
			Enabled: false, LocalPath: requestArchiveTestLocalPath(t, "moved-archive"),
		}},
	}, 1)
	require.ErrorIs(t, err, model.ErrRequestArchiveTargetInUse)

	// 即使主目标已切走，尚在保留期的成功归档仍阻止删除它的目标配置。
	config, err = GetRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	_, err = SaveRequestArchiveConfig(context.Background(), RequestArchiveUpdateRequest{
		ExpectedConfigVersion: config.ConfigVersion, Enabled: false, RetentionDays: config.RetentionDays,
		WorkerCount: config.WorkerCount, QueueCapacity: config.QueueCapacity,
		MaxBodyBytes: config.MaxBodyBytes, QueueMaxBytes: config.QueueMaxBytes,
	}, 1)
	require.ErrorIs(t, err, model.ErrRequestArchiveTargetInUse)

	expiredAt := time.Now().Add(-time.Minute).Unix()
	require.NoError(t, db.Model(&model.RequestArchiveJob{}).Where("id = ?", completed.Id).Update("expires_at", expiredAt).Error)
	removed, err := CleanupExpiredRequestArchiveObjects(context.Background(), time.Now().Unix(), 100)
	require.NoError(t, err)
	require.EqualValues(t, 1, removed)
	require.NoFileExists(t, objectPath)
	var count int64
	require.NoError(t, db.Model(&model.RequestArchiveJob{}).Where("id = ?", completed.Id).Count(&count).Error)
	require.Zero(t, count)
}

func TestRequestArchivePayloadCannotMoveAcrossJobs(t *testing.T) {
	setupRequestArchiveServiceTest(t)
	payload := []byte("immutable request archive payload")
	digest := sha256.Sum256(payload)
	sha256Value := hex.EncodeToString(digest[:])
	ciphertext, err := EncryptRequestArchivePayload(payload, sha256Value, int64(len(payload)), "target-a", 7)
	require.NoError(t, err)
	job := &model.RequestArchiveJob{
		TargetId: "target-b", ConfigVersion: 7, SHA256: sha256Value, ByteSize: int64(len(payload)),
		RequestCiphertext: model.RequestArchiveLargeText(ciphertext),
	}
	_, err = DecryptRequestArchivePayload(job)
	require.Error(t, err)
}

func TestRequestArchiveLegacyPayloadStillDecrypts(t *testing.T) {
	setupRequestArchiveServiceTest(t)
	payload := []byte("legacy request archive payload")
	digest := sha256.Sum256(payload)
	job := &model.RequestArchiveJob{
		TargetId: "legacy-target", ConfigVersion: 3,
		SHA256: hex.EncodeToString(digest[:]), ByteSize: int64(len(payload)),
	}
	ciphertext, err := EncryptRequestArchivePayload(payload, job.SHA256, job.ByteSize, job.TargetId, job.ConfigVersion)
	require.NoError(t, err)
	job.RequestCiphertext = model.RequestArchiveLargeText(ciphertext)

	plaintext, err := DecryptRequestArchivePayload(job)
	require.NoError(t, err)
	require.Equal(t, payload, plaintext)
}

func TestRequestArchiveEnqueuePreservesAuthenticatedCreatedAt(t *testing.T) {
	setupRequestArchiveServiceTest(t)
	configureRequestArchiveLocalTarget(t, requestArchiveTestLocalPath(t, "archive"))
	config, _, err := model.LoadRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	body := []byte(`{"model":"gpt-test","input":"created-at binding"}`)
	createdAt := time.Now().Add(-2 * time.Minute).Unix()
	job := &model.RequestArchiveJob{
		ArchiveId: uuid.NewString(), TargetId: config.ActiveTargetId, ConfigVersion: config.ConfigVersion,
		ByteSize:    int64(len(body)),
		ContentType: "application/json", Method: http.MethodPost, Path: "/v1/responses",
		RequestId: "created-at-binding", CreatedAt: createdAt,
		ExpiresAt: createdAt + int64(config.RetentionDays)*24*60*60,
	}
	digestText, err := requestArchivePlaintextDigest(job, requestArchiveCipherVersion, body)
	require.NoError(t, err)
	job.SHA256 = digestText
	ciphertext, err := EncryptRequestArchiveJobPayload(body, job)
	require.NoError(t, err)
	job.RequestCiphertext = model.RequestArchiveLargeText(ciphertext)
	require.NoError(t, model.EnqueueRequestArchiveJob(context.Background(), job, config.QueueCapacity))

	stored, err := model.GetRequestArchiveJob(context.Background(), job.Id)
	require.NoError(t, err)
	require.Equal(t, createdAt, stored.CreatedAt)
	plaintext, err := DecryptRequestArchivePayload(stored)
	require.NoError(t, err)
	require.Equal(t, body, plaintext)
}

func TestRequestArchiveV3ChunksAuthenticateBodyAndMetadata(t *testing.T) {
	setupRequestArchiveServiceTest(t)
	body := bytes.Repeat([]byte("chunked-request-body-"), 120000)
	job := &model.RequestArchiveJob{
		ArchiveId: uuid.NewString(), TargetId: "chunk-target", ConfigVersion: 7,
		ByteSize:    int64(len(body)),
		ContentType: "application/json", Method: http.MethodPost, Path: "/v1/responses",
		RequestId: "chunk-request", UserId: 12, Username: "alice", UserEmail: "alice@example.com",
		TokenId: 34, TokenName: "token", GroupId: 56, GroupName: "default",
		CreatedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
	}
	digestText, err := requestArchivePlaintextDigest(job, requestArchiveCipherVersion, body)
	require.NoError(t, err)
	job.SHA256 = digestText
	plainDigest := sha256.Sum256(body)
	require.NotEqual(t, hex.EncodeToString(plainDigest[:]), job.SHA256, "ra3 数据库不得保存可离线猜测的明文 SHA-256")
	ciphertext, err := EncryptRequestArchiveJobPayload(body, job)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(ciphertext, requestArchiveCipherVersion+"."))
	job.RequestCiphertext = model.RequestArchiveLargeText(ciphertext)
	require.NoError(t, ValidateRequestArchivePayload(job))
	plaintext, err := DecryptRequestArchivePayload(job)
	require.NoError(t, err)
	require.Equal(t, body, plaintext)

	tampered := *job
	tampered.Path = "/v1/chat/completions"
	require.Error(t, ValidateRequestArchivePayload(&tampered))
	tampered = *job
	tampered.ArchiveId = uuid.NewString()
	require.Error(t, ValidateRequestArchivePayload(&tampered))
	tampered = *job
	parts := strings.Split(ciphertext, ".")
	require.GreaterOrEqual(t, len(parts), 5)
	chunkIndex := len(parts[4]) / 2
	replacement := byte('A')
	if parts[4][chunkIndex] == replacement {
		replacement = 'B'
	}
	parts[4] = parts[4][:chunkIndex] + string(replacement) + parts[4][chunkIndex+1:]
	tampered.RequestCiphertext = model.RequestArchiveLargeText(strings.Join(parts, "."))
	require.Error(t, ValidateRequestArchivePayload(&tampered))
}

func TestRequestArchiveV2ChunkEnvelopeStillDecrypts(t *testing.T) {
	setupRequestArchiveServiceTest(t)
	body := bytes.Repeat([]byte("legacy-v2-chunk-"), 80000)
	digest := sha256.Sum256(body)
	job := &model.RequestArchiveJob{
		ArchiveId: uuid.NewString(), TargetId: "legacy-v2", ConfigVersion: 4,
		SHA256: hex.EncodeToString(digest[:]), ByteSize: int64(len(body)),
		ContentType: "application/json", Method: http.MethodPost, Path: "/v1/responses",
		RequestId: "legacy-v2-request", CreatedAt: time.Now().Unix(),
		ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
	}
	ciphertext, err := encryptRequestArchiveV2JobPayload(body, job)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(ciphertext, requestArchiveV2CipherVersion+"."))
	parts := strings.SplitN(ciphertext, ".", 5)
	require.Len(t, parts, 5)
	noncePrefix, err := base64.RawURLEncoding.DecodeString(parts[3])
	require.NoError(t, err)
	require.Len(t, noncePrefix, 8, "已发布 ra2 使用 8 字节随机前缀")
	job.RequestCiphertext = model.RequestArchiveLargeText(ciphertext)

	plaintext, err := DecryptRequestArchivePayload(job)
	require.NoError(t, err)
	require.Equal(t, body, plaintext)
}

func TestRequestArchiveRejectsKeepCredentialsWhenS3DestinationChanges(t *testing.T) {
	setupRequestArchiveServiceTest(t)
	initial, err := GetRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	_, err = SaveRequestArchiveConfig(context.Background(), RequestArchiveUpdateRequest{
		ExpectedConfigVersion: initial.ConfigVersion, Enabled: true, ActiveTargetId: "r2-archive",
		RetentionDays: 30, WorkerCount: 4, QueueCapacity: 1024,
		MaxBodyBytes: model.RequestArchiveDefaultMaxBodyBytes, QueueMaxBytes: model.RequestArchiveDefaultQueueMaxBytes,
		Targets: []RequestArchiveUpdateTarget{{
			Id: "r2-archive", Name: "R2", Type: model.RequestArchiveTargetS3, Enabled: true,
			Endpoint: "https://account-id.r2.cloudflarestorage.com", Bucket: "request-archive", Region: "auto",
			AccessKeyAction: RequestArchiveSecretReplace, AccessKey: "r2-access-key",
			SecretKeyAction: RequestArchiveSecretReplace, SecretKey: "r2-secret-key",
		}},
	}, 1)
	require.NoError(t, err)

	current, err := GetRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	_, err = SaveRequestArchiveConfig(context.Background(), RequestArchiveUpdateRequest{
		ExpectedConfigVersion: current.ConfigVersion, Enabled: true, ActiveTargetId: "r2-archive",
		RetentionDays: 30, WorkerCount: 4, QueueCapacity: 1024,
		MaxBodyBytes: model.RequestArchiveDefaultMaxBodyBytes, QueueMaxBytes: model.RequestArchiveDefaultQueueMaxBytes,
		Targets: []RequestArchiveUpdateTarget{{
			Id: "r2-archive", Name: "R2", Type: model.RequestArchiveTargetS3, Enabled: true,
			Endpoint: "https://another-account.r2.cloudflarestorage.com", Bucket: "request-archive", Region: "auto",
			AccessKeyAction: RequestArchiveSecretKeep, SecretKeyAction: RequestArchiveSecretKeep,
		}},
	}, 1)
	require.Error(t, err)

	_, err = SaveRequestArchiveConfig(context.Background(), RequestArchiveUpdateRequest{
		ExpectedConfigVersion: current.ConfigVersion, Enabled: true, ActiveTargetId: "r2-archive",
		RetentionDays: 30, WorkerCount: 4, QueueCapacity: 1024,
		MaxBodyBytes: model.RequestArchiveDefaultMaxBodyBytes, QueueMaxBytes: model.RequestArchiveDefaultQueueMaxBytes,
		Targets: []RequestArchiveUpdateTarget{{
			Id: "r2-archive", Name: "R2", Type: model.RequestArchiveTargetS3, Enabled: true,
			Endpoint: "https://another-account.r2.cloudflarestorage.com", Bucket: "request-archive", Region: "auto",
			AccessKeyAction: RequestArchiveSecretReplace, AccessKey: "replacement-access-key",
			SecretKeyAction: RequestArchiveSecretReplace, SecretKey: "replacement-secret-key",
		}},
	}, 1)
	require.NoError(t, err)
}

func TestRequestArchiveStorageKeyValidationAndPublicVersionField(t *testing.T) {
	setupRequestArchiveServiceTest(t)
	_, err := safeRequestArchiveRelativeKey("../escape")
	require.Error(t, err)
	_, err = safeRequestArchiveLocalPath(t.TempDir(), "/absolute")
	require.Error(t, err)
	require.Error(t, NormalizeRequestArchiveTarget(&model.RequestArchiveTarget{
		Type: model.RequestArchiveTargetS3, Endpoint: "https://key:secret@example.invalid", Bucket: "archive-bucket",
	}))

	field, ok := reflect.TypeOf(RequestArchiveUpdateRequest{}).FieldByName("ExpectedConfigVersion")
	require.True(t, ok)
	require.Equal(t, "expected_version", strings.Split(field.Tag.Get("json"), ",")[0])
}

func TestRequestArchiveLocalStorageCreatesSafeNestedDirectory(t *testing.T) {
	root := requestArchiveTestLocalPath(t, "archive")
	destination := filepath.Join(root, "requests", "2026", "07", "28", "1-test.enc")
	require.NoError(t, atomicWriteRequestArchiveFile(root, destination, "encrypted-envelope"))
	content, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, "encrypted-envelope", string(content))
}

func TestRequestArchiveLocalStorageRejectsLinkedRoot(t *testing.T) {
	base := requestArchiveTestLocalPath(t)
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.Mkdir(outside, 0o700))
	linkedRoot := filepath.Join(base, "linked-root")
	if err := os.Symlink(outside, linkedRoot); err != nil {
		t.Skipf("当前环境不能创建目录符号链接: %v", err)
	}
	require.Error(t, atomicWriteRequestArchiveFile(
		linkedRoot,
		filepath.Join(linkedRoot, "requests", "1.enc"),
		"encrypted-envelope",
	))
}

func TestRequestArchiveLocalStorageRejectsVolumeRoot(t *testing.T) {
	volumeRoot := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	require.Error(t, NormalizeRequestArchiveTarget(&model.RequestArchiveTarget{
		Type: model.RequestArchiveTargetLocal, LocalPath: volumeRoot,
	}))
}

func TestRequestArchiveLocalStorageAllowsLiteralTildeWithoutCreatingOnSave(t *testing.T) {
	root := requestArchiveTestLocalPath(t, "literal~archive", "nested")
	target := &model.RequestArchiveTarget{Type: model.RequestArchiveTargetLocal, LocalPath: root}
	require.NoError(t, NormalizeRequestArchiveTarget(target))
	_, err := os.Stat(root)
	require.ErrorIs(t, err, os.ErrNotExist, "配置校验不得创建本地归档目录")

	destination := filepath.Join(root, "requests", "1.enc")
	require.NoError(t, atomicWriteRequestArchiveFile(root, destination, "encrypted-envelope"))
	content, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, "encrypted-envelope", string(content))
}

func TestRequestArchiveLocalStorageAllowsActualWindowsShortPathAlias(t *testing.T) {
	if runtime.GOOS != "windows" || !strings.Contains(os.TempDir(), "~") {
		t.Skip("当前临时目录不是 Windows 8.3 短路径别名")
	}
	root := filepath.Join(os.TempDir(), "request-archive-short-path")
	require.NoError(t, NormalizeRequestArchiveTarget(&model.RequestArchiveTarget{
		Type: model.RequestArchiveTargetLocal, LocalPath: root,
	}))
	_, err := os.Stat(root)
	require.ErrorIs(t, err, os.ErrNotExist, "配置校验不得创建本地归档目录")
}

func TestRequestArchiveTargetInputLimitsAndSecretActions(t *testing.T) {
	require.Error(t, NormalizeRequestArchiveTarget(&model.RequestArchiveTarget{
		Type:      model.RequestArchiveTargetLocal,
		LocalPath: requestArchiveTestLocalPath(t, strings.Repeat("x", requestArchiveMaxLocalPathBytes)),
	}))
	require.Error(t, NormalizeRequestArchiveTarget(&model.RequestArchiveTarget{
		Type:     model.RequestArchiveTargetS3,
		Endpoint: "https://" + strings.Repeat("a", requestArchiveMaxEndpointBytes),
		Bucket:   "archive",
	}))
	_, err := requestArchiveSecretForAction(RequestArchiveSecretKeep, "unexpected", "ciphertext", true, "target", requestArchiveAccessKeyPurpose)
	require.Error(t, err)
	_, err = requestArchiveSecretForAction(RequestArchiveSecretClear, "unexpected", "ciphertext", true, "target", requestArchiveAccessKeyPurpose)
	require.Error(t, err)
	_, err = requestArchiveSecretForAction(RequestArchiveSecretReplace, strings.Repeat("x", requestArchiveMaxCredentialBytes+1), "", false, "target", requestArchiveAccessKeyPurpose)
	require.Error(t, err)
}

func TestRequestArchiveWorkerMemoryBudgetUsesCandidateMetadata(t *testing.T) {
	chunkedWeight, err := requestArchiveWorkerMemoryWeight(model.RequestArchiveJobCandidate{
		ArchiveId: uuid.NewString(), ByteSize: model.RequestArchiveMaximumBodyBytes,
	})
	require.NoError(t, err)
	require.Positive(t, chunkedWeight)
	require.Less(t, chunkedWeight, requestArchiveEncryptionMemoryBudget)

	legacyWeight, err := requestArchiveWorkerMemoryWeight(model.RequestArchiveJobCandidate{
		ByteSize: model.RequestArchiveMaximumBodyBytes,
	})
	require.NoError(t, err)
	legacyEnvelope := int64(base64.RawURLEncoding.EncodedLen(
		int(model.RequestArchiveMaximumBodyBytes)+requestArchiveGCMTagSize,
	) + 64)
	require.GreaterOrEqual(t, legacyWeight, legacyEnvelope+model.RequestArchiveMaximumBodyBytes)
	require.LessOrEqual(t, legacyWeight, requestArchiveEncryptionMemoryBudget)
}

func TestRequestArchiveDeliveryQuietWindowCoversStorageTimeout(t *testing.T) {
	require.GreaterOrEqual(t, model.RequestArchiveMinimumDeliveryWindow, requestArchiveStorageMaxTimeout)
	require.GreaterOrEqual(t, requestArchiveCleanupReconcileQuietPeriod, requestArchiveStorageMaxTimeout)
}

func TestRequestArchiveStorageSecretIsBoundToTargetAndPurpose(t *testing.T) {
	setupRequestArchiveServiceTest(t)
	envelope, err := EncryptRequestArchiveSecret("storage-access-key", "target-a", requestArchiveAccessKeyPurpose)
	require.NoError(t, err)
	require.NotContains(t, envelope, "storage-access-key")

	plaintext, err := DecryptRequestArchiveSecret(envelope, "target-a", requestArchiveAccessKeyPurpose)
	require.NoError(t, err)
	require.Equal(t, "storage-access-key", plaintext)
	_, err = DecryptRequestArchiveSecret(envelope, "target-b", requestArchiveAccessKeyPurpose)
	require.Error(t, err)
	_, err = DecryptRequestArchiveSecret(envelope, "target-a", requestArchiveSecretKeyPurpose)
	require.Error(t, err)
	_, err = DecryptPromptAuditSecret(envelope)
	require.Error(t, err)
}

func TestRequestArchiveObjectKeyDoesNotExposePlaintextHash(t *testing.T) {
	setupRequestArchiveServiceTest(t)
	body := []byte("known request body")
	digest := sha256.Sum256(body)
	job := &model.RequestArchiveJob{
		Id: 91, TargetId: "target-a", ConfigVersion: 2,
		SHA256: hex.EncodeToString(digest[:]), ByteSize: int64(len(body)), CreatedAt: time.Now().Unix(),
	}
	ciphertext, err := EncryptRequestArchivePayload(body, job.SHA256, job.ByteSize, job.TargetId, job.ConfigVersion)
	require.NoError(t, err)
	job.RequestCiphertext = model.RequestArchiveLargeText(ciphertext)

	key, err := requestArchiveObjectKey(model.RequestArchiveTarget{}, job)
	require.NoError(t, err)
	require.NotContains(t, key, job.SHA256)
	require.Contains(t, key, requestArchiveCiphertextDigest(job))
}

func TestRequestArchiveHeadObjectOnlyTreats404AsMissing(t *testing.T) {
	notFound := &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusNotFound}},
		Err:      errors.New("not found"),
	}
	serverFailure := &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: http.StatusServiceUnavailable}},
		Err:      errors.New("unavailable"),
	}
	require.True(t, requestArchiveObjectNotFound(notFound))
	require.False(t, requestArchiveObjectNotFound(serverFailure))
	require.False(t, requestArchiveObjectNotFound(errors.New("network failure")))
}

func TestQueueRealtimeRequestArchiveFramePreservesRawBytes(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	configureRequestArchiveLocalTarget(t, requestArchiveTestLocalPath(t, "archive"))

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime?key=must-not-be-stored", nil)
	c.Set(common.RequestIdKey, "realtime-request-1")
	common.SetContextKey(c, constant.ContextKeyUserId, 12)
	common.SetContextKey(c, constant.ContextKeyTokenId, 34)
	payload := []byte{0x00, 0x01, 0x02, 0xff}
	QueueRealtimeRequestArchiveFrame(c, websocket.BinaryMessage, payload)

	var job model.RequestArchiveJob
	require.NoError(t, db.First(&job).Error)
	require.Equal(t, "WS_BINARY", job.Method)
	require.Equal(t, "/v1/realtime", job.Path)
	require.NotContains(t, job.Path, "key")
	plaintext, err := DecryptRequestArchivePayload(&job)
	require.NoError(t, err)
	require.Equal(t, payload, plaintext)
}

func TestRequestArchiveS3WriteIsIdempotentAfterLostResponse(t *testing.T) {
	setupRequestArchiveServiceTest(t)
	body := []byte("request body stored after a lost S3 response")
	digest := sha256.Sum256(body)
	job := &model.RequestArchiveJob{
		Id: 77, TargetId: "s3-idempotent", ConfigVersion: 3,
		SHA256: hex.EncodeToString(digest[:]), ByteSize: int64(len(body)), CreatedAt: time.Now().Unix(),
	}
	ciphertext, err := EncryptRequestArchivePayload(body, job.SHA256, job.ByteSize, job.TargetId, job.ConfigVersion)
	require.NoError(t, err)
	job.RequestCiphertext = model.RequestArchiveLargeText(ciphertext)

	var mu sync.Mutex
	var stored []byte
	var handlerErr error
	putCount := 0
	conditionalHeader := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch request.Method {
		case http.MethodHead:
			if stored == nil {
				http.NotFound(w, request)
				return
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(stored)))
			w.Header().Set("x-amz-meta-cipher-sha256", requestArchiveBytesDigest(stored))
			w.Header().Set("x-amz-version-id", "version-1")
			w.WriteHeader(http.StatusOK)
		case http.MethodPut:
			putCount++
			conditionalHeader = request.Header.Get("If-None-Match")
			if stored != nil {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			stored, err = io.ReadAll(request.Body)
			if err != nil {
				handlerErr = err
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			// 模拟对象已经提交，但响应在客户端收到前断开。
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				handlerErr = errors.New("测试服务器不支持 Hijacker")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			connection, _, hijackErr := hijacker.Hijack()
			if hijackErr != nil {
				handlerErr = hijackErr
				return
			}
			_ = connection.Close()
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	accessCiphertext, err := EncryptRequestArchiveSecret("access", job.TargetId, requestArchiveAccessKeyPurpose)
	require.NoError(t, err)
	secretCiphertext, err := EncryptRequestArchiveSecret("secret", job.TargetId, requestArchiveSecretKeyPurpose)
	require.NoError(t, err)
	target := model.RequestArchiveTarget{
		Id: job.TargetId, Type: model.RequestArchiveTargetS3, Enabled: true,
		Endpoint: server.URL, Bucket: "archive-bucket", Region: "us-east-1", PathStyle: true,
		AccessKeyCiphertext: accessCiphertext, SecretKeyCiphertext: secretCiphertext,
	}
	require.NoError(t, NormalizeRequestArchiveTarget(&target))

	_, _, err = writeRequestArchiveObject(context.Background(), target, job)
	require.Error(t, err)
	key, versionID, err := writeRequestArchiveObject(context.Background(), target, job)
	require.NoError(t, err)
	mu.Lock()
	observedHandlerErr := handlerErr
	observedConditionalHeader := conditionalHeader
	observedPutCount := putCount
	mu.Unlock()
	require.NotEmpty(t, key)
	require.Equal(t, "version-1", versionID)
	require.NoError(t, observedHandlerErr)
	require.Equal(t, "*", observedConditionalHeader)
	require.Equal(t, 1, observedPutCount)
}

func TestRequestArchiveS3ClearRequiresDisabledTargetAndBothCredentials(t *testing.T) {
	setupRequestArchiveServiceTest(t)
	initial, err := GetRequestArchiveConfig(context.Background())
	require.NoError(t, err)
	configured, err := SaveRequestArchiveConfig(context.Background(), RequestArchiveUpdateRequest{
		ExpectedConfigVersion: initial.ConfigVersion,
		Enabled:               false,
		RetentionDays:         30,
		WorkerCount:           1,
		QueueCapacity:         16,
		MaxBodyBytes:          model.RequestArchiveDefaultMaxBodyBytes,
		QueueMaxBytes:         model.RequestArchiveDefaultQueueMaxBytes,
		Targets: []RequestArchiveUpdateTarget{{
			Id: "clear-s3", Name: "待停用 S3", Type: model.RequestArchiveTargetS3, Enabled: true,
			Endpoint: "https://s3.example.com", Bucket: "archive-bucket", Region: "us-east-1",
			AccessKeyAction: RequestArchiveSecretReplace, AccessKey: "access",
			SecretKeyAction: RequestArchiveSecretReplace, SecretKey: "secret",
		}},
	}, 1)
	require.NoError(t, err)

	_, err = SaveRequestArchiveConfig(context.Background(), RequestArchiveUpdateRequest{
		ExpectedConfigVersion: configured.ConfigVersion,
		Enabled:               false,
		RetentionDays:         30,
		WorkerCount:           1,
		QueueCapacity:         16,
		MaxBodyBytes:          model.RequestArchiveDefaultMaxBodyBytes,
		QueueMaxBytes:         model.RequestArchiveDefaultQueueMaxBytes,
		Targets: []RequestArchiveUpdateTarget{{
			Id: "clear-s3", Name: "待停用 S3", Type: model.RequestArchiveTargetS3, Enabled: false,
			Endpoint: "https://s3.example.com", Bucket: "archive-bucket", Region: "us-east-1",
			AccessKeyAction: RequestArchiveSecretClear,
			SecretKeyAction: RequestArchiveSecretKeep,
		}},
	}, 1)
	require.Error(t, err)

	cleared, err := SaveRequestArchiveConfig(context.Background(), RequestArchiveUpdateRequest{
		ExpectedConfigVersion: configured.ConfigVersion,
		Enabled:               false,
		RetentionDays:         30,
		WorkerCount:           1,
		QueueCapacity:         16,
		MaxBodyBytes:          model.RequestArchiveDefaultMaxBodyBytes,
		QueueMaxBytes:         model.RequestArchiveDefaultQueueMaxBytes,
		Targets: []RequestArchiveUpdateTarget{{
			Id: "clear-s3", Name: "待停用 S3", Type: model.RequestArchiveTargetS3, Enabled: false,
			Endpoint: "https://s3.example.com", Bucket: "archive-bucket", Region: "us-east-1",
			AccessKeyAction: RequestArchiveSecretClear,
			SecretKeyAction: RequestArchiveSecretClear,
		}},
	}, 1)
	require.NoError(t, err)
	require.False(t, cleared.Targets[0].AccessKeyConfigured)
	require.False(t, cleared.Targets[0].SecretKeyConfigured)
}

func TestRequestArchiveRejectsMetadataEndpointsButAllowsPrivateStorage(t *testing.T) {
	for _, endpoint := range []string{
		"http://169.254.169.254",
		"http://100.100.100.200",
		"http://metadata.google.internal",
		"http://[fd00:ec2::254]",
	} {
		t.Run(endpoint, func(t *testing.T) {
			target := model.RequestArchiveTarget{Type: model.RequestArchiveTargetS3, Endpoint: endpoint, Bucket: "archive-bucket"}
			require.Error(t, NormalizeRequestArchiveTarget(&target))
		})
	}
	for _, endpoint := range []string{"http://127.0.0.1:9000", "http://192.168.1.20:9000"} {
		t.Run(endpoint, func(t *testing.T) {
			target := model.RequestArchiveTarget{Type: model.RequestArchiveTargetS3, Endpoint: endpoint, Bucket: "archive-bucket"}
			require.NoError(t, NormalizeRequestArchiveTarget(&target))
		})
	}
	require.Error(t, NormalizeRequestArchiveTarget(&model.RequestArchiveTarget{
		Type: model.RequestArchiveTargetS3, Endpoint: "http://8.8.8.8:9000", Bucket: "archive-bucket",
	}))
	require.NoError(t, NormalizeRequestArchiveTarget(&model.RequestArchiveTarget{
		Type: model.RequestArchiveTargetS3, Endpoint: "https://8.8.8.8:9000", Bucket: "archive-bucket",
	}))
}

func TestRequestArchiveHTTPResolvedAddressesMustAllBePrivate(t *testing.T) {
	privateAddresses := []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}, {IP: net.ParseIP("10.20.30.40")}}
	require.NoError(t, validateRequestArchiveResolvedAddresses(privateAddresses, true))

	mixedAddresses := append(append([]net.IPAddr{}, privateAddresses...), net.IPAddr{IP: net.ParseIP("8.8.8.8")})
	require.Error(t, validateRequestArchiveResolvedAddresses(mixedAddresses, true))
	require.NoError(t, validateRequestArchiveResolvedAddresses(mixedAddresses, false))
	require.Error(t, validateRequestArchiveResolvedAddresses([]net.IPAddr{{IP: net.ParseIP("169.254.169.254")}}, false))
}

func TestRequestArchiveR2RecoveryUsesHeadWithoutVersionAPIs(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	content := "r2-unversioned-envelope"
	digest := requestArchiveStringDigest(content)
	key := fmt.Sprintf("requests/2026/07/28/900-%s.enc", digest)
	var observationMu sync.Mutex
	unexpectedMethods := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodHead {
			observationMu.Lock()
			unexpectedMethods = append(unexpectedMethods, request.Method)
			observationMu.Unlock()
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(content)))
		w.Header().Set("x-amz-meta-cipher-sha256", digest)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	target := configureRequestArchiveS3TestTarget(t, db, "r2-head-recovery", server.URL)
	versionID, exists, err := inspectRequestArchiveS3Object(
		context.Background(), newRequestArchiveS3Client(target, "access", "secret"), target.Bucket,
		key, digest, int64(len(content)), true, true,
	)
	require.NoError(t, err)
	require.True(t, exists)
	require.Empty(t, versionID)
	observationMu.Lock()
	observedUnexpectedMethods := append([]string(nil), unexpectedMethods...)
	observationMu.Unlock()
	require.Empty(t, observedUnexpectedMethods)
	require.True(t, isRequestArchiveR2Endpoint("https://account-id.r2.cloudflarestorage.com"))
	require.True(t, isRequestArchiveR2Endpoint("https://account-id.eu.r2.cloudflarestorage.com"))
	require.False(t, isRequestArchiveR2Endpoint("https://r2.cloudflarestorage.com.example.test"))
	require.Empty(t, requestArchiveVersionIDForTarget(model.RequestArchiveTarget{
		Type: model.RequestArchiveTargetS3, Endpoint: "https://account-id.r2.cloudflarestorage.com",
	}, ""))
	require.Equal(t, requestArchiveS3NullVersionID, requestArchiveVersionIDForTarget(model.RequestArchiveTarget{
		Type: model.RequestArchiveTargetS3,
	}, ""))
}

func TestRequestArchiveCleanupRecoversVersionBeforeExactDelete(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	content := "encrypted-envelope"
	digest := requestArchiveStringDigest(content)
	key := fmt.Sprintf("requests/2026/07/28/901-%s.enc", digest)
	var observationMu sync.Mutex
	deletedVersion := ""
	versionPersistedBeforeDelete := false
	var persistenceObservationErr error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodHead:
			w.Header().Set("Content-Length", fmt.Sprint(len(content)))
			w.Header().Set("x-amz-meta-cipher-sha256", digest)
			w.Header().Set("x-amz-version-id", "version-crash-window")
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			observationMu.Lock()
			defer observationMu.Unlock()
			deletedVersion = request.URL.Query().Get("versionId")
			var persisted model.RequestArchiveJob
			persistenceObservationErr = db.First(&persisted, "object_key = ?", key).Error
			versionPersistedBeforeDelete = persistenceObservationErr == nil &&
				persisted.ObjectVersionMode == model.RequestArchiveObjectVersionExact &&
				persisted.ObjectVersionId == "version-crash-window"
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	target := configureRequestArchiveS3TestTarget(t, db, "s3-version-recovery", server.URL)
	now := time.Now().Unix()
	job := model.RequestArchiveJob{
		Status: model.RequestArchiveJobDone, TargetId: target.Id, ObjectKey: key,
		ObjectVersionId: "", ObjectVersionMode: model.RequestArchiveObjectVersionUnknown, SHA256: strings.Repeat("a", 64),
		CreatedAt: now - 10, FinishedAt: now - 5, ExpiresAt: now - 1,
	}
	require.NoError(t, db.Create(&job).Error)

	removed, err := CleanupExpiredRequestArchiveObjects(context.Background(), now, 10)
	require.NoError(t, err)
	require.Zero(t, removed)
	removed, err = CleanupExpiredRequestArchiveObjects(
		context.Background(), now+int64(requestArchiveCleanupReconcileQuietPeriod/time.Second), 10,
	)
	require.NoError(t, err)
	require.EqualValues(t, 1, removed)
	observationMu.Lock()
	observedDeletedVersion := deletedVersion
	observedPersistenceErr := persistenceObservationErr
	observedPersistedBeforeDelete := versionPersistedBeforeDelete
	observationMu.Unlock()
	require.Equal(t, "version-crash-window", observedDeletedVersion)
	require.NoError(t, observedPersistenceErr)
	require.True(t, observedPersistedBeforeDelete)
	var remaining int64
	require.NoError(t, db.Model(&model.RequestArchiveJob{}).Where("id = ?", job.Id).Count(&remaining).Error)
	require.Zero(t, remaining)
}

func TestRequestArchiveCleanupOmitsVersionForConfirmedUnversionedObject(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	digest := requestArchiveStringDigest("confirmed-unversioned-envelope")
	key := fmt.Sprintf("requests/2026/07/28/904-%s.enc", digest)
	var observationMu sync.Mutex
	deleteHadVersion := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		observationMu.Lock()
		deleteHadVersion = request.URL.Query().Has("versionId")
		observationMu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	target := configureRequestArchiveS3TestTarget(t, db, "s3-null-version", server.URL)
	now := time.Now().Unix()
	job := model.RequestArchiveJob{
		Status: model.RequestArchiveJobDone, TargetId: target.Id, ObjectKey: key,
		ObjectVersionMode: model.RequestArchiveObjectVersionUnversioned, SHA256: strings.Repeat("d", 64),
		CreatedAt: now - 10, FinishedAt: now - 5, ExpiresAt: now - 1,
	}
	require.NoError(t, db.Create(&job).Error)

	removed, err := CleanupExpiredRequestArchiveObjects(context.Background(), now, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, removed)
	observationMu.Lock()
	observedDeleteHadVersion := deleteHadVersion
	observationMu.Unlock()
	require.False(t, observedDeleteHadVersion)
}

func TestRequestArchiveCleanupTrustsPersistedLegacyVersionID(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	digest := requestArchiveStringDigest("legacy-version-envelope")
	key := fmt.Sprintf("requests/2026/07/28/905-%s.enc", digest)
	var observationMu sync.Mutex
	deletedVersion := ""
	unexpectedMethod := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		observationMu.Lock()
		defer observationMu.Unlock()
		if request.Method != http.MethodDelete {
			unexpectedMethod = request.Method
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		deletedVersion = request.URL.Query().Get("versionId")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	target := configureRequestArchiveS3TestTarget(t, db, "s3-legacy-version", server.URL)
	now := time.Now().Unix()
	job := model.RequestArchiveJob{
		Status: model.RequestArchiveJobDone, TargetId: target.Id, ObjectKey: key,
		ObjectVersionId: "legacy-version-id", ObjectVersionMode: model.RequestArchiveObjectVersionUnknown, SHA256: strings.Repeat("e", 64),
		CreatedAt: now - 10, FinishedAt: now - 5, ExpiresAt: now - 1,
	}
	require.NoError(t, db.Create(&job).Error)

	removed, err := CleanupExpiredRequestArchiveObjects(context.Background(), now, 10)
	require.NoError(t, err)
	require.Zero(t, removed)
	removed, err = CleanupExpiredRequestArchiveObjects(
		context.Background(), now+int64(requestArchiveCleanupReconcileQuietPeriod/time.Second), 10,
	)
	require.NoError(t, err)
	require.EqualValues(t, 1, removed)
	observationMu.Lock()
	observedDeletedVersion := deletedVersion
	observedUnexpectedMethod := unexpectedMethod
	observationMu.Unlock()
	require.Equal(t, "legacy-version-id", observedDeletedVersion)
	require.Empty(t, observedUnexpectedMethod)
}

func TestRequestArchiveCleanupRecoversVersionHiddenByDeleteMarker(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	content := "encrypted-envelope-hidden-by-marker"
	digest := requestArchiveStringDigest(content)
	key := fmt.Sprintf("requests/2026/07/28/902-%s.enc", digest)
	var observationMu sync.Mutex
	deletedVersion := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodHead:
			if request.URL.Query().Get("versionId") != "version-under-marker" {
				http.NotFound(w, request)
				return
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(content)))
			w.Header().Set("x-amz-meta-cipher-sha256", digest)
			w.Header().Set("x-amz-version-id", "version-under-marker")
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			if !request.URL.Query().Has("versions") {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<ListVersionsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>archive-bucket</Name><Prefix>%s</Prefix><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>
  <Version><Key>%s</Key><VersionId>version-under-marker</VersionId><IsLatest>false</IsLatest><LastModified>2026-07-28T00:00:00.000Z</LastModified><ETag>"etag"</ETag><Size>%d</Size><StorageClass>STANDARD</StorageClass></Version>
  <DeleteMarker><Key>%s</Key><VersionId>delete-marker</VersionId><IsLatest>true</IsLatest><LastModified>2026-07-28T00:01:00.000Z</LastModified></DeleteMarker>
</ListVersionsResult>`, key, key, len(content), key)
		case http.MethodDelete:
			observationMu.Lock()
			deletedVersion = request.URL.Query().Get("versionId")
			observationMu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	target := configureRequestArchiveS3TestTarget(t, db, "s3-hidden-version", server.URL)
	now := time.Now().Unix()
	job := model.RequestArchiveJob{
		Status: model.RequestArchiveJobDone, TargetId: target.Id, ObjectKey: key,
		ObjectVersionMode: model.RequestArchiveObjectVersionUnknown, SHA256: strings.Repeat("b", 64),
		CreatedAt: now - 10, FinishedAt: now - 5, ExpiresAt: now - 1,
	}
	require.NoError(t, db.Create(&job).Error)

	removed, err := CleanupExpiredRequestArchiveObjects(context.Background(), now, 10)
	require.NoError(t, err)
	require.Zero(t, removed)
	removed, err = CleanupExpiredRequestArchiveObjects(
		context.Background(), now+int64(requestArchiveCleanupReconcileQuietPeriod/time.Second), 10,
	)
	require.NoError(t, err)
	require.EqualValues(t, 1, removed)
	observationMu.Lock()
	observedDeletedVersion := deletedVersion
	observationMu.Unlock()
	require.Equal(t, "version-under-marker", observedDeletedVersion)
}

func TestRequestArchiveCleanupKeepsTaskWhenVersionCannotBeConfirmed(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	digest := requestArchiveStringDigest("encrypted-envelope-unconfirmed")
	key := fmt.Sprintf("requests/2026/07/28/903-%s.enc", digest)
	var observationMu sync.Mutex
	deleteCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodHead:
			http.NotFound(w, request)
		case http.MethodGet:
			w.WriteHeader(http.StatusServiceUnavailable)
		case http.MethodDelete:
			observationMu.Lock()
			deleteCalled = true
			observationMu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	target := configureRequestArchiveS3TestTarget(t, db, "s3-unconfirmed-version", server.URL)
	now := time.Now().Unix()
	job := model.RequestArchiveJob{
		Status: model.RequestArchiveJobDone, TargetId: target.Id, ObjectKey: key,
		ObjectVersionMode: model.RequestArchiveObjectVersionUnknown, SHA256: strings.Repeat("c", 64),
		CreatedAt: now - 10, FinishedAt: now - 5, ExpiresAt: now - 1,
	}
	require.NoError(t, db.Create(&job).Error)

	removed, err := CleanupExpiredRequestArchiveObjects(context.Background(), now, 10)
	require.NoError(t, err)
	require.Zero(t, removed)
	removed, err = CleanupExpiredRequestArchiveObjects(
		context.Background(), now+int64(requestArchiveCleanupReconcileQuietPeriod/time.Second), 10,
	)
	require.NoError(t, err)
	require.Zero(t, removed)
	observationMu.Lock()
	observedDeleteCalled := deleteCalled
	observationMu.Unlock()
	require.False(t, observedDeleteCalled)
	var retained model.RequestArchiveJob
	require.NoError(t, db.First(&retained, job.Id).Error)
	require.Equal(t, "request_archive_object_version_unconfirmed", retained.LastErrorCode)
}

func TestRequestArchiveCleanupRetainsRowUntilLatePutCanBeDeleted(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	content := "late-committing-envelope"
	digest := requestArchiveStringDigest(content)
	key := fmt.Sprintf("requests/2026/07/28/906-%s.enc", digest)
	var stateMu sync.Mutex
	objectExists := false
	deletedVersion := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		stateMu.Lock()
		defer stateMu.Unlock()
		switch request.Method {
		case http.MethodHead:
			if !objectExists {
				http.NotFound(w, request)
				return
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(content)))
			w.Header().Set("x-amz-meta-cipher-sha256", digest)
			w.Header().Set("x-amz-version-id", "late-version")
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<ListVersionsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>archive-bucket</Name><Prefix>%s</Prefix><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>
</ListVersionsResult>`, key)
		case http.MethodDelete:
			deletedVersion = request.URL.Query().Get("versionId")
			objectExists = false
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	target := configureRequestArchiveS3TestTarget(t, db, "s3-late-commit", server.URL)
	now := time.Now().Unix()
	job := model.RequestArchiveJob{
		Status: model.RequestArchiveJobFailed, TargetId: target.Id, ObjectKey: key,
		ObjectVersionMode: model.RequestArchiveObjectVersionUnknown, SHA256: strings.Repeat("f", 64),
		CreatedAt: now - 10, FinishedAt: now - 5, ExpiresAt: now - 1,
	}
	require.NoError(t, db.Create(&job).Error)

	removed, err := CleanupExpiredRequestArchiveObjects(context.Background(), now, 10)
	require.NoError(t, err)
	require.Zero(t, removed, "首次清理只能开始静默期，不能删除唯一任务记录")
	var retained model.RequestArchiveJob
	require.NoError(t, db.First(&retained, job.Id).Error)
	require.Equal(t, now, retained.CleanupReconcileStartedAt)

	stateMu.Lock()
	objectExists = true
	stateMu.Unlock()
	removed, err = CleanupExpiredRequestArchiveObjects(context.Background(), now+1, 10)
	require.NoError(t, err)
	require.Zero(t, removed, "静默期内出现的晚到对象不能提前删除")
	removed, err = CleanupExpiredRequestArchiveObjects(
		context.Background(), now+int64(requestArchiveCleanupReconcileQuietPeriod/time.Second), 10,
	)
	require.NoError(t, err)
	require.EqualValues(t, 1, removed)
	stateMu.Lock()
	observedDeletedVersion := deletedVersion
	stateMu.Unlock()
	require.Equal(t, "late-version", observedDeletedVersion)
	var remaining int64
	require.NoError(t, db.Model(&model.RequestArchiveJob{}).Where("id = ?", job.Id).Count(&remaining).Error)
	require.Zero(t, remaining)
}

func TestRequestArchiveCleanupUnknownExistingObjectWaitsForAllLateWrites(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	content := "multiple-late-committing-envelope"
	digest := requestArchiveStringDigest(content)
	key := fmt.Sprintf("requests/2026/07/28/907-%s.enc", digest)
	var stateMu sync.Mutex
	objectExists := true
	currentVersion := "first-late-version"
	deletedVersion := ""
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		stateMu.Lock()
		defer stateMu.Unlock()
		requestCount++
		switch request.Method {
		case http.MethodHead:
			if !objectExists {
				http.NotFound(w, request)
				return
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(content)))
			w.Header().Set("x-amz-meta-cipher-sha256", digest)
			w.Header().Set("x-amz-version-id", currentVersion)
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			deletedVersion = request.URL.Query().Get("versionId")
			objectExists = false
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	target := configureRequestArchiveS3TestTarget(t, db, "s3-multiple-late-commit", server.URL)
	now := time.Now().Unix()
	job := model.RequestArchiveJob{
		Status: model.RequestArchiveJobFailed, TargetId: target.Id, ObjectKey: key,
		ObjectVersionMode: model.RequestArchiveObjectVersionUnknown, SHA256: strings.Repeat("1", 64),
		CreatedAt: now - 10, FinishedAt: now - 5, ExpiresAt: now - 1,
	}
	require.NoError(t, db.Create(&job).Error)

	removed, err := CleanupExpiredRequestArchiveObjects(context.Background(), now, 10)
	require.NoError(t, err)
	require.Zero(t, removed)
	stateMu.Lock()
	require.Zero(t, requestCount, "静默期开始时对象已存在也不能立即探测或删除")
	currentVersion = "second-late-version"
	stateMu.Unlock()

	quietSeconds := int64(requestArchiveCleanupReconcileQuietPeriod / time.Second)
	removed, err = CleanupExpiredRequestArchiveObjects(context.Background(), now+quietSeconds-1, 10)
	require.NoError(t, err)
	require.Zero(t, removed)
	stateMu.Lock()
	require.Zero(t, requestCount, "静默期结束前不能接触外部对象")
	stateMu.Unlock()

	removed, err = CleanupExpiredRequestArchiveObjects(context.Background(), now+quietSeconds, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, removed)
	stateMu.Lock()
	observedDeletedVersion := deletedVersion
	observedObjectExists := objectExists
	stateMu.Unlock()
	require.Equal(t, "second-late-version", observedDeletedVersion)
	require.False(t, observedObjectExists)
}

func TestRequestArchiveRetentionDrainHasNoFixedBatchCap(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	now := time.Now().Unix()
	jobs := make([]model.RequestArchiveJob, 2101)
	for index := range jobs {
		jobs[index] = model.RequestArchiveJob{
			Status: model.RequestArchiveJobFailed, TargetId: "expired-target",
			RequestCiphertext: "", SHA256: strings.Repeat("f", 64),
			CreatedAt: now - 10, FinishedAt: now - 5, ExpiresAt: now - 1,
		}
	}
	require.NoError(t, db.CreateInBatches(&jobs, 200).Error)
	removed, err := drainExpiredRequestArchiveObjects(context.Background(), now, 100, 10*time.Second)
	require.NoError(t, err)
	require.EqualValues(t, len(jobs), removed)
	var remaining int64
	require.NoError(t, db.Model(&model.RequestArchiveJob{}).Count(&remaining).Error)
	require.Zero(t, remaining)
}

func TestRequestArchivePrefixAndObjectKeyUseUTF8ByteLimits(t *testing.T) {
	validPrefix := strings.Repeat("界", 170)
	invalidPrefix := strings.Repeat("界", 171)
	_, err := normalizeRequestArchivePrefix(validPrefix)
	require.NoError(t, err)
	_, err = normalizeRequestArchivePrefix(invalidPrefix)
	require.Error(t, err)
	_, err = normalizeRequestArchivePrefix(string([]byte{0xff}))
	require.Error(t, err)

	keyAtLimit := strings.Repeat("a", requestArchiveMaxObjectKeyBytes)
	normalized, err := safeRequestArchiveRelativeKey(keyAtLimit)
	require.NoError(t, err)
	require.Len(t, []byte(normalized), requestArchiveMaxObjectKeyBytes)
	_, err = safeRequestArchiveRelativeKey(keyAtLimit + "a")
	require.Error(t, err)

	job := &model.RequestArchiveJob{
		Id: 8, TargetId: "target-prefix", ConfigVersion: 1,
		RequestCiphertext: "ra1.ciphertext.payload", CreatedAt: time.Now().Unix(),
	}
	key, err := requestArchiveObjectKey(model.RequestArchiveTarget{Prefix: validPrefix}, job)
	require.NoError(t, err)
	require.LessOrEqual(t, len([]byte(key)), requestArchiveMaxObjectKeyBytes)
	job.ObjectKey = keyAtLimit + "a"
	_, _, err = writeRequestArchiveObject(context.Background(), model.RequestArchiveTarget{
		Type: model.RequestArchiveTargetS3,
	}, job)
	require.Error(t, err, "持久化的旧对象键也必须在发往 S3 前重新校验")
}

func TestQueueRequestArchiveUsesShortDatabaseTimeout(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	configureRequestArchiveLocalTarget(t, requestArchiveTestLocalPath(t, "archive"))
	const callbackName = "request_archive_test_block_enqueue_create"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Table != (model.RequestArchiveQueueState{}).TableName() {
			return
		}
		select {
		case <-tx.Statement.Context.Done():
			tx.AddError(tx.Statement.Context.Err())
		case <-time.After(2 * time.Second):
			tx.AddError(errors.New("测试等待入队上下文超时"))
		}
	}))
	t.Cleanup(func() { db.Callback().Create().Remove(callbackName) })

	started := time.Now()
	_, err := QueueRequestArchive(context.Background(), RequestArchiveRequest{
		Body: []byte(`{"input":"enqueue timeout"}`), Method: http.MethodPost, Path: "/v1/responses",
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), time.Second)
}

func TestRequestArchiveWorkerDoesNotClaimWithoutCryptoSecret(t *testing.T) {
	db := setupRequestArchiveServiceTest(t)
	configureRequestArchiveLocalTarget(t, requestArchiveTestLocalPath(t, "archive"))
	result, err := QueueRequestArchive(context.Background(), RequestArchiveRequest{
		Body: []byte(`{"input":"wait for stable key"}`), Method: http.MethodPost, Path: "/v1/responses",
	})
	require.NoError(t, err)
	require.True(t, result.Enqueued)

	t.Setenv("CRYPTO_SECRET", "")
	common.CryptoSecret = ""
	processed, err := ProcessNextRequestArchiveJob(context.Background(), "missing-key-worker")
	require.NoError(t, err)
	require.False(t, processed)

	var job model.RequestArchiveJob
	require.NoError(t, db.First(&job, "id = ?", result.JobId).Error)
	require.Equal(t, model.RequestArchiveJobQueued, job.Status)
	require.Zero(t, job.Attempts)
}

func TestRequestArchiveRuntimeScalesActualWorkers(t *testing.T) {
	ShutdownRequestArchiveRuntime()
	db := setupRequestArchiveServiceTest(t)
	t.Cleanup(ShutdownRequestArchiveRuntime)
	configureRequestArchiveLocalTarget(t, requestArchiveTestLocalPath(t, "archive"))
	require.NoError(t, InitRequestArchiveRuntime())
	require.Eventually(t, func() bool {
		return requestArchiveWorkerRunning.Load() == RequestArchiveDefaultWorkerCount
	}, 3*time.Second, 20*time.Millisecond)

	require.NoError(t, db.WithContext(context.Background()).Model(&model.RequestArchiveConfig{}).
		Where("id = ?", model.RequestArchiveConfigID).Update("worker_count", 2).Error)
	InvalidateRequestArchiveConfig()
	require.Eventually(t, func() bool {
		return requestArchiveWorkerRunning.Load() == 2
	}, 3*time.Second, 20*time.Millisecond)

	require.NoError(t, db.WithContext(context.Background()).Model(&model.RequestArchiveConfig{}).
		Where("id = ?", model.RequestArchiveConfigID).Update("enabled", false).Error)
	InvalidateRequestArchiveConfig()
	require.Eventually(t, func() bool {
		return requestArchiveWorkerRunning.Load() == 1
	}, 3*time.Second, 20*time.Millisecond)

	ShutdownRequestArchiveRuntime()
	require.Eventually(t, func() bool {
		return requestArchiveWorkerRunning.Load() == 0
	}, time.Second, 20*time.Millisecond)
}

func TestRequestArchivePersistenceErrorsHideDatabaseDetails(t *testing.T) {
	t.Run("save config", func(t *testing.T) {
		db := setupRequestArchiveServiceTest(t)
		config, err := GetRequestArchiveConfig(context.Background())
		require.NoError(t, err)
		require.NoError(t, db.Migrator().DropTable(&model.RequestArchiveConfig{}))

		_, err = SaveRequestArchiveConfig(context.Background(), RequestArchiveUpdateRequest{
			ExpectedConfigVersion: config.ConfigVersion,
			RetentionDays:         config.RetentionDays,
			WorkerCount:           config.WorkerCount,
			QueueCapacity:         config.QueueCapacity,
			MaxBodyBytes:          config.MaxBodyBytes,
			QueueMaxBytes:         config.QueueMaxBytes,
		}, 1)
		require.ErrorIs(t, err, ErrRequestArchivePersistence)
		require.Equal(t, ErrRequestArchivePersistence.Error(), err.Error())
		require.NotContains(t, strings.ToLower(err.Error()), "table")
	})

	t.Run("probe target", func(t *testing.T) {
		db := setupRequestArchiveServiceTest(t)
		require.NoError(t, db.Migrator().DropTable(&model.RequestArchiveConfig{}))

		_, err := ProbeRequestArchiveTarget(context.Background(), RequestArchiveUpdateTarget{
			Id: "local-probe", Name: "本地探测", Type: model.RequestArchiveTargetLocal,
			Enabled: true, LocalPath: requestArchiveTestLocalPath(t),
		})
		require.ErrorIs(t, err, ErrRequestArchivePersistence)
		require.Equal(t, ErrRequestArchivePersistence.Error(), err.Error())
		require.NotContains(t, strings.ToLower(err.Error()), "table")
	})
}

package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/model"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

const (
	requestArchiveMaxPrefixBytes    = 512
	requestArchiveMaxObjectKeyBytes = 1024
	requestArchiveMaxEndpointBytes  = 2048
	requestArchiveMaxLocalPathBytes = 4096
	requestArchiveMaxVersionIDBytes = 4096
	requestArchiveS3NullVersionID   = "null"
)

const requestArchiveMaxResponseBytes int64 = 64 * 1024

var errRequestArchiveResponseTooLarge = errors.New("请求归档对象存储响应过大")

var errRequestArchiveObjectVersionUnconfirmed = errors.New("请求归档对象版本无法确认")

var errRequestArchiveStoredObjectNotFound = errors.New("请求归档存储对象不存在")

// 本地文件系统调用本身不接受 Context。只允许一个可脱离清理循环继续收尾的
// 删除操作，避免异常挂载点既阻塞维护协程，又按轮次无限制造后台 goroutine。
var requestArchiveLocalDeleteSlot = make(chan struct{}, 1)

// NormalizeRequestArchiveTarget 统一存储目标，并在保存前阻断不安全的本地
// 路径、带凭据的 URL、以及可逃逸的对象前缀。S3 endpoint 可留空使用 AWS
// 默认终端；填写自定义 endpoint 时可直接用于 Cloudflare R2 或 MinIO。
func NormalizeRequestArchiveTarget(target *model.RequestArchiveTarget) error {
	if target == nil {
		return errors.New("请求归档存储目标不能为空")
	}
	target.Type = strings.ToLower(strings.TrimSpace(target.Type))
	switch target.Type {
	case model.RequestArchiveTargetLocal:
		if strings.TrimSpace(target.LocalPath) == "" {
			return errors.New("本地请求归档存储路径不能为空")
		}
		if len(target.LocalPath) > requestArchiveMaxLocalPathBytes || strings.ContainsRune(target.LocalPath, 0) {
			return errors.New("本地请求归档存储路径无效")
		}
		if !filepath.IsAbs(target.LocalPath) {
			return errors.New("本地请求归档存储路径必须为绝对路径")
		}
		absolute, err := filepath.Abs(filepath.Clean(target.LocalPath))
		if err != nil {
			return errors.New("本地请求归档存储路径无效")
		}
		if err := validateRequestArchiveLocalRootShape(absolute); err != nil {
			return err
		}
		if err := validateRequestArchiveLocalExistingHierarchy(absolute); err != nil {
			return err
		}
		target.LocalPath = absolute
		target.Endpoint, target.Bucket, target.Region, target.Prefix = "", "", "", ""
		target.PathStyle = false
		target.AccessKeyCiphertext, target.SecretKeyCiphertext = "", ""
		return nil
	case model.RequestArchiveTargetS3:
		endpoint, err := normalizeRequestArchiveEndpoint(target.Endpoint)
		if err != nil {
			return err
		}
		bucket := strings.TrimSpace(target.Bucket)
		if len(bucket) < 3 || len(bucket) > 255 || strings.ContainsAny(bucket, "/\\?#@") || strings.ContainsAny(bucket, " \t\r\n") {
			return errors.New("S3 兼容存储 bucket 无效")
		}
		prefix, err := normalizeRequestArchivePrefix(target.Prefix)
		if err != nil {
			return err
		}
		region := trimRequestArchiveValue(target.Region, 128)
		if region == "" {
			region = "us-east-1"
		}
		target.LocalPath = ""
		target.Endpoint, target.Bucket, target.Region, target.Prefix = endpoint, bucket, region, prefix
		return nil
	default:
		return errors.New("请求归档存储类型仅支持 local 或 s3")
	}
}

func normalizeRequestArchiveEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if len(raw) > requestArchiveMaxEndpointBytes || strings.ContainsRune(raw, 0) {
		return "", errors.New("S3 兼容存储 endpoint 无效")
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed == nil || parsed.Opaque != "" || parsed.Host == "" || parsed.User != nil {
		return "", errors.New("S3 兼容存储 endpoint 无效")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("S3 兼容存储 endpoint 必须使用 HTTP 或 HTTPS")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.ForceQuery {
		return "", errors.New("S3 兼容存储 endpoint 不能包含查询参数或片段")
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return "", errors.New("S3 兼容存储 endpoint 缺少主机")
	}
	if strings.Contains(parsed.Hostname(), "%") || isForbiddenPromptAuditHostname(parsed.Hostname()) {
		return "", errors.New("S3 兼容存储 endpoint 不能指向云元数据或 link-local 地址")
	}
	if parsed.Scheme == "http" {
		if endpointIP := net.ParseIP(parsed.Hostname()); endpointIP != nil && !isRequestArchivePrivateEndpointIP(endpointIP) {
			return "", errors.New("S3 兼容存储 HTTP endpoint 仅允许回环或私网地址")
		}
	}
	if strings.Contains(parsed.EscapedPath(), "..") {
		return "", errors.New("S3 兼容存储 endpoint 路径无效")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	return parsed.String(), nil
}

func normalizeRequestArchivePrefix(value string) (string, error) {
	value = strings.Trim(strings.TrimSpace(strings.ReplaceAll(value, "\\", "/")), "/")
	if value == "" {
		return "", nil
	}
	if !utf8.ValidString(value) {
		return "", errors.New("请求归档对象前缀不是有效 UTF-8")
	}
	if len(value) > requestArchiveMaxPrefixBytes {
		return "", errors.New("请求归档对象前缀过长")
	}
	if strings.ContainsRune(value, 0) {
		return "", errors.New("请求归档对象前缀无效")
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || component == "." || component == ".." {
			return "", errors.New("请求归档对象前缀不能包含相对路径")
		}
	}
	cleaned := path.Clean(value)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", errors.New("请求归档对象前缀无效")
	}
	return cleaned, nil
}

func requestArchiveObjectKey(target model.RequestArchiveTarget, job *model.RequestArchiveJob) (string, error) {
	if job == nil || job.Id <= 0 || job.RequestCiphertext == "" {
		return "", errors.New("请求归档对象键参数无效")
	}
	prefix, err := normalizeRequestArchivePrefix(target.Prefix)
	if err != nil {
		return "", err
	}
	createdAt := job.CreatedAt
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}
	date := time.Unix(createdAt, 0).UTC()
	key := fmt.Sprintf("requests/%04d/%02d/%02d/%d-%s.enc",
		date.Year(), date.Month(), date.Day(), job.Id, requestArchiveCiphertextDigest(job))
	if prefix != "" {
		key = prefix + "/" + key
	}
	return safeRequestArchiveRelativeKey(key)
}

func requestArchiveCiphertextDigest(job *model.RequestArchiveJob) string {
	if job == nil {
		return ""
	}
	digest := sha256.New()
	_, _ = io.WriteString(digest, string(job.RequestCiphertext))
	return hex.EncodeToString(digest.Sum(nil))
}

func safeRequestArchiveRelativeKey(key string) (string, error) {
	key = strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
	if key == "" || !utf8.ValidString(key) || strings.HasPrefix(key, "/") || path.IsAbs(key) || strings.ContainsRune(key, 0) {
		return "", errors.New("请求归档对象键无效")
	}
	components := strings.Split(key, "/")
	for _, component := range components {
		if component == "" || component == "." || component == ".." || strings.Contains(component, ":") {
			return "", errors.New("请求归档对象键必须是安全相对路径")
		}
	}
	cleaned := path.Clean(key)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", errors.New("请求归档对象键无效")
	}
	if len(cleaned) > requestArchiveMaxObjectKeyBytes {
		return "", errors.New("请求归档对象键超过 S3 兼容存储限制")
	}
	return cleaned, nil
}

func writeRequestArchiveObject(ctx context.Context, target model.RequestArchiveTarget, job *model.RequestArchiveJob) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	key := job.ObjectKey
	if key == "" {
		var err error
		key, err = requestArchiveObjectKey(target, job)
		if err != nil {
			return "", "", err
		}
	}
	key, err := safeRequestArchiveRelativeKey(key)
	if err != nil {
		return "", "", err
	}
	if existingVersion, exists, err := requestArchiveObjectAlreadyStored(ctx, target, key, job); err != nil {
		return "", "", err
	} else if exists {
		return key, requestArchiveVersionIDForTarget(target, existingVersion), nil
	}
	switch target.Type {
	case model.RequestArchiveTargetLocal:
		location, err := safeRequestArchiveLocalPath(target.LocalPath, key)
		if err != nil {
			return "", "", err
		}
		if err := atomicWriteRequestArchiveFile(target.LocalPath, location, string(job.RequestCiphertext)); err != nil {
			return "", "", err
		}
		return key, "", nil
	case model.RequestArchiveTargetS3:
		versionID, err := putRequestArchiveS3Object(ctx, target, key, string(job.RequestCiphertext))
		if err != nil {
			return "", "", err
		}
		return key, requestArchiveVersionIDForTarget(target, versionID), nil
	default:
		return "", "", errors.New("请求归档任务使用了未知存储类型")
	}
}

// requestArchiveObjectAlreadyStored 处理“外部写入成功但数据库完成事务失败”
// 的恢复场景。对象键由任务 ID 决定，已存在且哈希一致时直接完成任务，避免
// 版本化 bucket 因重试产生多份不可精确删除的对象版本。
func requestArchiveObjectAlreadyStored(ctx context.Context, target model.RequestArchiveTarget, key string, job *model.RequestArchiveJob) (string, bool, error) {
	if job == nil || key == "" {
		return "", false, nil
	}
	switch target.Type {
	case model.RequestArchiveTargetLocal:
		directory, objectName, location, err := openRequestArchiveLocalObjectParent(target.LocalPath, key, false)
		if os.IsNotExist(err) {
			return "", false, nil
		} else if err != nil {
			return "", false, err
		}
		defer directory.Close()
		info, err := directory.Lstat(objectName)
		if os.IsNotExist(err) {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() != int64(len(job.RequestCiphertext)) {
			return "", false, errors.New("请求归档本地对象完整性校验失败")
		}
		if err := validateRequestArchiveLocalPlatformPath(location); err != nil {
			return "", false, err
		}
		file, err := directory.Open(objectName)
		if err != nil {
			return "", false, err
		}
		openedInfo, statErr := file.Stat()
		if statErr != nil || !os.SameFile(info, openedInfo) || !openedInfo.Mode().IsRegular() {
			_ = file.Close()
			return "", false, errors.New("请求归档本地对象在打开期间发生变化")
		}
		if err := validateRequestArchiveLocalPlatformPath(location); err != nil {
			_ = file.Close()
			return "", false, err
		}
		digest := sha256.New()
		_, copyErr := io.Copy(digest, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", false, copyErr
		}
		if closeErr != nil {
			return "", false, closeErr
		}
		if !strings.EqualFold(hex.EncodeToString(digest.Sum(nil)), requestArchiveCiphertextDigest(job)) {
			return "", false, errors.New("请求归档本地对象完整性校验失败")
		}
		return "", true, nil
	case model.RequestArchiveTargetS3:
		accessKey, secretKey, err := requestArchiveS3Credentials(target)
		if err != nil {
			return "", false, err
		}
		return inspectRequestArchiveS3Object(
			ctx,
			newRequestArchiveS3Client(target, accessKey, secretKey),
			target.Bucket,
			key,
			requestArchiveCiphertextDigest(job),
			int64(len(job.RequestCiphertext)),
			job.Attempts > 1,
			isRequestArchiveR2Endpoint(target.Endpoint),
		)
	default:
		return "", false, errors.New("请求归档任务使用了未知存储类型")
	}
}

func requestArchiveObjectNotFound(err error) bool {
	return requestArchiveObjectHTTPStatus(err, http.StatusNotFound)
}

func requestArchiveObjectHTTPStatus(err error, status int) bool {
	var responseError *smithyhttp.ResponseError
	return errors.As(err, &responseError) && responseError.HTTPStatusCode() == status
}

func validateRequestArchiveS3Object(head *s3.HeadObjectOutput, expectedDigest string, expectedLength int64) (string, error) {
	if head == nil || head.Metadata == nil || head.Metadata["cipher-sha256"] != expectedDigest {
		return "", errors.New("请求归档对象完整性校验失败")
	}
	if expectedLength >= 0 && (head.ContentLength == nil || *head.ContentLength != expectedLength) {
		return "", errors.New("请求归档对象完整性校验失败")
	}
	return aws.ToString(head.VersionId), nil
}

// inspectRequestArchiveS3Object 恢复“对象已提交、版本号尚未落库”的状态。
// 当前对象被删除标记遮蔽时，会按精确 key 查询历史版本并逐个校验密文摘要。
// 无法唯一确认时必须返回稳定错误，调用方不得退化为空版本删除。
func inspectRequestArchiveS3Object(
	ctx context.Context,
	client *s3.Client,
	bucket string,
	key string,
	expectedDigest string,
	expectedLength int64,
	searchHistoricalVersions bool,
	knownUnversioned bool,
) (string, bool, error) {
	if client == nil || strings.TrimSpace(bucket) == "" || strings.TrimSpace(key) == "" || len(expectedDigest) != sha256.Size*2 {
		return "", false, errRequestArchiveObjectVersionUnconfirmed
	}
	currentExists := false
	currentMatches := false
	head, headErr := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if headErr == nil {
		currentExists = true
		versionID, validateErr := validateRequestArchiveS3Object(head, expectedDigest, expectedLength)
		if validateErr == nil {
			if versionID != "" {
				if !validRequestArchiveS3VersionID(versionID) {
					return "", false, errRequestArchiveObjectVersionUnconfirmed
				}
				return versionID, true, nil
			}
			currentMatches = true
		}
	} else if !requestArchiveObjectNotFound(headErr) {
		return "", false, fmt.Errorf("%w: head object", errRequestArchiveObjectVersionUnconfirmed)
	} else if !searchHistoricalVersions || knownUnversioned {
		return "", false, nil
	}
	if currentMatches && knownUnversioned {
		return "", true, nil
	}

	versions, versionsErr := client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
		Bucket: aws.String(bucket), Prefix: aws.String(key), MaxKeys: aws.Int32(1000),
	})
	if versionsErr == nil {
		versionID, exactVersions, inspectErr := inspectRequestArchiveS3Versions(
			ctx, client, bucket, key, expectedDigest, expectedLength, versions,
		)
		if inspectErr != nil {
			return "", false, inspectErr
		}
		if versionID != "" {
			return versionID, true, nil
		}
		if exactVersions > 0 && !currentMatches {
			return "", false, errRequestArchiveObjectVersionUnconfirmed
		}
		if !currentExists {
			return "", false, nil
		}
	}
	if currentExists && !currentMatches {
		return "", false, errRequestArchiveObjectVersionUnconfirmed
	}

	if currentMatches {
		versioning, versioningErr := client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: aws.String(bucket)})
		if versioningErr == nil && versioning != nil && versioning.Status == "" {
			// Head 已校验对象摘要且 bucket 明确未启用版本控制。空版本在数据库
			// 中记为“已确认”，删除时使用显式 versionId=null。
			return "", true, nil
		}
		if versioningErr == nil && versioning != nil &&
			(versioning.Status == s3types.BucketVersioningStatusEnabled || versioning.Status == s3types.BucketVersioningStatusSuspended) {
			return "", false, errRequestArchiveObjectVersionUnconfirmed
		}
	}
	return "", false, errRequestArchiveObjectVersionUnconfirmed
}

func inspectRequestArchiveS3Versions(
	ctx context.Context,
	client *s3.Client,
	bucket string,
	key string,
	expectedDigest string,
	expectedLength int64,
	versions *s3.ListObjectVersionsOutput,
) (string, int, error) {
	if versions == nil || aws.ToBool(versions.IsTruncated) {
		return "", 0, errRequestArchiveObjectVersionUnconfirmed
	}
	exactVersions := 0
	matchingVersions := make([]string, 0, 1)
	for _, version := range versions.Versions {
		if aws.ToString(version.Key) != key {
			continue
		}
		exactVersions++
		versionID := aws.ToString(version.VersionId)
		if !validRequestArchiveS3VersionID(versionID) {
			if strings.TrimSpace(versionID) == "" {
				continue
			}
			return "", exactVersions, errRequestArchiveObjectVersionUnconfirmed
		}
		head, err := client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(bucket), Key: aws.String(key), VersionId: aws.String(versionID),
		})
		if err != nil {
			return "", exactVersions, fmt.Errorf("%w: head object version", errRequestArchiveObjectVersionUnconfirmed)
		}
		if _, err := validateRequestArchiveS3Object(head, expectedDigest, expectedLength); err == nil {
			matchingVersions = append(matchingVersions, versionID)
		}
	}
	if len(matchingVersions) != 1 {
		return "", exactVersions, nil
	}
	return matchingVersions[0], exactVersions, nil
}

func requestArchiveCipherDigestFromObjectKey(key string) (string, error) {
	key, err := safeRequestArchiveRelativeKey(key)
	if err != nil {
		return "", errRequestArchiveObjectVersionUnconfirmed
	}
	name := path.Base(key)
	if !strings.HasSuffix(name, ".enc") {
		return "", errRequestArchiveObjectVersionUnconfirmed
	}
	name = strings.TrimSuffix(name, ".enc")
	separator := strings.LastIndexByte(name, '-')
	if separator < 1 || separator+1 >= len(name) {
		return "", errRequestArchiveObjectVersionUnconfirmed
	}
	digest := name[separator+1:]
	decoded, decodeErr := hex.DecodeString(digest)
	if decodeErr != nil || len(decoded) != sha256.Size {
		return "", errRequestArchiveObjectVersionUnconfirmed
	}
	return strings.ToLower(digest), nil
}

func resolveRequestArchiveS3ObjectVersion(ctx context.Context, target model.RequestArchiveTarget, key string) (string, bool, error) {
	digest, err := requestArchiveCipherDigestFromObjectKey(key)
	if err != nil {
		return "", false, err
	}
	accessKey, secretKey, err := requestArchiveS3Credentials(target)
	if err != nil {
		return "", false, fmt.Errorf("%w: credentials", errRequestArchiveObjectVersionUnconfirmed)
	}
	return inspectRequestArchiveS3Object(
		ctx, newRequestArchiveS3Client(target, accessKey, secretKey), target.Bucket, key, digest, -1, true,
		isRequestArchiveR2Endpoint(target.Endpoint),
	)
}

func validRequestArchiveS3VersionID(versionID string) bool {
	return strings.TrimSpace(versionID) != "" && len(versionID) <= requestArchiveMaxVersionIDBytes &&
		!strings.ContainsRune(versionID, 0)
}

func requestArchiveObjectVersionModeForID(versionID string) string {
	if versionID == "" {
		return model.RequestArchiveObjectVersionUnversioned
	}
	return model.RequestArchiveObjectVersionExact
}

func requestArchiveVersionIDForTarget(target model.RequestArchiveTarget, versionID string) string {
	if target.Type == model.RequestArchiveTargetS3 && versionID == "" && !isRequestArchiveR2Endpoint(target.Endpoint) {
		return requestArchiveS3NullVersionID
	}
	return versionID
}

func isRequestArchiveR2Endpoint(endpoint string) bool {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed == nil {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	return host == "r2.cloudflarestorage.com" || strings.HasSuffix(host, ".r2.cloudflarestorage.com")
}

func safeRequestArchiveLocalPath(root, key string) (string, error) {
	if strings.TrimSpace(root) == "" || !filepath.IsAbs(root) {
		return "", errors.New("本地请求归档存储根路径无效")
	}
	key, err := safeRequestArchiveRelativeKey(key)
	if err != nil {
		return "", err
	}
	rootAbsolute, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", errors.New("本地请求归档存储根路径无效")
	}
	destination := filepath.Join(rootAbsolute, filepath.FromSlash(key))
	relative, err := filepath.Rel(rootAbsolute, destination)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", errors.New("请求归档对象键逃逸本地存储根路径")
	}
	return destination, nil
}

func validateRequestArchiveLocalRootShape(rootAbsolute string) error {
	if strings.TrimSpace(rootAbsolute) == "" || !filepath.IsAbs(rootAbsolute) {
		return errors.New("本地请求归档存储根路径无效")
	}
	volume := filepath.VolumeName(rootAbsolute)
	if runtime.GOOS == "windows" && strings.HasPrefix(volume, `\\`) {
		return errors.New("本地请求归档存储不支持 UNC 或网络共享路径")
	}
	anchor := volume + string(filepath.Separator)
	if filepath.Clean(rootAbsolute) == filepath.Clean(anchor) {
		return errors.New("本地请求归档存储不能使用卷根目录")
	}
	relative, err := filepath.Rel(anchor, rootAbsolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return errors.New("本地请求归档存储路径无效")
	}
	return nil
}

func validateRequestArchiveLocalExistingHierarchy(rootAbsolute string) error {
	root, err := openRequestArchiveLocalRoot(rootAbsolute, false)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return root.Close()
}

// openRequestArchiveLocalRoot 从卷根目录开始逐级取得目录句柄。每一级都在
// os.Root 内完成检查、创建和打开，后续文件操作不会因路径替换逃逸配置根目录。
func openRequestArchiveLocalRoot(rootAbsolute string, create bool) (*os.Root, error) {
	rootAbsolute, err := filepath.Abs(filepath.Clean(rootAbsolute))
	if err != nil {
		return nil, errors.New("本地请求归档存储根路径无效")
	}
	if err := validateRequestArchiveLocalRootShape(rootAbsolute); err != nil {
		return nil, err
	}
	volume := filepath.VolumeName(rootAbsolute)
	anchor := volume + string(filepath.Separator)
	relative, err := filepath.Rel(anchor, rootAbsolute)
	if err != nil {
		return nil, errors.New("本地请求归档存储根路径无效")
	}
	current, err := os.OpenRoot(anchor)
	if err != nil {
		return nil, err
	}
	currentPath := anchor
	for _, component := range splitRequestArchiveLocalPath(relative) {
		nextPath := filepath.Join(currentPath, component)
		next, openErr := openRequestArchiveLocalChildRoot(current, component, nextPath, create)
		_ = current.Close()
		if openErr != nil {
			return nil, openErr
		}
		current = next
		currentPath = nextPath
	}
	return current, nil
}

func openRequestArchiveLocalChildRoot(parent *os.Root, component, absolutePath string, create bool) (*os.Root, error) {
	if parent == nil || component == "" || component == "." || component == ".." || filepath.IsAbs(component) {
		return nil, errors.New("请求归档本地目录层级无效")
	}
	info, err := parent.Lstat(component)
	if os.IsNotExist(err) && create {
		if mkdirErr := parent.Mkdir(component, 0o700); mkdirErr != nil && !os.IsExist(mkdirErr) {
			return nil, mkdirErr
		}
		info, err = parent.Lstat(component)
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("请求归档本地目录不能包含符号链接或重解析点")
	}
	if err := validateRequestArchiveLocalPlatformPath(absolutePath); err != nil {
		return nil, err
	}
	next, err := parent.OpenRoot(component)
	if err != nil {
		return nil, err
	}
	openedInfo, err := next.Stat(".")
	if err != nil || !os.SameFile(info, openedInfo) || !openedInfo.IsDir() {
		_ = next.Close()
		return nil, errors.New("请求归档本地目录在打开期间发生变化")
	}
	if err := validateRequestArchiveLocalPlatformPath(absolutePath); err != nil {
		_ = next.Close()
		return nil, err
	}
	return next, nil
}

func splitRequestArchiveLocalPath(relative string) []string {
	components := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	result := make([]string, 0, len(components))
	for _, component := range components {
		if component != "" && component != "." {
			result = append(result, component)
		}
	}
	return result
}

func openRequestArchiveLocalObjectParent(root, key string, create bool) (*os.Root, string, string, error) {
	key, err := safeRequestArchiveRelativeKey(key)
	if err != nil {
		return nil, "", "", err
	}
	rootAbsolute, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, "", "", errors.New("本地请求归档存储根路径无效")
	}
	localRelative := filepath.FromSlash(key)
	objectName := filepath.Base(localRelative)
	if objectName == "" || objectName == "." || objectName == ".." {
		return nil, "", "", errors.New("请求归档本地对象名称无效")
	}
	current, err := openRequestArchiveLocalRoot(rootAbsolute, create)
	if err != nil {
		return nil, "", "", err
	}
	currentPath := rootAbsolute
	for _, component := range splitRequestArchiveLocalPath(filepath.Dir(localRelative)) {
		nextPath := filepath.Join(currentPath, component)
		next, openErr := openRequestArchiveLocalChildRoot(current, component, nextPath, create)
		_ = current.Close()
		if openErr != nil {
			return nil, "", "", openErr
		}
		current = next
		currentPath = nextPath
	}
	return current, objectName, filepath.Join(currentPath, objectName), nil
}

func createRequestArchiveLocalTemp(directory *os.Root, prefix string) (*os.File, string, error) {
	if directory == nil {
		return nil, "", errors.New("请求归档本地目录未打开")
	}
	for attempt := 0; attempt < 32; attempt++ {
		random := make([]byte, 16)
		if _, err := rand.Read(random); err != nil {
			return nil, "", err
		}
		name := prefix + hex.EncodeToString(random) + ".tmp"
		file, err := directory.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		return file, name, err
	}
	return nil, "", errors.New("无法创建请求归档本地临时文件")
}

func atomicWriteRequestArchiveFile(root, destination, content string) error {
	rootAbsolute, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return errors.New("本地请求归档存储根路径无效")
	}
	destinationAbsolute, err := filepath.Abs(filepath.Clean(destination))
	if err != nil {
		return errors.New("请求归档本地对象路径无效")
	}
	relative, err := filepath.Rel(rootAbsolute, destinationAbsolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return errors.New("请求归档对象键逃逸本地存储根路径")
	}
	key, err := safeRequestArchiveRelativeKey(filepath.ToSlash(relative))
	if err != nil {
		return err
	}
	directory, objectName, _, err := openRequestArchiveLocalObjectParent(rootAbsolute, key, true)
	if err != nil {
		return err
	}
	defer directory.Close()
	temporary, temporaryName, err := createRequestArchiveLocalTemp(directory, ".request-archive-")
	if err != nil {
		return err
	}
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = directory.Remove(temporaryName)
		}
	}()
	if _, err := io.WriteString(temporary, content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if info, statErr := directory.Lstat(objectName); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("请求归档本地对象必须是普通文件")
		}
		return errors.New("请求归档本地对象已经存在")
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	if err := directory.Rename(temporaryName, objectName); err != nil {
		return err
	}
	removeTemporary = false
	return nil
}

func putRequestArchiveS3Object(ctx context.Context, target model.RequestArchiveTarget, key, content string) (string, error) {
	accessKey, secretKey, err := requestArchiveS3Credentials(target)
	if err != nil {
		return "", err
	}
	client := newRequestArchiveS3Client(target, accessKey, secretKey)
	output, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(target.Bucket),
		Key:         aws.String(key),
		Body:        strings.NewReader(content),
		ContentType: aws.String("application/vnd.newapi.request-archive-envelope"),
		IfNoneMatch: aws.String("*"),
		Metadata: map[string]string{
			"cipher-sha256": requestArchiveStringDigest(content),
		},
	})
	if err != nil {
		if requestArchiveObjectHTTPStatus(err, http.StatusPreconditionFailed) {
			head, headErr := client.HeadObject(ctx, &s3.HeadObjectInput{
				Bucket: aws.String(target.Bucket), Key: aws.String(key),
			})
			if headErr != nil {
				return "", headErr
			}
			return validateRequestArchiveS3Object(head, requestArchiveStringDigest(content), int64(len(content)))
		}
		return "", err
	}
	return aws.ToString(output.VersionId), nil
}

func requestArchiveBytesDigest(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func requestArchiveStringDigest(content string) string {
	digest := sha256.New()
	_, _ = io.WriteString(digest, content)
	return hex.EncodeToString(digest.Sum(nil))
}

func requestArchiveS3Credentials(target model.RequestArchiveTarget) (string, string, error) {
	if !RequestArchiveCryptoReady() {
		return "", "", errors.New("请求归档存储密钥不可用")
	}
	accessKey, err := DecryptRequestArchiveSecret(target.AccessKeyCiphertext, target.Id, requestArchiveAccessKeyPurpose)
	if err != nil || strings.TrimSpace(accessKey) == "" {
		return "", "", errors.New("请求归档存储访问密钥不可用")
	}
	secretKey, err := DecryptRequestArchiveSecret(target.SecretKeyCiphertext, target.Id, requestArchiveSecretKeyPurpose)
	if err != nil || strings.TrimSpace(secretKey) == "" {
		return "", "", errors.New("请求归档存储密钥不可用")
	}
	return accessKey, secretKey, nil
}

func newRequestArchiveS3Client(target model.RequestArchiveTarget, accessKey, secretKey string) *s3.Client {
	config := aws.Config{
		Region:      target.Region,
		Credentials: credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		HTTPClient:  requestArchiveHTTPClientForTarget(target),
		// PutObject 使用条件写并由数据库任务重试。SDK 内部不能自动重放
		// 已经可能落盘的请求，否则版本化 bucket 会产生不可追踪版本。
		RetryMaxAttempts: 1,
	}
	return s3.NewFromConfig(config, func(options *s3.Options) {
		options.UsePathStyle = target.PathStyle
		if target.Endpoint != "" {
			endpoint := target.Endpoint
			options.BaseEndpoint = &endpoint
		}
	})
}

var (
	requestArchiveSharedHTTPSClient       = newRequestArchiveSecureHTTPClient(false)
	requestArchiveSharedPrivateHTTPClient = newRequestArchiveSecureHTTPClient(true)
)

func requestArchiveHTTPClientForTarget(target model.RequestArchiveTarget) *http.Client {
	if parsed, err := url.Parse(target.Endpoint); err == nil && parsed != nil && strings.EqualFold(parsed.Scheme, "http") {
		return requestArchiveSharedPrivateHTTPClient
	}
	return requestArchiveSharedHTTPSClient
}

func newRequestArchiveSecureHTTPClient(requirePrivateNetwork bool) *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	// 归档端点不能因为进程环境变量而绕经未知代理，也不能接受重定向到
	// 另一个地址。Root 仍可显式配置内网 MinIO 或 R2 终端。
	transport.DisableCompression = true
	transport.ResponseHeaderTimeout = 10 * time.Second
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.New("请求归档存储拨号地址无效")
		}
		if isForbiddenPromptAuditHostname(host) {
			return nil, errors.New("请求归档存储不能指向云元数据或 link-local 地址")
		}
		resolved, err := resolvePromptAuditHost(ctx, host)
		if err != nil {
			return nil, errors.New("请求归档存储 DNS 解析失败")
		}
		if len(resolved) == 0 {
			return nil, errors.New("请求归档存储 DNS 未返回可用地址")
		}
		if err := validateRequestArchiveResolvedAddresses(resolved, requirePrivateNetwork); err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(resolved[0].IP.String(), port))
	}
	return &http.Client{
		Transport: requestArchiveResponseLimitTransport{base: transport, limit: requestArchiveMaxResponseBytes},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		// Worker 通过按密文大小计算的 Context 设置更短边界；这里保留
		// 5 分钟硬上限，避免默认 85 MiB 左右的密文被固定 20 秒误杀。
		Timeout: requestArchiveStorageMaxTimeout,
	}
}

func validateRequestArchiveResolvedAddresses(resolved []net.IPAddr, requirePrivateNetwork bool) error {
	if len(resolved) == 0 {
		return errors.New("请求归档存储 DNS 未返回可用地址")
	}
	for _, resolvedAddress := range resolved {
		if isForbiddenPromptAuditIP(resolvedAddress.IP) {
			return errors.New("请求归档存储 DNS 结果包含云元数据或 link-local 地址")
		}
		if requirePrivateNetwork && !isRequestArchivePrivateEndpointIP(resolvedAddress.IP) {
			return errors.New("请求归档存储 HTTP endpoint 的 DNS 结果必须全部属于回环或私网")
		}
	}
	return nil
}

func isRequestArchivePrivateEndpointIP(ip net.IP) bool {
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

// DeleteRequestArchiveObject 是精确对象删除预留接口。它仅接受已经通过
// safeRequestArchiveRelativeKey 校验的单个键，永不枚举或递归删除目录。
func DeleteRequestArchiveObject(ctx context.Context, target model.RequestArchiveTarget, key, objectVersionID string) error {
	if target.Type == model.RequestArchiveTargetS3 && objectVersionID == "" {
		return errRequestArchiveObjectVersionUnconfirmed
	}
	return deleteRequestArchiveObject(
		ctx, target, key, objectVersionID, requestArchiveObjectVersionModeForID(objectVersionID),
	)
}

func deleteRequestArchiveObject(
	ctx context.Context,
	target model.RequestArchiveTarget,
	key string,
	objectVersionID string,
	objectVersionMode string,
) error {
	key, err := safeRequestArchiveRelativeKey(key)
	if err != nil {
		return err
	}
	switch target.Type {
	case model.RequestArchiveTargetLocal:
		return deleteRequestArchiveLocalObject(ctx, target.LocalPath, key)
	case model.RequestArchiveTargetS3:
		if objectVersionMode == model.RequestArchiveObjectVersionExact && !validRequestArchiveS3VersionID(objectVersionID) {
			return errRequestArchiveObjectVersionUnconfirmed
		}
		if objectVersionMode == model.RequestArchiveObjectVersionUnversioned && objectVersionID != "" {
			return errRequestArchiveObjectVersionUnconfirmed
		}
		if objectVersionMode != model.RequestArchiveObjectVersionExact &&
			objectVersionMode != model.RequestArchiveObjectVersionUnversioned {
			return errRequestArchiveObjectVersionUnconfirmed
		}
		accessKey, secretKey, err := requestArchiveS3Credentials(target)
		if err != nil {
			return err
		}
		input := &s3.DeleteObjectInput{Bucket: aws.String(target.Bucket), Key: aws.String(key)}
		if objectVersionMode == model.RequestArchiveObjectVersionExact {
			input.VersionId = aws.String(objectVersionID)
		}
		_, err = newRequestArchiveS3Client(target, accessKey, secretKey).DeleteObject(ctx, input)
		return err
	default:
		return errors.New("请求归档对象使用了未知存储类型")
	}
}

func deleteRequestArchiveLocalObject(ctx context.Context, root, key string) error {
	select {
	case requestArchiveLocalDeleteSlot <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	done := make(chan error, 1)
	go func() {
		defer func() { <-requestArchiveLocalDeleteSlot }()
		directory, objectName, location, err := openRequestArchiveLocalObjectParent(root, key, false)
		if os.IsNotExist(err) {
			done <- errRequestArchiveStoredObjectNotFound
			return
		}
		if err != nil {
			done <- err
			return
		}
		defer directory.Close()
		var info os.FileInfo
		info, err = directory.Lstat(objectName)
		if os.IsNotExist(err) {
			done <- errRequestArchiveStoredObjectNotFound
			return
		}
		if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
			err = errors.New("请求归档本地对象必须是普通文件")
		}
		if err == nil {
			err = validateRequestArchiveLocalPlatformPath(location)
		}
		if err == nil {
			opened, openErr := directory.Open(objectName)
			if openErr == nil {
				openedInfo, statErr := opened.Stat()
				_ = opened.Close()
				if statErr != nil || !os.SameFile(info, openedInfo) || !openedInfo.Mode().IsRegular() {
					openErr = errors.New("请求归档本地对象在删除前发生变化")
				}
			}
			err = openErr
		}
		if err == nil {
			err = validateRequestArchiveLocalPlatformPath(location)
		}
		if err == nil {
			err = directory.Remove(objectName)
			if os.IsNotExist(err) {
				err = errRequestArchiveStoredObjectNotFound
			}
		}
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ProbeRequestArchiveTarget 验证一个尚未保存或已保存的存储目标。它不会上传
// 归档正文；本地目标只创建并删除一个零字节临时文件，S3/R2 仅执行 HeadBucket。
// 返回值刻意不包含底层错误，以免在管理接口或日志中泄露 endpoint 或凭据。
func ProbeRequestArchiveTarget(ctx context.Context, input RequestArchiveUpdateTarget) (RequestArchiveProbeResult, error) {
	if err := ctx.Err(); err != nil {
		return RequestArchiveProbeResult{}, err
	}
	_, existing, err := model.LoadRequestArchiveConfig(ctx)
	if err != nil {
		return RequestArchiveProbeResult{}, wrapRequestArchivePersistenceError(err)
	}
	byID := make(map[string]model.RequestArchiveTarget, len(existing))
	for _, target := range existing {
		byID[target.Id] = target
	}
	target, err := buildRequestArchiveTarget(input, byID)
	if err != nil {
		return RequestArchiveProbeResult{}, err
	}

	started := time.Now()
	switch target.Type {
	case model.RequestArchiveTargetLocal:
		err = probeRequestArchiveLocalTarget(target)
	case model.RequestArchiveTargetS3:
		var accessKey, secretKey string
		accessKey, secretKey, err = requestArchiveS3Credentials(target)
		if err == nil {
			_, err = newRequestArchiveS3Client(target, accessKey, secretKey).HeadBucket(ctx, &s3.HeadBucketInput{
				Bucket: aws.String(target.Bucket),
			})
		}
	default:
		err = errors.New("未知请求归档存储类型")
	}
	latency := time.Since(started).Milliseconds()
	if err != nil {
		return RequestArchiveProbeResult{
			Healthy: false, LatencyMs: latency, Status: "unhealthy",
			ErrorCode: "request_archive_target_unavailable", Message: "存储目标暂时不可用",
		}, nil
	}
	return RequestArchiveProbeResult{
		Healthy: true, LatencyMs: latency, Status: "healthy", Message: "存储目标可用",
	}, nil
}

func probeRequestArchiveLocalTarget(target model.RequestArchiveTarget) error {
	rootAbsolute, err := filepath.Abs(filepath.Clean(target.LocalPath))
	if err != nil {
		return err
	}
	if err := validateRequestArchiveLocalRootShape(rootAbsolute); err != nil {
		return err
	}
	root, err := openRequestArchiveLocalRoot(rootAbsolute, true)
	if err != nil {
		return err
	}
	defer root.Close()
	probe, probeName, err := createRequestArchiveLocalTemp(root, ".request-archive-probe-")
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(probeName) }()
	if err := probe.Close(); err != nil {
		return err
	}
	return root.Remove(probeName)
}

type requestArchiveResponseLimitTransport struct {
	base  http.RoundTripper
	limit int64
}

func (transport requestArchiveResponseLimitTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.base.RoundTrip(request)
	if err != nil || response == nil || response.Body == nil {
		return response, err
	}
	response.Body = &requestArchiveLimitedReadCloser{ReadCloser: response.Body, remaining: transport.limit}
	return response, nil
}

type requestArchiveLimitedReadCloser struct {
	io.ReadCloser
	remaining int64
}

func (reader *requestArchiveLimitedReadCloser) Read(buffer []byte) (int, error) {
	if reader.remaining <= 0 {
		return 0, errRequestArchiveResponseTooLarge
	}
	if int64(len(buffer)) > reader.remaining {
		buffer = buffer[:reader.remaining]
	}
	count, err := reader.ReadCloser.Read(buffer)
	reader.remaining -= int64(count)
	return count, err
}

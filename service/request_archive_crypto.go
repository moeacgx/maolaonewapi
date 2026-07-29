package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	requestArchiveLegacyCipherVersion = "ra1"
	requestArchiveV2CipherVersion     = "ra2"
	requestArchiveCipherVersion       = "ra3"
	requestArchivePlaintextVersion    = "plain_ra1"
	requestArchivePlaintextPrefix     = requestArchivePlaintextVersion + ":"
	requestArchiveSecretCipherVersion = "ras1"
	requestArchiveAccessKeyPurpose    = "access_key"
	requestArchiveSecretKeyPurpose    = "secret_key"
	requestArchiveChunkSize           = 1024 * 1024
	requestArchiveNoncePrefixSize     = 8
	requestArchiveGCMTagSize          = 16
)

func RequestArchiveCryptoReady() bool {
	return strings.TrimSpace(os.Getenv("CRYPTO_SECRET")) != "" && strings.TrimSpace(common.CryptoSecret) != ""
}

func requestArchiveAEAD() (cipher.AEAD, error) {
	if !RequestArchiveCryptoReady() {
		return nil, errors.New("必须显式配置稳定的 CRYPTO_SECRET")
	}
	digest := sha256.Sum256([]byte("new-api:request-archive:v1:" + common.CryptoSecret))
	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func requestArchiveJobAEAD(job *model.RequestArchiveJob) (cipher.AEAD, error) {
	key, err := requestArchiveJobDerivedKey(job, "new-api:request-archive:v3:aes-256-gcm")
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func requestArchiveJobDerivedKey(job *model.RequestArchiveJob, info string) ([]byte, error) {
	if !RequestArchiveCryptoReady() {
		return nil, errors.New("必须显式配置稳定的 CRYPTO_SECRET")
	}
	if job == nil || strings.TrimSpace(job.ArchiveId) == "" {
		return nil, errors.New("请求归档任务身份无效")
	}
	return hkdf.Key(
		sha256.New,
		[]byte(common.CryptoSecret),
		[]byte(job.ArchiveId),
		info,
		32,
	)
}

func requestArchiveSecretAEAD() (cipher.AEAD, error) {
	if !RequestArchiveCryptoReady() {
		return nil, errors.New("必须显式配置稳定的 CRYPTO_SECRET")
	}
	digest := sha256.Sum256([]byte("new-api:request-archive-secret:v1:" + common.CryptoSecret))
	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func requestArchiveLegacyAAD(sha256Value string, byteSize int64, targetID string, configVersion int64) []byte {
	return []byte(strings.Join([]string{
		"new-api:request-archive:v1",
		strings.ToLower(strings.TrimSpace(sha256Value)),
		strconv.FormatInt(byteSize, 10),
		strings.TrimSpace(targetID),
		strconv.FormatInt(configVersion, 10),
	}, ":"))
}

// EncryptRequestArchivePayload 仅用于读取 ra1 兼容性测试和旧任务构造。
// 新任务必须使用 EncryptRequestArchiveJobPayload，把任务身份和元数据一并绑定。
func EncryptRequestArchivePayload(plaintext []byte, sha256Value string, byteSize int64, targetID string, configVersion int64) (string, error) {
	aead, err := requestArchiveAEAD()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, requestArchiveLegacyAAD(sha256Value, byteSize, targetID, configVersion))
	return strings.Join([]string{
		requestArchiveLegacyCipherVersion,
		base64.RawURLEncoding.EncodeToString(nonce),
		base64.RawURLEncoding.EncodeToString(ciphertext),
	}, "."), nil
}

func requestArchiveJobAAD(job *model.RequestArchiveJob, version string) ([]byte, error) {
	if job == nil || strings.TrimSpace(job.ArchiveId) == "" || job.ByteSize < 0 ||
		job.ByteSize > model.RequestArchiveMaximumBodyBytes || strings.TrimSpace(job.TargetId) == "" ||
		job.ConfigVersion < 1 {
		return nil, errors.New("请求归档任务绑定无效")
	}
	if version != requestArchiveV2CipherVersion && version != requestArchiveCipherVersion {
		return nil, errors.New("请求归档密文版本无效")
	}
	fields := []string{
		"new-api:request-archive:v" + strings.TrimPrefix(version, "ra"),
		job.ArchiveId,
		strings.ToLower(strings.TrimSpace(job.SHA256)),
		strconv.FormatInt(job.ByteSize, 10),
		job.TargetId,
		strconv.FormatInt(job.ConfigVersion, 10),
		job.ContentType,
		job.Method,
		job.Path,
		job.RequestId,
		strconv.Itoa(job.UserId),
		job.Username,
		job.UserEmail,
		strconv.Itoa(job.TokenId),
		job.TokenName,
		strconv.Itoa(job.GroupId),
		job.GroupName,
		strconv.FormatInt(job.CreatedAt, 10),
		strconv.FormatInt(job.ExpiresAt, 10),
	}
	var builder strings.Builder
	for _, field := range fields {
		builder.WriteString(strconv.Itoa(len(field)))
		builder.WriteByte(':')
		builder.WriteString(field)
	}
	return []byte(builder.String()), nil
}

func requestArchiveChunkAAD(base []byte, index, count int) []byte {
	result := make([]byte, len(base)+8)
	copy(result, base)
	binary.BigEndian.PutUint32(result[len(base):], uint32(index))
	binary.BigEndian.PutUint32(result[len(base)+4:], uint32(count))
	return result
}

func requestArchiveChunkNonce(prefix []byte, index int) ([]byte, error) {
	if len(prefix) != requestArchiveNoncePrefixSize || index < 0 {
		return nil, errors.New("请求归档分片 nonce 无效")
	}
	nonce := make([]byte, requestArchiveNoncePrefixSize+4)
	copy(nonce, prefix)
	// 旧 ra2 与新 ra3 都使用 64 位随机前缀加 32 位分片序号。
	// ra3 还会按 archive_id 派生任务独立密钥，避免跨任务复用同一 GCM 密钥域。
	binary.BigEndian.PutUint32(nonce[requestArchiveNoncePrefixSize:], uint32(index))
	return nonce, nil
}

func requestArchiveChunkCount(byteSize int64) (int, error) {
	if byteSize < 0 || byteSize > model.RequestArchiveMaximumBodyBytes {
		return 0, errors.New("请求归档正文长度无效")
	}
	if byteSize == 0 {
		return 1, nil
	}
	return int((byteSize + requestArchiveChunkSize - 1) / requestArchiveChunkSize), nil
}

// requestArchiveChunkedEnvelopeSize 精确计算 ra2/ra3 分片信封长度。每个分片都独立做
// RawURL Base64，因此不能把所有密文字节合并后只计算一次编码长度。
func requestArchiveChunkedEnvelopeSize(byteSize int64, overhead int) (int, error) {
	chunkCount, err := requestArchiveChunkCount(byteSize)
	if err != nil {
		return 0, err
	}
	if overhead < 1 {
		return 0, errors.New("请求归档密文开销无效")
	}
	headerSize := len(requestArchiveCipherVersion) + 1 + len(strconv.Itoa(requestArchiveChunkSize)) +
		1 + len(strconv.Itoa(chunkCount)) + 1 + base64.RawURLEncoding.EncodedLen(requestArchiveNoncePrefixSize)
	total := int64(headerSize + chunkCount)
	remaining := byteSize
	for index := 0; index < chunkCount; index++ {
		chunkBytes := remaining
		if chunkBytes > requestArchiveChunkSize {
			chunkBytes = requestArchiveChunkSize
		}
		if chunkBytes < 0 || chunkBytes > int64(^uint(0)>>1)-int64(overhead) {
			return 0, errors.New("请求归档密文长度无效")
		}
		total += int64(base64.RawURLEncoding.EncodedLen(int(chunkBytes) + overhead))
		remaining -= chunkBytes
	}
	if remaining != 0 || total < 1 || total > int64(^uint(0)>>1) {
		return 0, errors.New("请求归档密文长度无效")
	}
	return int(total), nil
}

// EncryptRequestArchiveJobPayload 使用固定大小分片构造 ra3 信封。最终 TEXT
// 信封仍可跨数据库保存，但加密过程只额外保留一个分片，避免同时持有整块
// GCM 密文、Base64 中间值和最终字符串。
func EncryptRequestArchiveJobPayload(plaintext []byte, job *model.RequestArchiveJob) (string, error) {
	return encryptRequestArchiveChunkedPayload(plaintext, job, requestArchiveCipherVersion)
}

// encryptRequestArchiveV2JobPayload 仅用于验证已发布 ra2 任务的只读兼容性。
func encryptRequestArchiveV2JobPayload(plaintext []byte, job *model.RequestArchiveJob) (string, error) {
	return encryptRequestArchiveChunkedPayload(plaintext, job, requestArchiveV2CipherVersion)
}

func encryptRequestArchiveChunkedPayload(plaintext []byte, job *model.RequestArchiveJob, version string) (string, error) {
	if job == nil || int64(len(plaintext)) != job.ByteSize {
		return "", errors.New("请求归档正文长度与任务不一致")
	}
	digestText, err := requestArchivePlaintextDigest(job, version, plaintext)
	if err != nil {
		return "", err
	}
	if !hmac.Equal(
		[]byte(strings.ToLower(digestText)),
		[]byte(strings.ToLower(strings.TrimSpace(job.SHA256))),
	) {
		return "", errors.New("请求归档正文哈希与任务不一致")
	}
	baseAAD, err := requestArchiveJobAAD(job, version)
	if err != nil {
		return "", err
	}
	var aead cipher.AEAD
	if version == requestArchiveV2CipherVersion {
		aead, err = requestArchiveAEAD()
	} else if version == requestArchiveCipherVersion {
		aead, err = requestArchiveJobAEAD(job)
	} else {
		return "", errors.New("请求归档密文版本无效")
	}
	if err != nil {
		return "", err
	}
	chunkCount, err := requestArchiveChunkCount(job.ByteSize)
	if err != nil {
		return "", err
	}
	prefix := make([]byte, requestArchiveNoncePrefixSize)
	if _, err := io.ReadFull(rand.Reader, prefix); err != nil {
		return "", err
	}

	var builder strings.Builder
	estimated, err := requestArchiveChunkedEnvelopeSize(job.ByteSize, aead.Overhead())
	if err != nil {
		return "", err
	}
	builder.Grow(estimated)
	builder.WriteString(version)
	builder.WriteByte('.')
	builder.WriteString(strconv.Itoa(requestArchiveChunkSize))
	builder.WriteByte('.')
	builder.WriteString(strconv.Itoa(chunkCount))
	builder.WriteByte('.')
	builder.WriteString(base64.RawURLEncoding.EncodeToString(prefix))

	chunkBuffer := make([]byte, 0, requestArchiveChunkSize+aead.Overhead())
	for index := 0; index < chunkCount; index++ {
		start := index * requestArchiveChunkSize
		end := start + requestArchiveChunkSize
		if end > len(plaintext) {
			end = len(plaintext)
		}
		nonce, err := requestArchiveChunkNonce(prefix, index)
		if err != nil {
			return "", err
		}
		chunkBuffer = aead.Seal(chunkBuffer[:0], nonce, plaintext[start:end], requestArchiveChunkAAD(baseAAD, index, chunkCount))
		builder.WriteByte('.')
		encoder := base64.NewEncoder(base64.RawURLEncoding, &builder)
		if _, err := encoder.Write(chunkBuffer); err != nil {
			return "", err
		}
		if err := encoder.Close(); err != nil {
			return "", err
		}
	}
	return builder.String(), nil
}

func parseRequestArchiveChunkedEnvelope(job *model.RequestArchiveJob, version string) (cipher.AEAD, []byte, []byte, []string, error) {
	expectedCount, err := requestArchiveChunkCount(job.ByteSize)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// 分段数量由已经受上限约束的正文长度决定。SplitN 可避免数据库中的
	// 篡改密文通过大量分隔符制造无界字符串切片。
	parts := strings.SplitN(string(job.RequestCiphertext), ".", 4+expectedCount)
	if len(parts) < 5 || parts[0] != version {
		return nil, nil, nil, nil, errors.New("请求归档密文版本无效")
	}
	chunkSize, err := strconv.Atoi(parts[1])
	if err != nil || chunkSize != requestArchiveChunkSize {
		return nil, nil, nil, nil, errors.New("请求归档密文分片大小无效")
	}
	chunkCount, err := strconv.Atoi(parts[2])
	if err != nil || chunkCount < 1 || len(parts) != 4+chunkCount {
		return nil, nil, nil, nil, errors.New("请求归档密文分片数量无效")
	}
	if expectedCount != chunkCount {
		return nil, nil, nil, nil, errors.New("请求归档密文分片数量与正文长度不一致")
	}
	prefix, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(prefix) != requestArchiveNoncePrefixSize {
		return nil, nil, nil, nil, errors.New("请求归档密文 nonce 前缀无效")
	}
	baseAAD, err := requestArchiveJobAAD(job, version)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var aead cipher.AEAD
	if version == requestArchiveV2CipherVersion {
		aead, err = requestArchiveAEAD()
	} else if version == requestArchiveCipherVersion {
		aead, err = requestArchiveJobAEAD(job)
	} else {
		return nil, nil, nil, nil, errors.New("请求归档密文版本无效")
	}
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return aead, prefix, baseAAD, parts[4:], nil
}

func streamRequestArchiveChunkedPlaintext(job *model.RequestArchiveJob, version string, consume func([]byte) error) error {
	aead, prefix, baseAAD, chunks, err := parseRequestArchiveChunkedEnvelope(job, version)
	if err != nil {
		return err
	}
	for index, encoded := range chunks {
		ciphertext, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return errors.New("请求归档密文分片无效")
		}
		nonce, err := requestArchiveChunkNonce(prefix, index)
		if err != nil {
			return err
		}
		plaintext, err := aead.Open(ciphertext[:0], nonce, ciphertext, requestArchiveChunkAAD(baseAAD, index, len(chunks)))
		if err != nil {
			return fmt.Errorf("请求归档密文分片解密失败: %w", err)
		}
		if err := consume(plaintext); err != nil {
			return err
		}
	}
	return nil
}

func requestArchivePlaintextDigestHash(job *model.RequestArchiveJob, version string) (hash.Hash, error) {
	if version == requestArchivePlaintextVersion || version == requestArchiveLegacyCipherVersion || version == requestArchiveV2CipherVersion {
		return sha256.New(), nil
	}
	if version != requestArchiveCipherVersion {
		return nil, errors.New("请求归档密文版本无效")
	}
	key, err := requestArchiveJobDerivedKey(job, "new-api:request-archive:v3:payload-hmac-sha256")
	if err != nil {
		return nil, err
	}
	return hmac.New(sha256.New, key), nil
}

func requestArchivePlaintextDigest(job *model.RequestArchiveJob, version string, plaintext []byte) (string, error) {
	digest, err := requestArchivePlaintextDigestHash(job, version)
	if err != nil {
		return "", err
	}
	_, _ = digest.Write(plaintext)
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

func validateRequestArchivePlaintext(job *model.RequestArchiveJob, version string, byteSize int64, digest hash.Hash) error {
	actual := fmt.Sprintf("%x", digest.Sum(nil))
	if byteSize != job.ByteSize || !hmac.Equal(
		[]byte(strings.ToLower(actual)),
		[]byte(strings.ToLower(strings.TrimSpace(job.SHA256))),
	) {
		return errors.New("请求归档密文校验失败")
	}
	return nil
}

func decryptLegacyRequestArchivePayload(job *model.RequestArchiveJob) ([]byte, error) {
	parts := strings.SplitN(string(job.RequestCiphertext), ".", 4)
	if len(parts) != 3 || parts[0] != requestArchiveLegacyCipherVersion {
		return nil, errors.New("请求归档密文版本无效")
	}
	aead, err := requestArchiveAEAD()
	if err != nil {
		return nil, err
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(nonce) != aead.NonceSize() {
		return nil, errors.New("请求归档密文 nonce 无效")
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("请求归档密文正文无效")
	}
	// 旧 ra1 任务使用单块信封。原地解密避免同时保留一份完整密文和
	// 完整明文，确保最大兼容正文仍受 Worker 的 384 MiB 预算约束。
	plaintext, err := aead.Open(ciphertext[:0], nonce, ciphertext, requestArchiveLegacyAAD(job.SHA256, job.ByteSize, job.TargetId, job.ConfigVersion))
	if err != nil {
		return nil, fmt.Errorf("请求归档密文解密失败: %w", err)
	}
	return plaintext, nil
}

func DecryptRequestArchivePayload(job *model.RequestArchiveJob) ([]byte, error) {
	if job == nil || job.RequestCiphertext == "" {
		return nil, errors.New("请求归档密文为空")
	}
	if _, err := requestArchiveChunkCount(job.ByteSize); err != nil {
		return nil, err
	}
	stored := string(job.RequestCiphertext)
	if strings.HasPrefix(stored, requestArchivePlaintextPrefix) {
		plaintext := []byte(strings.TrimPrefix(stored, requestArchivePlaintextPrefix))
		digest, digestErr := requestArchivePlaintextDigest(job, requestArchivePlaintextVersion, plaintext)
		if digestErr != nil || int64(len(plaintext)) != job.ByteSize || !hmac.Equal([]byte(strings.ToLower(digest)), []byte(strings.ToLower(strings.TrimSpace(job.SHA256)))) {
			return nil, errors.New("请求归档明文校验失败")
		}
		return plaintext, nil
	}
	version := strings.SplitN(stored, ".", 2)[0]
	if version == requestArchiveLegacyCipherVersion {
		plaintext, err := decryptLegacyRequestArchivePayload(job)
		if err != nil {
			return nil, err
		}
		digest, digestErr := requestArchivePlaintextDigestHash(job, version)
		if digestErr != nil {
			return nil, digestErr
		}
		_, _ = digest.Write(plaintext)
		if err := validateRequestArchivePlaintext(job, version, int64(len(plaintext)), digest); err != nil {
			return nil, err
		}
		return plaintext, nil
	}
	if version != requestArchiveV2CipherVersion && version != requestArchiveCipherVersion {
		return nil, errors.New("请求归档密文版本无效")
	}
	digest, err := requestArchivePlaintextDigestHash(job, version)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, 0, job.ByteSize)
	var byteSize int64
	err = streamRequestArchiveChunkedPlaintext(job, version, func(chunk []byte) error {
		byteSize += int64(len(chunk))
		_, _ = digest.Write(chunk)
		plaintext = append(plaintext, chunk...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := validateRequestArchivePlaintext(job, version, byteSize, digest); err != nil {
		return nil, err
	}
	return plaintext, nil
}

// ValidateRequestArchivePayload 对 ra2/ra3 分片逐片解密并计算哈希，不在
// Worker 中重新组装整块明文。旧信封仅保留兼容读取。
func ValidateRequestArchivePayload(job *model.RequestArchiveJob) error {
	if job == nil || job.RequestCiphertext == "" {
		return errors.New("请求归档密文为空")
	}
	stored := string(job.RequestCiphertext)
	if strings.HasPrefix(stored, requestArchivePlaintextPrefix) {
		_, err := DecryptRequestArchivePayload(job)
		return err
	}
	version := strings.SplitN(stored, ".", 2)[0]
	if version == requestArchiveLegacyCipherVersion {
		_, err := DecryptRequestArchivePayload(job)
		return err
	}
	if version != requestArchiveV2CipherVersion && version != requestArchiveCipherVersion {
		return errors.New("请求归档密文版本无效")
	}
	digest, err := requestArchivePlaintextDigestHash(job, version)
	if err != nil {
		return err
	}
	var byteSize int64
	if err := streamRequestArchiveChunkedPlaintext(job, version, func(chunk []byte) error {
		byteSize += int64(len(chunk))
		_, _ = digest.Write(chunk)
		return nil
	}); err != nil {
		return err
	}
	return validateRequestArchivePlaintext(job, version, byteSize, digest)
}

func requestArchiveSecretAAD(targetID, purpose string) ([]byte, error) {
	targetID = strings.TrimSpace(targetID)
	purpose = strings.TrimSpace(purpose)
	if targetID == "" || (purpose != requestArchiveAccessKeyPurpose && purpose != requestArchiveSecretKeyPurpose) {
		return nil, errors.New("请求归档存储密钥绑定无效")
	}
	return []byte(strings.Join([]string{
		"new-api:request-archive-secret:v1",
		targetID,
		purpose,
	}, ":")), nil
}

// EncryptRequestArchiveSecret 使用与 Guard 节点令牌和请求正文都不同的密钥域，
// 并把密文绑定到目标 ID 与凭据用途，防止数据库中的有效密文被跨字段调换。
func EncryptRequestArchiveSecret(plaintext, targetID, purpose string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	aad, err := requestArchiveSecretAAD(targetID, purpose)
	if err != nil {
		return "", err
	}
	aead, err := requestArchiveSecretAEAD()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := aead.Seal(nil, nonce, []byte(plaintext), aad)
	return strings.Join([]string{
		requestArchiveSecretCipherVersion,
		base64.RawURLEncoding.EncodeToString(nonce),
		base64.RawURLEncoding.EncodeToString(ciphertext),
	}, "."), nil
}

func DecryptRequestArchiveSecret(envelope, targetID, purpose string) (string, error) {
	if envelope == "" {
		return "", nil
	}
	parts := strings.SplitN(envelope, ".", 4)
	if len(parts) != 3 || parts[0] != requestArchiveSecretCipherVersion {
		return "", errors.New("请求归档存储密钥版本无效")
	}
	aad, err := requestArchiveSecretAAD(targetID, purpose)
	if err != nil {
		return "", err
	}
	aead, err := requestArchiveSecretAEAD()
	if err != nil {
		return "", err
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(nonce) != aead.NonceSize() {
		return "", errors.New("请求归档存储密钥 nonce 无效")
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", errors.New("请求归档存储密钥正文无效")
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return "", fmt.Errorf("请求归档存储密钥解密失败: %w", err)
	}
	return string(plaintext), nil
}

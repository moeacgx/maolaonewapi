package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	promptAuditContextEncryptedPrefix  = "enc_context_v1:"
	promptAuditContextPlaintextPrefix  = "plain_context_v1:"
	promptAuditKeywordsEncryptedPrefix = "enc_keywords_v1:"
	promptAuditKeywordsPlaintextPrefix = "plain_keywords_v1:"
)

// StorePromptAuditMatchedKeywords 保存屏蔽词事件实际命中的关键词。数据库字段不直接
// 序列化；Root 列表和详情按需解密。有稳定密钥时使用 AES-GCM，无密钥时沿用
// Root-only 明文兼容模式。
func StorePromptAuditMatchedKeywords(keywords []string) (string, error) {
	if len(keywords) == 0 {
		return "", nil
	}
	data, err := common.Marshal(keywords)
	if err != nil {
		return "", err
	}
	if PromptAuditCryptoReady() {
		ciphertext, err := EncryptPromptAuditSecret(string(data))
		if err != nil {
			return "", err
		}
		return promptAuditKeywordsEncryptedPrefix + ciphertext, nil
	}
	return promptAuditKeywordsPlaintextPrefix + string(data), nil
}

func LoadPromptAuditMatchedKeywords(stored string) ([]string, error) {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return []string{}, nil
	}
	plain := stored
	switch {
	case strings.HasPrefix(stored, promptAuditKeywordsEncryptedPrefix):
		var err error
		plain, err = DecryptPromptAuditSecret(strings.TrimPrefix(stored, promptAuditKeywordsEncryptedPrefix))
		if err != nil {
			return nil, err
		}
	case strings.HasPrefix(stored, promptAuditKeywordsPlaintextPrefix):
		plain = strings.TrimPrefix(stored, promptAuditKeywordsPlaintextPrefix)
	}
	var keywords []string
	if err := common.UnmarshalJsonStr(plain, &keywords); err != nil {
		return nil, err
	}
	if keywords == nil {
		keywords = []string{}
	}
	return keywords, nil
}

// promptAuditContextSegmentsForPersistence 只保留详情重建所需的角色和偏移元数据。
// 分段正文不得绕过 FullPrompt 的持久化上限重复落库。
func promptAuditContextSegmentsForPersistence(segments []PromptAuditContextSegment) []PromptAuditContextSegment {
	bounded := make([]PromptAuditContextSegment, 0, len(segments))
	for _, segment := range segments {
		start, end := segment.Start, segment.End
		if start < 0 {
			start = 0
		}
		if end < start || start >= PromptAuditMaxFullPromptRunes {
			continue
		}
		if end > PromptAuditMaxFullPromptRunes {
			end = PromptAuditMaxFullPromptRunes
		}
		bounded = append(bounded, PromptAuditContextSegment{
			Role: segment.Role, Kind: segment.Kind, Start: start, End: end,
		})
	}
	return bounded
}

// StorePromptAuditContextSegments 保存上下文角色和偏移元数据。详情正文从受限的
// FullPrompt 临时重建；配置稳定密钥时元数据使用 AES-GCM，未配置密钥时保留
// Root-only 明文兼容模式。
func StorePromptAuditContextSegments(segments []PromptAuditContextSegment) (string, error) {
	if len(segments) == 0 {
		return "", nil
	}
	data, err := common.Marshal(promptAuditContextSegmentsForPersistence(segments))
	if err != nil {
		return "", err
	}
	if PromptAuditCryptoReady() {
		ciphertext, err := EncryptPromptAuditSecret(string(data))
		if err != nil {
			return "", err
		}
		return promptAuditContextEncryptedPrefix + ciphertext, nil
	}
	return promptAuditContextPlaintextPrefix + string(data), nil
}

// LoadPromptAuditContextSegments 解密并读取上下文分段，同时兼容历史版本直接
// 保存的 JSON 数组，便于平滑升级已有审计记录。
func LoadPromptAuditContextSegments(stored string) ([]PromptAuditContextSegment, error) {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return []PromptAuditContextSegment{}, nil
	}
	plain := stored
	switch {
	case strings.HasPrefix(stored, promptAuditContextEncryptedPrefix):
		var err error
		plain, err = DecryptPromptAuditSecret(strings.TrimPrefix(stored, promptAuditContextEncryptedPrefix))
		if err != nil {
			return nil, err
		}
	case strings.HasPrefix(stored, promptAuditContextPlaintextPrefix):
		plain = strings.TrimPrefix(stored, promptAuditContextPlaintextPrefix)
	}
	var segments []PromptAuditContextSegment
	if err := common.UnmarshalJsonStr(plain, &segments); err != nil {
		return nil, err
	}
	if segments == nil {
		segments = []PromptAuditContextSegment{}
	}
	return segments, nil
}

// StorePromptAuditSecret 保存审核正文。配置了稳定密钥时使用 AES-GCM；未配置
// 密钥时保留明确的明文模式，确保 Root 审计不会退化成只有哈希的空记录。
func StorePromptAuditSecret(plaintext string) (string, string, error) {
	if plaintext == "" {
		return "", "", nil
	}
	if PromptAuditCryptoReady() {
		ciphertext, err := EncryptPromptAuditSecret(plaintext)
		if err != nil {
			return "", "", err
		}
		return ciphertext, model.PromptAuditCipherKindPrompt, nil
	}
	return plaintext, model.PromptAuditCipherKindPlaintext, nil
}

func LoadPromptAuditSecret(stored, cipherKind string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(cipherKind)) {
	case model.PromptAuditCipherKindPlaintext, "plaintext":
		return stored, nil
	case model.PromptAuditCipherKindJobPayload:
		if strings.HasPrefix(stored, promptAuditPlaintextPrefix) {
			return strings.TrimPrefix(stored, promptAuditPlaintextPrefix), nil
		}
	}
	return DecryptPromptAuditSecret(stored)
}

const promptAuditCipherVersion = "v1"

func PromptAuditCryptoReady() bool {
	return strings.TrimSpace(os.Getenv("CRYPTO_SECRET")) != "" && strings.TrimSpace(common.CryptoSecret) != ""
}

func promptAuditAEAD() (cipher.AEAD, error) {
	if !PromptAuditCryptoReady() {
		return nil, errors.New("必须显式配置稳定的 CRYPTO_SECRET")
	}
	digest := sha256.Sum256([]byte("new-api:prompt-security-audit:v1:" + common.CryptoSecret))
	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func EncryptPromptAuditSecret(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	aead, err := promptAuditAEAD()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := aead.Seal(nil, nonce, []byte(plaintext), []byte(promptAuditCipherVersion))
	return strings.Join([]string{
		promptAuditCipherVersion,
		base64.RawURLEncoding.EncodeToString(nonce),
		base64.RawURLEncoding.EncodeToString(ciphertext),
	}, "."), nil
}

func DecryptPromptAuditSecret(envelope string) (string, error) {
	if envelope == "" {
		return "", nil
	}
	parts := strings.Split(envelope, ".")
	if len(parts) != 3 || parts[0] != promptAuditCipherVersion {
		return "", errors.New("提示词审计密文版本无效")
	}
	aead, err := promptAuditAEAD()
	if err != nil {
		return "", err
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(nonce) != aead.NonceSize() {
		return "", errors.New("提示词审计密文 nonce 无效")
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", errors.New("提示词审计密文正文无效")
	}
	plain, err := aead.Open(nil, nonce, ciphertext, []byte(promptAuditCipherVersion))
	if err != nil {
		return "", fmt.Errorf("提示词审计密文解密失败: %w", err)
	}
	return string(plain), nil
}

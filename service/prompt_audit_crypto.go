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
)

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

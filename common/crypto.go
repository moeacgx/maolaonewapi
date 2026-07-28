package common

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func GenerateHMACWithKey(key []byte, data string) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func GenerateHMAC(data string) string {
	return GenerateHMACWithKey([]byte(CryptoSecret), data)
}

func ValidateHMACWithKey(key []byte, data string, signature string) bool {
	provided, err := hex.DecodeString(strings.TrimSpace(signature))
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	expected, err := hex.DecodeString(GenerateHMACWithKey(key, data))
	if err != nil {
		return false
	}
	return hmac.Equal(provided, expected)
}

func ValidateHMAC(data string, signature string) bool {
	return ValidateHMACWithKey([]byte(CryptoSecret), data, signature)
}

func Password2Hash(password string) (string, error) {
	passwordBytes := []byte(password)
	hashedPassword, err := bcrypt.GenerateFromPassword(passwordBytes, bcrypt.DefaultCost)
	return string(hashedPassword), err
}

func ValidatePasswordAndHash(password string, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

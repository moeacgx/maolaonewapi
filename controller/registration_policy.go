package controller

import (
	"errors"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"gorm.io/gorm"
)

var errNewUserRegistrationDisabled = errors.New("new user registration is disabled")

const invitationRegistrationHMACDomain = "invitation-registration:v1:"

type registrationInvitationCredential struct {
	AffCode   string
	Signature string
}

func invitationRegistrationSigningReady() bool {
	hasStableEnvironmentSecret := strings.TrimSpace(os.Getenv("CRYPTO_SECRET")) != "" ||
		strings.TrimSpace(os.Getenv("SESSION_SECRET")) != ""
	return hasStableEnvironmentSecret && strings.TrimSpace(common.CryptoSecret) != ""
}

func generateInvitationRegistrationSignature(affCode string) (string, error) {
	affCode = strings.TrimSpace(affCode)
	if affCode == "" {
		return "", errors.New("邀请码为空")
	}
	if !invitationRegistrationSigningReady() {
		return "", errors.New("邀请注册需要显式配置稳定的 CRYPTO_SECRET 或 SESSION_SECRET")
	}
	return common.GenerateHMAC(invitationRegistrationHMACDomain + affCode), nil
}

func validateInvitationRegistrationSignature(credential registrationInvitationCredential) bool {
	affCode := strings.TrimSpace(credential.AffCode)
	signature := strings.TrimSpace(credential.Signature)
	if affCode == "" || signature == "" || !invitationRegistrationSigningReady() {
		return false
	}
	return common.ValidateHMAC(invitationRegistrationHMACDomain+affCode, signature)
}

// resolveNewUserRegistrationInviter 统一决定新用户是否可以注册，并且只返回已验证的邀请码归属用户。
func resolveNewUserRegistrationInviter(affCode string, signature string) (int, error) {
	credential := registrationInvitationCredential{
		AffCode:   strings.TrimSpace(affCode),
		Signature: strings.TrimSpace(signature),
	}

	if common.RegisterEnabled {
		if credential.AffCode == "" {
			return 0, nil
		}
		inviterId, err := model.GetUserIdByAffCode(credential.AffCode)
		if err != nil {
			// 公开注册开启时保持既有行为：无效邀请码不影响普通注册。
			return 0, nil
		}
		return inviterId, nil
	}

	if !common.InvitationRegisterEnabled || !validateInvitationRegistrationSignature(credential) {
		return 0, errNewUserRegistrationDisabled
	}
	inviterId, err := model.GetActiveInviterIdByAffCode(credential.AffCode)
	if err != nil {
		// 关闭公开注册时不向客户端区分缺失、无效和风控封禁的邀请码。
		return 0, errNewUserRegistrationDisabled
	}
	return inviterId, nil
}

func revalidateNewUserRegistrationInviterWithDB(
	db *gorm.DB,
	credential registrationInvitationCredential,
	expectedInviterId int,
) error {
	if common.RegisterEnabled {
		return nil
	}
	if !common.InvitationRegisterEnabled || expectedInviterId <= 0 ||
		!validateInvitationRegistrationSignature(credential) {
		return errNewUserRegistrationDisabled
	}
	inviterId, err := model.GetActiveInviterIdByAffCodeForUpdateWithDB(db, credential.AffCode)
	if err != nil || inviterId != expectedInviterId {
		return errNewUserRegistrationDisabled
	}
	return nil
}

func isNewUserRegistrationDisabled(err error) bool {
	return errors.Is(err, errNewUserRegistrationDisabled)
}

// insertNewUserWithRegistrationPolicy 在同一事务内复核邀请资格并创建用户，
// 避免校验通过后、写入用户前邀请人被禁用或邀请码被风控封禁。
func insertNewUserWithRegistrationPolicy(
	user *model.User,
	credential registrationInvitationCredential,
) error {
	if user == nil {
		return errors.New("用户为空")
	}
	inviterId, err := resolveNewUserRegistrationInviter(credential.AffCode, credential.Signature)
	if err != nil {
		return err
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := revalidateNewUserRegistrationInviterWithDB(tx, credential, inviterId); err != nil {
			return err
		}
		return user.InsertWithTx(tx, inviterId)
	}); err != nil {
		return err
	}
	user.FinalizeOAuthUserCreation(inviterId)
	return nil
}

func insertOAuthNewUserWithRegistrationPolicy(user *model.User, session sessions.Session) error {
	return insertNewUserWithRegistrationPolicy(user, registrationInvitationCredentialFromSession(session))
}

func registrationInvitationCredentialFromSession(session sessions.Session) registrationInvitationCredential {
	if session == nil {
		return registrationInvitationCredential{}
	}
	affCode, _ := session.Get("aff").(string)
	signature, _ := session.Get("invite").(string)
	return registrationInvitationCredential{
		AffCode:   strings.TrimSpace(affCode),
		Signature: strings.TrimSpace(signature),
	}
}

func setOAuthRegistrationInvitationCredential(session sessions.Session, affCode string, signature string) {
	if session == nil {
		return
	}
	affCode = strings.TrimSpace(affCode)
	signature = strings.TrimSpace(signature)
	if affCode == "" {
		session.Delete("aff")
	} else {
		session.Set("aff", affCode)
	}
	if signature == "" {
		session.Delete("invite")
	} else {
		session.Set("invite", signature)
	}
}

func resolveOAuthRegistrationInviter(session sessions.Session) (int, error) {
	credential := registrationInvitationCredentialFromSession(session)
	return resolveNewUserRegistrationInviter(credential.AffCode, credential.Signature)
}

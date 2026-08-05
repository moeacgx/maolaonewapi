package controller

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-contrib/sessions"
	"gorm.io/gorm"
)

var errNewUserRegistrationDisabled = errors.New("new user registration is disabled")

type registrationInvitationCredential struct {
	AffCode string
}

// resolveNewUserRegistrationInviter 统一决定新用户是否可以注册，并且只返回已验证的邀请码归属用户。
func resolveNewUserRegistrationInviter(affCode string) (int, error) {
	credential := registrationInvitationCredential{
		AffCode: strings.TrimSpace(affCode),
	}

	if common.RegisterEnabled {
		if credential.AffCode == "" {
			return 0, nil
		}
		inviterId, err := model.GetActiveInviterIdByAffCode(credential.AffCode)
		if err != nil || !model.AffiliateUserCanInvite(inviterId, setting.GetAffiliateSetting()) {
			// 公开注册开启时保持既有行为：无效邀请码不影响普通注册。
			return 0, nil
		}
		return inviterId, nil
	}

	if !common.InvitationRegisterEnabled || credential.AffCode == "" {
		return 0, errNewUserRegistrationDisabled
	}
	inviterId, err := model.GetActiveInviterIdByAffCode(credential.AffCode)
	if err != nil || !model.AffiliateUserCanInvite(inviterId, setting.GetAffiliateSetting()) {
		// 关闭公开注册时不向客户端区分缺失、无效和风控封禁的邀请码。
		return 0, errNewUserRegistrationDisabled
	}
	return inviterId, nil
}

func revalidateNewUserRegistrationInviterWithDB(
	db *gorm.DB,
	credential registrationInvitationCredential,
	expectedInviterId int,
) (int, error) {
	credential.AffCode = strings.TrimSpace(credential.AffCode)
	if expectedInviterId <= 0 || credential.AffCode == "" {
		if common.RegisterEnabled {
			return 0, nil
		}
		return 0, errNewUserRegistrationDisabled
	}
	if !common.RegisterEnabled && !common.InvitationRegisterEnabled {
		return 0, errNewUserRegistrationDisabled
	}
	inviterId, err := model.GetActiveInviterIdByAffCodeForUpdateWithDB(db, credential.AffCode)
	if err != nil || inviterId != expectedInviterId ||
		!model.AffiliateUserCanInviteForUpdateWithDB(db, inviterId, setting.GetAffiliateSetting()) {
		if common.RegisterEnabled {
			// 公开注册仍可继续，但失效或已撤销权限的邀请码不得建立邀请关系。
			return 0, nil
		}
		return 0, errNewUserRegistrationDisabled
	}
	return inviterId, nil
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
	inviterId, err := resolveNewUserRegistrationInviter(credential.AffCode)
	if err != nil {
		return err
	}
	validatedInviterId := 0
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var revalidateErr error
		validatedInviterId, revalidateErr = revalidateNewUserRegistrationInviterWithDB(tx, credential, inviterId)
		if revalidateErr != nil {
			return revalidateErr
		}
		return user.InsertWithTx(tx, validatedInviterId)
	}); err != nil {
		return err
	}
	user.FinalizeOAuthUserCreation(validatedInviterId)
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
	return registrationInvitationCredential{
		AffCode: strings.TrimSpace(affCode),
	}
}

func setOAuthRegistrationInvitationCredential(session sessions.Session, affCode string) {
	if session == nil {
		return
	}
	affCode = strings.TrimSpace(affCode)
	if affCode == "" {
		session.Delete("aff")
	} else {
		session.Set("aff", affCode)
	}
	// 清理旧版本可能遗留在服务端会话中的签名字段。
	session.Delete("invite")
}

func resolveOAuthRegistrationInviter(session sessions.Session) (int, error) {
	credential := registrationInvitationCredentialFromSession(session)
	return resolveNewUserRegistrationInviter(credential.AffCode)
}

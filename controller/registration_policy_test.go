package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type registrationPolicySession struct {
	values map[interface{}]interface{}
}

func (s *registrationPolicySession) ID() string { return "registration-policy" }
func (s *registrationPolicySession) Get(key interface{}) interface{} {
	return s.values[key]
}
func (s *registrationPolicySession) Set(key interface{}, value interface{}) {
	s.values[key] = value
}
func (s *registrationPolicySession) Delete(key interface{}) { delete(s.values, key) }
func (s *registrationPolicySession) Clear()                 { clear(s.values) }
func (s *registrationPolicySession) AddFlash(value interface{}, vars ...string) {
}
func (s *registrationPolicySession) Flashes(vars ...string) []interface{} { return nil }
func (s *registrationPolicySession) Options(options sessions.Options)     {}
func (s *registrationPolicySession) Save() error                          { return nil }

type registrationPolicyOAuthProvider struct {
	taken    bool
	existing model.User
}

func (p *registrationPolicyOAuthProvider) GetName() string { return "Registration Policy" }
func (p *registrationPolicyOAuthProvider) IsEnabled() bool { return true }
func (p *registrationPolicyOAuthProvider) ExchangeToken(context.Context, string, *gin.Context) (*oauth.OAuthToken, error) {
	return nil, nil
}
func (p *registrationPolicyOAuthProvider) GetUserInfo(context.Context, *oauth.OAuthToken) (*oauth.OAuthUser, error) {
	return nil, nil
}
func (p *registrationPolicyOAuthProvider) IsUserIDTaken(string) bool { return p.taken }
func (p *registrationPolicyOAuthProvider) FillUserByProviderID(user *model.User, _ string) error {
	*user = p.existing
	return nil
}
func (p *registrationPolicyOAuthProvider) SetProviderUserID(user *model.User, providerUserID string) {
	user.GitHubId = providerUserID
}
func (p *registrationPolicyOAuthProvider) GetProviderPrefix() string { return "invite_oauth_" }

func setupRegistrationPolicyTest(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	originalRegisterEnabled := common.RegisterEnabled
	originalInvitationRegisterEnabled := common.InvitationRegisterEnabled
	originalPasswordRegisterEnabled := common.PasswordRegisterEnabled
	originalEmailVerificationEnabled := common.EmailVerificationEnabled
	originalQuotaForNewUser := common.QuotaForNewUser
	originalQuotaForInviter := common.QuotaForInviter
	originalQuotaForInvitee := common.QuotaForInvitee
	originalRedisEnabled := common.RedisEnabled
	originalUsingSQLite := common.UsingSQLite
	originalGenerateDefaultToken := constant.GenerateDefaultToken

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "invitation-registration.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AffiliateRiskUser{}, &model.Group{}))
	require.NoError(t, db.Create(&model.Group{Code: "default", Name: "默认分组", Status: model.GroupStatusActive}).Error)
	model.DB = db
	common.QuotaForNewUser = 0
	common.QuotaForInviter = 0
	common.QuotaForInvitee = 0
	common.RedisEnabled = false
	common.UsingSQLite = true
	constant.GenerateDefaultToken = false

	t.Cleanup(func() {
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = originalDB
		common.RegisterEnabled = originalRegisterEnabled
		common.InvitationRegisterEnabled = originalInvitationRegisterEnabled
		common.PasswordRegisterEnabled = originalPasswordRegisterEnabled
		common.EmailVerificationEnabled = originalEmailVerificationEnabled
		common.QuotaForNewUser = originalQuotaForNewUser
		common.QuotaForInviter = originalQuotaForInviter
		common.QuotaForInvitee = originalQuotaForInvitee
		common.RedisEnabled = originalRedisEnabled
		common.UsingSQLite = originalUsingSQLite
		constant.GenerateDefaultToken = originalGenerateDefaultToken
	})
}

func createRegistrationPolicyInviter(t *testing.T, affCode string, blocked bool) int {
	t.Helper()
	user := model.User{
		Username:  "inviter_" + affCode,
		AffCode:   affCode,
		Status:    common.UserStatusEnabled,
		Role:      common.RoleCommonUser,
		Group:     "default",
		CreatedAt: common.GetTimestamp(),
	}
	require.NoError(t, model.DB.Create(&user).Error)
	if blocked {
		require.NoError(t, model.DB.Create(&model.AffiliateRiskUser{
			UserId:          user.Id,
			Status:          model.AffiliateRiskStatusActive,
			BlockInviteCode: true,
		}).Error)
	}
	return user.Id
}

func TestResolveNewUserRegistrationInviter(t *testing.T) {
	setupRegistrationPolicyTest(t)
	validInviterId := createRegistrationPolicyInviter(t, "valid-code", false)
	createRegistrationPolicyInviter(t, "blocked-code", true)
	disabledInviterId := createRegistrationPolicyInviter(t, "disabled-code", false)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", disabledInviterId).
		Update("status", common.UserStatusDisabled).Error)
	tests := []struct {
		name              string
		registerEnabled   bool
		invitationEnabled bool
		affCode           string
		wantInviterId     int
		wantAllowed       bool
	}{
		{name: "公开注册开启且无邀请码", registerEnabled: true, affCode: "", wantAllowed: true},
		{name: "公开注册开启且邀请码有效", registerEnabled: true, affCode: "valid-code", wantInviterId: validInviterId, wantAllowed: true},
		{name: "公开注册开启且邀请码无效", registerEnabled: true, affCode: "missing-code", wantAllowed: true},
		{name: "公开注册开启且邀请码被封禁", registerEnabled: true, affCode: "blocked-code", wantAllowed: true},
		{name: "公开注册关闭且邀请注册关闭", affCode: "valid-code", wantAllowed: false},
		{name: "邀请注册开启但缺少邀请码", invitationEnabled: true, affCode: "", wantAllowed: false},
		{name: "邀请注册开启但邀请码无效", invitationEnabled: true, affCode: "missing-code", wantAllowed: false},
		{name: "邀请注册开启但邀请码被封禁", invitationEnabled: true, affCode: "blocked-code", wantAllowed: false},
		{name: "邀请注册开启但邀请人已禁用", invitationEnabled: true, affCode: "disabled-code", wantAllowed: false},
		{name: "邀请注册开启且邀请码有效", invitationEnabled: true, affCode: " valid-code ", wantInviterId: validInviterId, wantAllowed: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			common.RegisterEnabled = test.registerEnabled
			common.InvitationRegisterEnabled = test.invitationEnabled
			inviterId, err := resolveNewUserRegistrationInviter(test.affCode)
			if test.wantAllowed {
				require.NoError(t, err)
				require.Equal(t, test.wantInviterId, inviterId)
				return
			}
			require.ErrorIs(t, err, errNewUserRegistrationDisabled)
			require.Zero(t, inviterId)
		})
	}
}

func TestInvitationRegistrationFailsClosedWhenRiskStateUnavailable(t *testing.T) {
	setupRegistrationPolicyTest(t)
	createRegistrationPolicyInviter(t, "risk-state-unavailable", false)
	require.NoError(t, model.DB.Migrator().DropTable(&model.AffiliateRiskUser{}))
	common.RegisterEnabled = false
	common.InvitationRegisterEnabled = true
	inviterId, err := resolveNewUserRegistrationInviter("risk-state-unavailable")
	require.ErrorIs(t, err, errNewUserRegistrationDisabled)
	require.Zero(t, inviterId)
}

func TestOAuthRegistrationUsesSessionInviteAndPreservesExistingLogin(t *testing.T) {
	setupRegistrationPolicyTest(t)
	validInviterId := createRegistrationPolicyInviter(t, "oauth-valid", false)
	common.RegisterEnabled = false
	common.InvitationRegisterEnabled = true

	session := &registrationPolicySession{values: map[interface{}]interface{}{
		"aff": "oauth-valid",
	}}
	created, err := findOrCreateOAuthUser(nil, &registrationPolicyOAuthProvider{}, &oauth.OAuthUser{
		ProviderUserID: "new-provider-user",
		Username:       "new_oauth_user",
		DisplayName:    "New OAuth User",
	}, session)
	require.NoError(t, err)
	require.Equal(t, validInviterId, created.InviterId)

	var stored model.User
	require.NoError(t, model.DB.First(&stored, created.Id).Error)
	require.Equal(t, validInviterId, stored.InviterId)

	common.InvitationRegisterEnabled = false
	existing := model.User{Id: 9001, Username: "existing", Status: common.UserStatusEnabled}
	loggedIn, err := findOrCreateOAuthUser(nil, &registrationPolicyOAuthProvider{
		taken:    true,
		existing: existing,
	}, &oauth.OAuthUser{ProviderUserID: "existing-provider-user"}, nil)
	require.NoError(t, err)
	require.Equal(t, existing.Id, loggedIn.Id)
}

func TestOAuthNewUserRejectsMissingInviteWhenPublicRegistrationIsClosed(t *testing.T) {
	setupRegistrationPolicyTest(t)
	common.RegisterEnabled = false
	common.InvitationRegisterEnabled = true

	_, err := findOrCreateOAuthUser(nil, &registrationPolicyOAuthProvider{}, &oauth.OAuthUser{
		ProviderUserID: "new-without-invite",
	}, &registrationPolicySession{values: make(map[interface{}]interface{})})
	require.Error(t, err)
	require.IsType(t, &OAuthRegistrationDisabledError{}, err)
}

func TestPasswordRegistrationRemainsDisabledWithValidInvite(t *testing.T) {
	setupRegistrationPolicyTest(t)
	createRegistrationPolicyInviter(t, "password-code", false)
	common.RegisterEnabled = false
	common.InvitationRegisterEnabled = true
	common.PasswordRegisterEnabled = false

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(
		`{"username":"password_invitee","password":"password123","aff_code":"password-code"}`,
	))
	ctx.Request.Header.Set("Content-Type", "application/json")
	Register(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.False(t, response.Success)
	var userCount int64
	require.NoError(t, model.DB.Model(&model.User{}).Count(&userCount).Error)
	require.EqualValues(t, 1, userCount)
}

func TestPasswordRegistrationRejectsInvalidInviteBeforeAccountSpecificChecks(t *testing.T) {
	setupRegistrationPolicyTest(t)
	createRegistrationPolicyInviter(t, "oracle", false)
	common.RegisterEnabled = false
	common.InvitationRegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = true

	router := gin.New()
	router.POST("/api/user/register", Register)
	request := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		return recorder
	}

	existingUser := request(`{"username":"inviter_oracle","password":"password123"}`)
	invalidVerification := request(`{"username":"new_oracle","password":"password123","email":"new@example.com","verification_code":"wrong","aff_code":"missing-code"}`)
	require.Equal(t, http.StatusOK, existingUser.Code)
	require.Equal(t, http.StatusOK, invalidVerification.Code)
	require.JSONEq(t, existingUser.Body.String(), invalidVerification.Body.String())

	var userCount int64
	require.NoError(t, model.DB.Model(&model.User{}).Count(&userCount).Error)
	require.EqualValues(t, 1, userCount)
}

func TestPasswordRegistrationInvitePolicyThroughPostRoute(t *testing.T) {
	setupRegistrationPolicyTest(t)
	validInviterId := createRegistrationPolicyInviter(t, "post-valid", false)
	createRegistrationPolicyInviter(t, "post-blocked", true)
	common.RegisterEnabled = false
	common.InvitationRegisterEnabled = true
	common.PasswordRegisterEnabled = true
	common.EmailVerificationEnabled = false

	router := gin.New()
	router.POST("/api/user/register", Register)

	tests := []struct {
		name          string
		username      string
		affCode       string
		wantSuccess   bool
		wantInviterId int
	}{
		{name: "有效邀请码注册成功", username: "post_valid_user", affCode: "post-valid", wantSuccess: true, wantInviterId: validInviterId},
		{name: "缺少邀请码注册拒绝", username: "post_missing_code", affCode: "", wantSuccess: false},
		{name: "无效邀请码注册拒绝", username: "post_invalid_user", affCode: "not-found", wantSuccess: false},
		{name: "风控封禁邀请码注册拒绝", username: "post_blocked_user", affCode: "post-blocked", wantSuccess: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestBody := `{"username":"` + test.username + `","password":"password123","aff_code":"` +
				test.affCode + `"}`
			request := httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(requestBody))
			request.Header.Set("Content-Type", "application/json")
			request.RemoteAddr = "127.0.0.1:12345"
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, test.wantSuccess, response.Success)

			var registered model.User
			err := model.DB.Where("username = ?", test.username).First(&registered).Error
			if !test.wantSuccess {
				require.ErrorIs(t, err, gorm.ErrRecordNotFound)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.wantInviterId, registered.InviterId)
			require.Equal(t, common.RoleCommonUser, registered.Role)
			require.Equal(t, common.UserStatusEnabled, registered.Status)
		})
	}
}

func TestSetOAuthRegistrationInvitationCredentialClearsStaleValue(t *testing.T) {
	session := &registrationPolicySession{values: map[interface{}]interface{}{
		"aff": "stale-code", "invite": "stale-signature",
	}}
	setOAuthRegistrationInvitationCredential(session, "")
	require.Nil(t, session.Get("aff"))
	require.Nil(t, session.Get("invite"))

	setOAuthRegistrationInvitationCredential(session, "  current-code  ")
	require.Equal(t, "current-code", session.Get("aff"))
	require.Nil(t, session.Get("invite"))
}

func TestAffiliateInviteLinkContainsOnlyAffiliateCode(t *testing.T) {
	setupRegistrationPolicyTest(t)
	originalServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://invite.example.test/base/"
	t.Cleanup(func() { system_setting.ServerAddress = originalServerAddress })

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/affiliate/summary", nil)
	inviteLink, err := buildAffiliateInviteLink(ctx, "round-trip-link")
	require.NoError(t, err)
	parsed, err := url.Parse(inviteLink)
	require.NoError(t, err)
	require.Equal(t, "/base/register", parsed.Path)
	require.Equal(t, "round-trip-link", parsed.Query().Get("aff"))
	require.Empty(t, parsed.Query().Get("invite"))
}

func TestAffiliateInviteLinkSupportsInvitationOnlyRegistration(t *testing.T) {
	setupRegistrationPolicyTest(t)
	common.RegisterEnabled = false
	common.InvitationRegisterEnabled = true

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/affiliate/summary", nil)
	inviteLink, err := buildAffiliateInviteLink(ctx, "invitation-only-link")
	require.NoError(t, err)
	parsed, err := url.Parse(inviteLink)
	require.NoError(t, err)
	require.Equal(t, "invitation-only-link", parsed.Query().Get("aff"))
	require.Empty(t, parsed.Query().Get("invite"))
}

func TestOAuthStateSavesAndClearsInvitationCredentialTogether(t *testing.T) {
	setupRegistrationPolicyTest(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("registration-policy-session-test"))))
	router.GET("/api/oauth/state", GenerateOAuthCode)
	router.GET("/test/session", func(c *gin.Context) {
		session := sessions.Default(c)
		c.JSON(http.StatusOK, gin.H{"aff": session.Get("aff"), "invite": session.Get("invite")})
	})

	setRequest := httptest.NewRequest(http.MethodGet,
		"/api/oauth/state?aff=oauth-state&invite=legacy-signature", nil)
	setRecorder := httptest.NewRecorder()
	router.ServeHTTP(setRecorder, setRequest)
	require.Equal(t, http.StatusOK, setRecorder.Code)
	require.NotEmpty(t, setRecorder.Result().Cookies())

	readSession := func(cookieValue *http.Cookie) map[string]interface{} {
		request := httptest.NewRequest(http.MethodGet, "/test/session", nil)
		request.AddCookie(cookieValue)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		var values map[string]interface{}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &values))
		return values
	}
	setCookie := setRecorder.Result().Cookies()[0]
	values := readSession(setCookie)
	require.Equal(t, "oauth-state", values["aff"])
	require.Nil(t, values["invite"])

	clearRequest := httptest.NewRequest(http.MethodGet, "/api/oauth/state", nil)
	clearRequest.AddCookie(setCookie)
	clearRecorder := httptest.NewRecorder()
	router.ServeHTTP(clearRecorder, clearRequest)
	require.Equal(t, http.StatusOK, clearRecorder.Code)
	require.NotEmpty(t, clearRecorder.Result().Cookies())
	cleared := readSession(clearRecorder.Result().Cookies()[0])
	require.Nil(t, cleared["aff"])
	require.Nil(t, cleared["invite"])
}

func TestInvitationRegistrationRevalidationRejectsDisabledInviter(t *testing.T) {
	setupRegistrationPolicyTest(t)
	inviterId := createRegistrationPolicyInviter(t, "revalidate", false)
	credential := registrationInvitationCredential{
		AffCode: "revalidate",
	}
	common.RegisterEnabled = false
	common.InvitationRegisterEnabled = true
	resolved, err := resolveNewUserRegistrationInviter(credential.AffCode)
	require.NoError(t, err)
	require.Equal(t, inviterId, resolved)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", inviterId).
		Update("status", common.UserStatusDisabled).Error)
	require.ErrorIs(t,
		revalidateNewUserRegistrationInviterWithDB(model.DB, credential, inviterId),
		errNewUserRegistrationDisabled,
	)
}

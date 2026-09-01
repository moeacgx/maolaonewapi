package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromoCodeAdminRoutesAreRegisteredAndRejectNonAdmins(t *testing.T) {
	setupRelayRouterTestDB(t)

	accessToken := "promo-common-user-access-token"
	user := &model.User{
		Username:    "promo-common-user",
		Password:    "password-placeholder",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AccessToken: &accessToken,
		AuthVersion: 1,
		AffCode:     "promo-common-user-aff",
	}
	require.NoError(t, model.DB.Create(user).Error)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	routes := []struct {
		method         string
		registeredPath string
		requestPath    string
	}{
		{http.MethodGet, "/api/promo_code/", "/api/promo_code/"},
		{http.MethodGet, "/api/promo_code/search", "/api/promo_code/search"},
		{http.MethodGet, "/api/promo_code/:id", "/api/promo_code/1"},
		{http.MethodPost, "/api/promo_code/", "/api/promo_code/"},
		{http.MethodPut, "/api/promo_code/", "/api/promo_code/"},
		{http.MethodDelete, "/api/promo_code/invalid", "/api/promo_code/invalid"},
		{http.MethodDelete, "/api/promo_code/batch", "/api/promo_code/batch"},
		{http.MethodDelete, "/api/promo_code/:id", "/api/promo_code/1"},
	}

	registered := make(map[string]struct{}, len(engine.Routes()))
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	for _, route := range routes {
		_, ok := registered[route.method+" "+route.registeredPath]
		assert.True(t, ok, "route %s %s must be registered", route.method, route.registeredPath)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(route.method, route.requestPath, nil)
		request.Header.Set("Authorization", "Bearer "+accessToken)
		engine.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusForbidden, recorder.Code, "non-admin reached %s %s", route.method, route.requestPath)
	}
}

/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesWebsocketRouteRegistration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetRelayRouter(engine)

	var getRoute, postRoute *gin.RouteInfo
	for index := range engine.Routes() {
		route := engine.Routes()[index]
		if route.Path != "/v1/responses" {
			continue
		}
		switch route.Method {
		case http.MethodGet:
			getRoute = &route
		case http.MethodPost:
			postRoute = &route
		}
	}

	require.NotNil(t, getRoute)
	require.NotNil(t, postRoute)
	assert.Contains(t, getRoute.Handler, "ResponsesWebsocket")
	assert.Contains(t, postRoute.Handler, "SetRelayRouter.func")
	assert.NotEqual(t, getRoute.Handler, postRoute.Handler)
}

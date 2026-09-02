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
package service

import (
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestAppendTransportInfoWritesStableFields(t *testing.T) {
	other := map[string]interface{}{}
	AppendTransportInfo(other, "websocket", "http")

	assert.Equal(t, "websocket", other["transport"])
	assert.Equal(t, "http", other["upstream_transport"])
}

func TestAppendTransportInfoDefaultsEmptyValuesToHTTP(t *testing.T) {
	other := map[string]interface{}{}
	AppendTransportInfo(other, "", "")

	assert.Equal(t, "http", other["transport"])
	assert.Equal(t, "http", other["upstream_transport"])
}

func TestGenerateTextOtherInfoUsesRelayTransport(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	now := time.Now()
	info := &relaycommon.RelayInfo{
		StartTime:         now,
		FirstResponseTime: now,
		Transport:         "websocket",
		UpstreamTransport: "http",
		ChannelMeta:       &relaycommon.ChannelMeta{},
	}

	other := GenerateTextOtherInfo(context, info, 1, 1, 1, 0, 0, 0, 1)

	assert.Equal(t, "websocket", other["transport"])
	assert.Equal(t, "http", other["upstream_transport"])
}

func TestGenerateWssOtherInfoMarksBothSidesWebsocket(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", "/v1/realtime", nil)
	now := time.Now()
	info := &relaycommon.RelayInfo{
		StartTime:         now,
		FirstResponseTime: now,
		ChannelMeta:       &relaycommon.ChannelMeta{},
	}

	other := GenerateWssOtherInfo(context, info, &dto.RealtimeUsage{}, 1, 1, 1, 1, 1, 0, 1)

	assert.Equal(t, "websocket", other["transport"])
	assert.Equal(t, "websocket", other["upstream_transport"])
}

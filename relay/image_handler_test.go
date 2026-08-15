package relay

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPrepareImagePassthroughBodyReturnsReplayablePayload(t *testing.T) {
	payload := []byte("multipart-image-payload")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(payload))
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	info := &relaycommon.RelayInfo{}

	reader, err := prepareImagePassthroughBody(c, info)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(reader)
	require.NoError(t, err)

	require.Equal(t, payload, forwarded)
	_, replayable := reader.(common.ReplayableBody)
	require.True(t, replayable)
}

func TestResolveImageSettlementCount(t *testing.T) {
	tests := []struct {
		name        string
		requested   uint
		ratios      map[string]float64
		billingMeta map[string]string
		settled     uint
		delivered   uint
	}{
		{name: "无实际数量时使用请求数量", requested: 4, settled: 4, delivered: 4},
		{name: "少交付时仍按请求数量结算", requested: 4, ratios: map[string]float64{"n": 1}, settled: 4, delivered: 1},
		{name: "多交付时按请求数量封顶", requested: 2, ratios: map[string]float64{"n": 4}, settled: 2, delivered: 4},
		{name: "fanout 按实际成功数量结算", requested: 4, ratios: map[string]float64{"n": 2}, billingMeta: map[string]string{"image_count_settlement": "actual"}, settled: 2, delivered: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settled, delivered := resolveImageSettlementCount(test.requested, test.ratios, test.billingMeta)
			require.Equal(t, test.settled, settled)
			require.Equal(t, test.delivered, delivered)
		})
	}
}

func TestNormalizeImageUsageInfoUsesOutputTokensForSyntheticImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	usage := normalizeImageUsageInfo(c, &dto.Usage{}, 2)

	require.Equal(t, 2, usage.TotalTokens)
	require.Zero(t, usage.PromptTokens)
	require.Equal(t, 2, usage.CompletionTokens)
	require.Equal(t, 2, usage.CompletionTokenDetails.ImageTokens)
	require.True(t, common.GetContextKeyBool(c, constant.ContextKeyImageTokenUsageSynthetic))
}

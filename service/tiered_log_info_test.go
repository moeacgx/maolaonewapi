package service

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInjectTieredBillingInfoIncludesActualSettlementTrace(t *testing.T) {
	expr := `tier("base", p * 2 + c * 10)`
	relayInfo := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:               "tiered_expr",
			ExprString:                expr,
			ExprHash:                  billingexpr.ExprHashString(expr),
			EstimatedTier:             "estimate",
			EstimatedQuotaBeforeGroup: 100,
			EstimatedQuotaAfterGroup:  125,
			GroupRatio:                1.25,
			QuotaPerUnit:              500000,
		},
	}
	requestRules := []billingexpr.RequestRuleTrace{{Cond: `param("fast") == true`, Multiplier: 2, Matched: true}}
	result := &billingexpr.TieredResult{
		ActualQuotaBeforeGroup: 200,
		ActualQuotaAfterGroup:  250,
		MatchedTier:            "base",
		RequestRules:           requestRules,
		TokenParams:            billingexpr.TokenParams{P: 100, C: 20},
		CrossedTier:            true,
	}

	other := map[string]interface{}{}
	InjectTieredBillingInfo(other, relayInfo, result)

	require.Equal(t, "tiered_expr", other["billing_mode"])
	assert.Equal(t, "base", other["matched_tier"])
	assert.Equal(t, "estimate", other["estimated_tier"])
	assert.Equal(t, float64(100), other["estimated_quota_before_group"])
	assert.Equal(t, 125, other["estimated_quota_after_group"])
	assert.Equal(t, float64(200), other["actual_quota_before_group"])
	assert.Equal(t, 250, other["actual_quota_after_group"])
	assert.Equal(t, 1.25, other["group_ratio"])
	assert.Equal(t, float64(500000), other["quota_per_unit"])
	assert.Equal(t, true, other["crossed_tier"])
	assert.Equal(t, billingexpr.TokenParams{P: 100, C: 20}, other["tiered_token_params"])
	assert.Equal(t, requestRules, other["request_rules"])
	assert.Equal(t, 2.0, other["request_multiplier"])
	assert.NotEmpty(t, other["expr_b64"])
}

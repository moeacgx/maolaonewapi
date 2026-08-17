package game_setting

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGameSettingNormalizesUnsafeValues(t *testing.T) {
	oldRate, oldFee := gameSetting.TokenExchangeRate, gameSetting.AwardFeeRate
	t.Cleanup(func() {
		gameSetting.TokenExchangeRate = oldRate
		gameSetting.AwardFeeRate = oldFee
	})

	gameSetting.TokenExchangeRate = 0
	assert.Equal(t, 100, GetTokenExchangeRate())
	gameSetting.TokenExchangeRate = -10
	assert.Equal(t, 100, GetTokenExchangeRate())
	assert.Equal(t, float64(0), NormalizeAwardFeeRate(math.NaN()))
	assert.Equal(t, float64(0), NormalizeAwardFeeRate(math.Inf(-1)))
	assert.Equal(t, float64(0), NormalizeAwardFeeRate(-0.1))
	assert.Equal(t, float64(1), NormalizeAwardFeeRate(1.1))
}

func TestGameSettingKeepsAutomaticJudgingDisabledByDefault(t *testing.T) {
	old := gameSetting.AutoJudgeEnabled
	t.Cleanup(func() { gameSetting.AutoJudgeEnabled = old })
	gameSetting.AutoJudgeEnabled = true
	assert.False(t, IsAutoJudgeEnabled())
	gameSetting.AutoJudgeEnabled = false
	assert.False(t, IsAutoJudgeEnabled())
}

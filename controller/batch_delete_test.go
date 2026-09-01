package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeBatchDeleteIDs(t *testing.T) {
	ids, err := normalizeBatchDeleteIDs("优惠码", []int{3, 1, 3, 2})
	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, ids)

	_, err = normalizeBatchDeleteIDs("优惠码", nil)
	require.EqualError(t, err, "优惠码 ID 数量必须为 1-500")

	_, err = normalizeBatchDeleteIDs("优惠码", []int{1, 0})
	require.EqualError(t, err, "优惠码 ID 必须为正整数")
}

func TestNormalizeBatchDeleteIDsRejectsTooManyIDs(t *testing.T) {
	ids := make([]int, maxBatchDeleteIDs+1)
	for i := range ids {
		ids[i] = i + 1
	}
	_, err := normalizeBatchDeleteIDs("福利活动", ids)
	require.EqualError(t, err, "福利活动 ID 数量必须为 1-500")
}

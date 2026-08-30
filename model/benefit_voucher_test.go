package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitBenefitSharesPreservesRandomBudgetAndBounds(t *testing.T) {
	shares, err := SplitBenefitShares(BenefitShareSplitInput{
		Mode:             BenefitAmountModeRandom,
		TotalAmountCents: 1000,
		TotalCount:       4,
		MinAmountCents:   100,
		MaxAmountCents:   700,
		QuotaPerCent:     10,
		RandomIntn: func(max int) int {
			return max / 2
		},
	})
	require.NoError(t, err)
	require.Len(t, shares, 4)

	var totalAmount int64
	var totalQuota int64
	for _, share := range shares {
		assert.GreaterOrEqual(t, share.AmountCents, int64(100))
		assert.LessOrEqual(t, share.AmountCents, int64(700))
		assert.Equal(t, share.AmountCents*10, share.Quota)
		totalAmount += share.AmountCents
		totalQuota += share.Quota
	}
	assert.Equal(t, int64(1000), totalAmount)
	assert.Equal(t, int64(10000), totalQuota)
}

func TestSplitBenefitSharesRejectsUnsatisfiableRandomBounds(t *testing.T) {
	_, err := SplitBenefitShares(BenefitShareSplitInput{
		Mode:             BenefitAmountModeRandom,
		TotalAmountCents: 100,
		TotalCount:       2,
		MinAmountCents:   60,
		MaxAmountCents:   80,
		QuotaPerCent:     10,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "120")
	assert.Contains(t, err.Error(), "160")
}

func TestSplitBenefitSharesRequiresExactFixedBudget(t *testing.T) {
	shares, err := SplitBenefitShares(BenefitShareSplitInput{
		Mode:             BenefitAmountModeFixed,
		TotalAmountCents: 300,
		TotalCount:       3,
		FixedAmountCents: 100,
		QuotaPerCent:     25,
	})
	require.NoError(t, err)
	require.Len(t, shares, 3)
	for _, share := range shares {
		assert.Equal(t, BenefitShareAllocation{AmountCents: 100, Quota: 2500}, share)
	}

	_, err = SplitBenefitShares(BenefitShareSplitInput{
		Mode:             BenefitAmountModeFixed,
		TotalAmountCents: 301,
		TotalCount:       3,
		FixedAmountCents: 100,
		QuotaPerCent:     25,
	})
	require.Error(t, err)
}

func TestSaveGroupConfigPersistsSingleUserConcurrencyLimit(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{},
		&Channel{}, &Token{}, &User{},
	))

	group := &Group{
		Code: "benefit", Name: "活动福利", Ratio: 1,
		Status: GroupStatusActive, CreatedTime: 1, UpdatedTime: 1,
	}
	require.NoError(t, db.Create(group).Error)

	require.NoError(t, SaveGroupConfig([]GroupConfig{{
		Id: group.Id, Code: group.Code, Name: group.Name, Ratio: 1,
		Status: GroupStatusActive, SingleUserConcurrencyLimit: 3,
	}}, nil))

	var stored Group
	require.NoError(t, db.First(&stored, group.Id).Error)
	assert.Equal(t, 3, stored.SingleUserConcurrencyLimit)
	assert.Equal(t, 3, stored.ToConfig(nil).SingleUserConcurrencyLimit)
}

func TestSaveGroupConfigRejectsNegativeSingleUserConcurrencyLimit(t *testing.T) {
	db := openGroupIdentityTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&Option{}, &Group{}, &GroupAlias{}, &AutoGroupMember{},
		&Channel{}, &Token{}, &User{},
	))
	group := &Group{Code: "benefit", Name: "活动福利", Ratio: 1, Status: GroupStatusActive}
	require.NoError(t, db.Create(group).Error)

	err := SaveGroupConfig([]GroupConfig{{
		Id: group.Id, Code: group.Code, Name: group.Name, Ratio: 1,
		Status: GroupStatusActive, SingleUserConcurrencyLimit: -1,
	}}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "并发")
}

package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPromptAuditEventChannelFilterAndKeywordCiphertextLookup(t *testing.T) {
	db := setupPromptAuditTestDB(t)
	now := time.Now().Unix()
	first := promptAuditTestEvent("channel-filter-first", now)
	first.ChannelId = 41
	first.MatchedKeywordsCiphertext = "plain_kw1:[\"first-keyword\"]"
	second := promptAuditTestEvent("channel-filter-second", now+1)
	second.ChannelId = 42
	second.MatchedKeywordsCiphertext = "plain_kw1:[\"second-keyword\"]"
	require.NoError(t, db.Create(&first).Error)
	require.NoError(t, db.Create(&second).Error)

	events, total, err := ListPromptAuditEvents(PromptAuditEventFilter{ChannelId: 42}, 1, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, events, 1)
	require.Equal(t, second.Id, events[0].Id)
	require.Empty(t, events[0].MatchedKeywordsCiphertext)

	ciphertexts, err := GetPromptAuditEventMatchedKeywordCiphertexts([]int64{first.Id, second.Id})
	require.NoError(t, err)
	require.Equal(t, first.MatchedKeywordsCiphertext, ciphertexts[first.Id])
	require.Equal(t, second.MatchedKeywordsCiphertext, ciphertexts[second.Id])
}

package setting

import (
	"reflect"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSensitivePolicySnapshotPublishesAtomically(t *testing.T) {
	original := GetSensitivePolicySnapshot()
	t.Cleanup(func() { ReplaceSensitivePolicySnapshot(original) })

	first := SensitivePolicySnapshot{
		CheckEnabled:         true,
		CheckOnPromptEnabled: false,
		Words:                []string{"first-word"},
		Rules: []SensitiveRule{{
			ID:         "first-rule",
			Enabled:    true,
			Action:     SensitiveRuleActionBlock,
			Scope:      SensitiveRuleScopeRequest,
			Keywords:   []string{"first-keyword"},
			TargetType: SensitiveRuleTargetChannels,
			ChannelIds: []int{11},
		}},
		RulesConfigured:  true,
		LegacyChannelIds: []int{101},
	}
	second := SensitivePolicySnapshot{
		CheckEnabled:         false,
		CheckOnPromptEnabled: true,
		Words:                []string{"second-word"},
		Rules: []SensitiveRule{{
			ID:          "second-rule",
			Enabled:     true,
			Action:      SensitiveRuleActionMask,
			Scope:       SensitiveRuleScopeResponse,
			Keywords:    []string{"second-keyword"},
			TargetType:  SensitiveRuleTargetChannelTags,
			ChannelTags: []string{"batch-b"},
		}},
		RulesConfigured:  false,
		LegacyChannelIds: []int{202},
	}
	ReplaceSensitivePolicySnapshot(first)

	const iterations = 2000
	errors := make(chan SensitivePolicySnapshot, 1)
	var workers sync.WaitGroup
	workers.Add(5)
	go func() {
		defer workers.Done()
		for index := 0; index < iterations; index++ {
			if index%2 == 0 {
				ReplaceSensitivePolicySnapshot(second)
			} else {
				ReplaceSensitivePolicySnapshot(first)
			}
		}
	}()
	for reader := 0; reader < 4; reader++ {
		go func() {
			defer workers.Done()
			for index := 0; index < iterations; index++ {
				snapshot := GetSensitivePolicySnapshot()
				if reflect.DeepEqual(snapshot, first) || reflect.DeepEqual(snapshot, second) {
					continue
				}
				select {
				case errors <- snapshot:
				default:
				}
				return
			}
		}()
	}
	workers.Wait()
	close(errors)
	for mixed := range errors {
		t.Fatalf("读取到混合的屏蔽词策略快照: %#v", mixed)
	}
}

func TestParseSensitiveRulesJSONStringNormalizesRules(t *testing.T) {
	raw := `{
		"rules": [
			{
				"id": " rule-1 ",
				"name": " Mask rule ",
				"enabled": true,
				"action": "mask",
				"scope": " response ",
				"replacement": " [MASK] ",
				"keywords": [" Secret ", "secret", "", "中文"],
				"group_refs": [" 1 ", "1", "", "Sensitive Group"],
				"target_type": " CHANNELS ",
				"channel_ids": [9, 3, 9, 0, -1]
			},
			{
				"id": "",
				"name": "",
				"enabled": true,
				"action": "unknown",
				"scope": "unknown",
				"keywords": [" block-me "]
			}
		]
	}`

	rules, err := ParseSensitiveRulesJSONString(raw)
	require.NoError(t, err)
	require.Len(t, rules, 2)

	assert.Equal(t, "rule-1", rules[0].ID)
	assert.Equal(t, "Mask rule", rules[0].Name)
	assert.Equal(t, SensitiveRuleActionMask, rules[0].Action)
	assert.Equal(t, SensitiveRuleScopeResponse, rules[0].Scope)
	assert.Equal(t, "[MASK]", rules[0].Replacement)
	assert.Equal(t, []string{"Secret", "中文"}, rules[0].Keywords)
	assert.Equal(t, []string{"1", "Sensitive Group"}, rules[0].GroupRefs)
	assert.Equal(t, SensitiveRuleTargetChannels, rules[0].TargetType)
	assert.Equal(t, []int{3, 9}, rules[0].ChannelIds)
	assert.Empty(t, rules[0].ChannelTags)

	assert.Equal(t, "block-me", rules[1].ID)
	assert.Equal(t, "block-me", rules[1].Name)
	assert.Equal(t, SensitiveRuleActionBlock, rules[1].Action)
	assert.Equal(t, SensitiveRuleScopeRequest, rules[1].Scope)
	assert.Equal(t, []string{"block-me"}, rules[1].Keywords)
}

func TestParseSensitiveRulesJSONStringNormalizesChannelTagTargets(t *testing.T) {
	rules, err := ParseSensitiveRulesJSONString(`{
		"rules": [{
			"id": "groups",
			"name": "Groups",
			"enabled": true,
			"action": "block",
			"scope": "request",
			"keywords": ["blocked"],
			"target_type": "channel_tags",
			"channel_tags": [" backup ", "primary", "backup", ""]
		}]
	}`)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, SensitiveRuleTargetChannelTags, rules[0].TargetType)
	assert.Equal(t, []string{"backup", "primary"}, rules[0].ChannelTags)
	assert.Empty(t, rules[0].ChannelIds)
}

func TestParseSensitiveRulesJSONStringNormalizesCombinedRouteTargets(t *testing.T) {
	rules, err := ParseSensitiveRulesJSONString(`{
		"rules": [{
			"id": "routes",
			"name": "Routes",
			"enabled": true,
			"action": "block",
			"scope": "request",
			"keywords": ["blocked"],
			"target_type": "routes",
			"channel_ids": [9, 3, 9, 0],
			"group_codes": [" beta ", "alpha", "beta", ""]
		}]
	}`)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, SensitiveRuleTargetRoutes, rules[0].TargetType)
	assert.Equal(t, []int{3, 9}, rules[0].ChannelIds)
	assert.Equal(t, []string{"alpha", "beta"}, rules[0].GroupCodes)
	assert.Empty(t, rules[0].ChannelTags)
}

func TestParseSensitiveRulesJSONStringRejectsInvalidTargetContracts(t *testing.T) {
	tests := []string{
		`{"rules":[{"enabled":true,"keywords":["x"],"channel_ids":[1]}]}`,
		`{"rules":[{"enabled":true,"keywords":["x"],"group_codes":["group-a"]}]}`,
		`{"rules":[{"enabled":true,"keywords":["x"],"target_type":"channels","channel_tags":["group-a"]}]}`,
		`{"rules":[{"enabled":true,"keywords":["x"],"target_type":"channel_tags","channel_ids":[1]}]}`,
		`{"rules":[{"enabled":true,"keywords":["x"],"target_type":"channel_tags","channel_tags":["tag-a"],"group_codes":["group-a"]}]}`,
		`{"rules":[{"enabled":true,"keywords":["x"],"target_type":"groups","group_ids":[1]}]}`,
		`{"rules":[{"enabled":true,"keywords":["x"],"target_type":"channel_groups","channel_group_ids":[1]}]}`,
		`{"rules":[{"enabled":true,"keywords":["x"],"target_type":"channel_tags","channel_group_ids":[1]}]}`,
		`{"rules":[{"enabled":true,"keywords":["x"],"target_type":"channels","channel_ids":[]}]}`,
		`{"rules":[{"enabled":true,"keywords":["x"],"target_type":"channel_tags","channel_tags":[]}]}`,
		`{"rules":[{"enabled":true,"keywords":["x"],"target_type":"routes","channel_ids":[],"group_codes":[]}]}`,
		`{"rules":[{"enabled":true,"keywords":["x"],"target_type":"routes","channel_ids":[1],"channel_tags":["tag-a"]}]}`,
		`{"rules":[{"enabled":true,"keywords":["x"],"target_type":"unknown"}]}`,
	}
	for _, raw := range tests {
		_, err := ParseSensitiveRulesJSONString(raw)
		require.Error(t, err, raw)
	}
}

func TestParseSensitiveRulesJSONStringAllowsDisabledExplicitRuleWithoutTargets(t *testing.T) {
	rules, err := ParseSensitiveRulesJSONString(`{"rules":[{"enabled":false,"keywords":["x"],"target_type":"channel_tags","channel_tags":[]}]}`)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, SensitiveRuleTargetChannelTags, rules[0].TargetType)

	rules, err = ParseSensitiveRulesJSONString(`{"rules":[{"enabled":false,"keywords":["x"],"target_type":"routes"}]}`)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, SensitiveRuleTargetRoutes, rules[0].TargetType)
}

func TestResolveSensitiveRuleTargetsKeepsLegacyGlobalChannels(t *testing.T) {
	oldChannelIds := SensitiveRuleChannelIds
	defer func() { SensitiveRuleChannelIds = oldChannelIds }()
	SensitiveRuleChannelIds = []int{7, 3, 7}

	legacy := ResolveSensitiveRuleTargets(SensitiveRule{})
	assert.Equal(t, []int{3, 7}, legacy.ChannelIds)
	assert.Empty(t, legacy.ChannelTags)

	channels := ResolveSensitiveRuleTargets(SensitiveRule{
		TargetType: SensitiveRuleTargetChannels,
		ChannelIds: []int{11, 9, 11},
	})
	assert.Equal(t, []int{9, 11}, channels.ChannelIds)
	assert.Empty(t, channels.ChannelTags)

	tags := ResolveSensitiveRuleTargets(SensitiveRule{
		TargetType:  SensitiveRuleTargetChannelTags,
		ChannelTags: []string{" zeta ", "alpha", "zeta"},
	})
	assert.Empty(t, tags.ChannelIds)
	assert.Equal(t, []string{"alpha", "zeta"}, tags.ChannelTags)

	routes := ResolveSensitiveRuleTargets(SensitiveRule{
		TargetType: SensitiveRuleTargetRoutes,
		ChannelIds: []int{11, 9, 11},
		GroupCodes: []string{" zeta ", "alpha", "zeta"},
	})
	assert.Equal(t, []int{9, 11}, routes.ChannelIds)
	assert.Empty(t, routes.ChannelTags)
	assert.Equal(t, []string{"alpha", "zeta"}, routes.GroupCodes)
}

func TestParseSensitiveRulesJSONStringKeepsGroupOnlyRules(t *testing.T) {
	raw := `{
		"rules": [
			{
				"id": "",
				"name": "",
				"enabled": true,
				"action": "block",
				"scope": "request",
				"keywords": [],
				"group_refs": [" Sensitive Group "]
			}
		]
	}`

	rules, err := ParseSensitiveRulesJSONString(raw)
	require.NoError(t, err)
	require.Len(t, rules, 1)

	assert.Equal(t, "sensitive group", rules[0].ID)
	assert.Equal(t, "Sensitive Group", rules[0].Name)
	assert.Equal(t, []string{"Sensitive Group"}, rules[0].GroupRefs)
	assert.Empty(t, rules[0].Keywords)
}

func TestGetEffectiveSensitiveRulesFallsBackToLegacyWords(t *testing.T) {
	oldRules := SensitiveRules
	oldConfigured := SensitiveRulesConfigured
	oldWords := SensitiveWords
	defer func() {
		SensitiveRules = oldRules
		SensitiveRulesConfigured = oldConfigured
		SensitiveWords = oldWords
	}()

	SensitiveRules = nil
	SensitiveRulesConfigured = false
	SensitiveWords = []string{"legacy", "词"}

	rules := GetEffectiveSensitiveRules()
	require.Len(t, rules, 1)
	assert.Equal(t, "legacy-sensitive-words", rules[0].ID)
	assert.Equal(t, SensitiveRuleActionBlock, rules[0].Action)
	assert.Equal(t, SensitiveRuleScopeRequest, rules[0].Scope)
	assert.Equal(t, []string{"legacy", "词"}, rules[0].Keywords)
}

func TestGetEffectiveSensitiveRulesDoesNotFallbackAfterRulesConfigured(t *testing.T) {
	oldRules := SensitiveRules
	oldConfigured := SensitiveRulesConfigured
	oldWords := SensitiveWords
	defer func() {
		SensitiveRules = oldRules
		SensitiveRulesConfigured = oldConfigured
		SensitiveWords = oldWords
	}()

	SensitiveWords = []string{"legacy"}

	err := UpdateSensitiveRulesByJSONString(`{"rules":[]}`)
	require.NoError(t, err)

	assert.True(t, SensitiveRulesConfigured)
	assert.Empty(t, GetEffectiveSensitiveRules())
}

func TestGetEffectiveSensitiveRulesByScope(t *testing.T) {
	oldRules := SensitiveRules
	oldConfigured := SensitiveRulesConfigured
	oldWords := SensitiveWords
	defer func() {
		SensitiveRules = oldRules
		SensitiveRulesConfigured = oldConfigured
		SensitiveWords = oldWords
	}()

	SensitiveWords = nil
	SensitiveRulesConfigured = false
	SensitiveRules = []SensitiveRule{
		{
			ID:       "request",
			Name:     "Request",
			Enabled:  true,
			Action:   SensitiveRuleActionBlock,
			Scope:    SensitiveRuleScopeRequest,
			Keywords: []string{"request-only"},
		},
		{
			ID:       "response",
			Name:     "Response",
			Enabled:  true,
			Action:   SensitiveRuleActionBlock,
			Scope:    SensitiveRuleScopeResponse,
			Keywords: []string{"response-only"},
		},
		{
			ID:       "both",
			Name:     "Both",
			Enabled:  true,
			Action:   SensitiveRuleActionBlock,
			Scope:    SensitiveRuleScopeBoth,
			Keywords: []string{"both"},
		},
	}

	requestRules := GetEffectiveSensitiveRulesByScope(SensitiveRuleScopeRequest)
	require.Len(t, requestRules, 2)
	assert.Equal(t, []string{"request", "both"}, []string{requestRules[0].ID, requestRules[1].ID})

	responseRules := GetEffectiveSensitiveRulesByScope(SensitiveRuleScopeResponse)
	require.Len(t, responseRules, 2)
	assert.Equal(t, []string{"response", "both"}, []string{responseRules[0].ID, responseRules[1].ID})
}

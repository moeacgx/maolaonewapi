package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
	"github.com/tidwall/gjson"
)

const (
	cyberPolicyConversationCacheNamespace = "new-api:cyber_policy_conversation:v1"
	defaultCyberPolicyConversationTTL     = 720 * time.Hour
)

var (
	cyberPolicyConversationCacheOnce sync.Once
	cyberPolicyConversationCache     *cachex.HybridCache[bool]
)

func getCyberPolicyConversationCache() *cachex.HybridCache[bool] {
	cyberPolicyConversationCacheOnce.Do(func() {
		cyberPolicyConversationCache = cachex.NewHybridCache[bool](cachex.HybridCacheConfig[bool]{
			Namespace: cachex.Namespace(cyberPolicyConversationCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[bool]{},
			Memory: func() *hot.HotCache[string, bool] {
				return hot.NewHotCache[string, bool](hot.LRU, 100_000).
					WithTTL(defaultCyberPolicyConversationTTL).
					WithJanitor().
					Build()
			},
		})
	})
	return cyberPolicyConversationCache
}

// IsCyberPolicyConversationBlocked 只使用客户端提供的稳定会话标识。
// 没有稳定标识时返回 false，不能用请求 ID 或正文近似值扩大拦截范围。
func IsCyberPolicyConversationBlocked(c *gin.Context) (bool, error) {
	key := cyberPolicyConversationKey(c)
	if key == "" {
		return false, nil
	}
	blocked, found, err := getCyberPolicyConversationCache().Get(key)
	return found && blocked, err
}

// MarkCyberPolicyConversationBlocked 在上游明确返回 cyber_policy 后标记当前会话。
// ttlHours 与官方风控滚动窗口一致；非法值回退到默认 30 天。
func MarkCyberPolicyConversationBlocked(c *gin.Context, ttlHours int) bool {
	key := cyberPolicyConversationKey(c)
	if key == "" {
		return false
	}
	ttl := time.Duration(ttlHours) * time.Hour
	if ttl <= 0 {
		ttl = defaultCyberPolicyConversationTTL
	}
	if err := getCyberPolicyConversationCache().SetWithTTL(key, true, ttl); err != nil {
		common.SysError("cyber_policy 对话拦截标记写入失败: " + err.Error())
		return false
	}
	return true
}

func cyberPolicyConversationKey(c *gin.Context) string {
	if c == nil {
		return ""
	}
	userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	if userId <= 0 {
		return ""
	}
	source, identity := stableCyberPolicyConversationIdentity(c)
	if identity == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s", userId, source, identity)))
	return hex.EncodeToString(digest[:])
}

func stableCyberPolicyConversationIdentity(c *gin.Context) (string, string) {
	if c == nil || c.Request == nil {
		return "", ""
	}
	for _, name := range stableOpenAISessionHeaderNames {
		if value := strings.TrimSpace(c.Request.Header.Get(name)); value != "" {
			return "header:" + name, value
		}
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return "", ""
	}
	body, err := storage.Bytes()
	if err != nil || len(body) == 0 {
		return "", ""
	}
	for _, path := range []string{
		"prompt_cache_key",
		"conversation_id",
		"conversation",
		"conversation.id",
		"metadata.session_id",
		"metadata.conversation_id",
	} {
		if value := stableCyberPolicyJSONScalar(body, path); value != "" {
			return "json:" + path, value
		}
	}
	if value := stableCyberPolicyJSONScalar(body, "metadata.user_id"); value != "" {
		if sessionId := extractClaudeSessionID(value); sessionId != "" {
			return "json:metadata.user_id:claude_session", sessionId
		}
	}
	return "", ""
}

func stableCyberPolicyJSONScalar(body []byte, path string) string {
	value := gjson.GetBytes(body, path)
	if !value.Exists() {
		return ""
	}
	switch value.Type {
	case gjson.String, gjson.Number:
		return strings.TrimSpace(value.String())
	default:
		return ""
	}
}

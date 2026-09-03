package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	// SecurityPolicySessionBlockedCode 是跨上游策略来源的统一会话屏蔽错误码。
	SecurityPolicySessionBlockedCode = "session_blocked_by_security_policy"
	// CyberSessionBlockedCode 保留为源码兼容别名；其值已不再绑定单一 cyber 来源。
	CyberSessionBlockedCode            = SecurityPolicySessionBlockedCode
	LegacyCyberSessionBlockedCode      = "session_blocked_by_cyber_policy"
	CyberSessionBlockDefaultTTLSeconds = 3600
	CyberSessionBlockMaxTTLSeconds     = 31536000
	cyberSessionBlockContextKey        = "cyber_session_block_key"
	cyberSessionBlockedConnectionKey   = "cyber_session_blocked_current_connection"
	cyberSessionBlockRedisKeyPrefix    = "cyber_session_block:"
	cyberSessionBlockRedisValue        = "1"
)

var cyberSessionIdentityHeaderNames = []string{
	"x-claude-code-session-id",
	"x-codex-session-id",
	"x-session-affinity",
	"x-session-id",
	"x-opencode-session",
	"x-conversation-id",
	"conversation_id",
	"session_id",
}

type cyberSessionMemoryBlockStore struct {
	mu      sync.Mutex
	entries map[string]time.Time
}

var cyberSessionBlocks cyberSessionMemoryBlockStore

func normalizeCyberSessionBlockTTLSeconds(seconds int) int {
	if seconds <= 0 {
		return CyberSessionBlockDefaultTTLSeconds
	}
	if seconds > CyberSessionBlockMaxTTLSeconds {
		return CyberSessionBlockMaxTTLSeconds
	}
	return seconds
}

func validateCyberSessionBlockConfig(ttlSeconds int) error {
	if ttlSeconds < 1 || ttlSeconds > CyberSessionBlockMaxTTLSeconds {
		return errors.New("cyber_policy 会话屏蔽 TTL 配置无效")
	}
	return nil
}

func resolveCyberSessionIdentity(c *gin.Context, body []byte) string {
	if c != nil && c.Request != nil {
		for _, name := range cyberSessionIdentityHeaderNames {
			if value := strings.TrimSpace(c.Request.Header.Get(name)); value != "" {
				return value
			}
		}
	}
	if len(body) == 0 {
		return ""
	}
	value := gjson.GetBytes(body, "prompt_cache_key")
	if !value.Exists() || value.Type == gjson.Null {
		return ""
	}
	switch value.Type {
	case gjson.String, gjson.Number, gjson.True, gjson.False:
		return strings.TrimSpace(value.String())
	default:
		return ""
	}
}

func CyberSessionBlockKey(c *gin.Context, body []byte) string {
	identity := resolveCyberSessionIdentity(c, body)
	if identity == "" {
		return ""
	}
	tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	if tokenID <= 0 && c != nil {
		tokenID = c.GetInt(string(constant.ContextKeyTokenId))
	}
	if tokenID <= 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("api_key:%d:%s", tokenID, identity)))
	return cyberSessionBlockRedisKeyPrefix + fmt.Sprintf("%x", sum[:])
}

func CacheCyberSessionBlockKey(c *gin.Context, body []byte) string {
	if c == nil {
		return ""
	}
	key := CyberSessionBlockKey(c, body)
	if key != "" {
		c.Set(cyberSessionBlockContextKey, key)
	}
	return key
}

func cachedCyberSessionBlockKey(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if value := strings.TrimSpace(c.GetString(cyberSessionBlockContextKey)); value != "" {
		return value
	}
	return CyberSessionBlockKey(c, nil)
}

func IsCyberSessionBlocked(c *gin.Context, cfg *PromptAuditConfig, body []byte) bool {
	if cfg == nil || !cfg.CyberSessionBlockEnabled {
		return false
	}
	if cfg.PolicyActionSources != nil && len(cfg.PolicyActionSources) == 0 {
		return false
	}
	key := CacheCyberSessionBlockKey(c, body)
	if key == "" {
		return false
	}
	return isCyberSessionBlockKeyBlocked(c, key)
}

func MarkCyberSessionBlocked(c *gin.Context, cfg *PromptAuditConfig) bool {
	return MarkSecurityPolicySessionBlocked(c, cfg, upstreamCyberPolicyMatch)
}

func MarkSecurityPolicySessionBlocked(c *gin.Context, cfg *PromptAuditConfig, match upstreamPolicyMatch) bool {
	if cfg == nil || !cfg.CyberSessionBlockEnabled || !promptAuditPolicyActionEnabled(cfg, policyActionSourceForMatch(match)) {
		return false
	}
	key := cachedCyberSessionBlockKey(c)
	if key == "" {
		return false
	}
	ttl := time.Duration(normalizeCyberSessionBlockTTLSeconds(cfg.CyberSessionBlockTTLSeconds)) * time.Second
	if common.RedisEnabled && common.RDB != nil {
		if err := common.RedisSet(key, cyberSessionBlockRedisValue, ttl); err == nil {
			MarkCyberSessionBlockedThisConnection(c)
			return true
		} else {
			logger.LogWarn(c, "写入 cyber_policy 会话屏蔽 Redis 失败，退回本机内存: "+err.Error())
		}
	}
	cyberSessionBlocks.mark(key, ttl)
	MarkCyberSessionBlockedThisConnection(c)
	return true
}

func MarkCyberSessionBlockedThisConnection(c *gin.Context) {
	if c != nil {
		common.SetContextKey(c, constant.ContextKey(cyberSessionBlockedConnectionKey), true)
	}
}

func IsCyberSessionBlockedThisConnection(c *gin.Context) bool {
	if c == nil {
		return false
	}
	return common.GetContextKeyBool(c, constant.ContextKey(cyberSessionBlockedConnectionKey))
}

func NewSecurityPolicySessionBlockedAPIError(_ *gin.Context) *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		errors.New("当前会话因上游安全策略拒绝已被本地屏蔽，请开启新会话后重试"),
		types.ErrorCode(SecurityPolicySessionBlockedCode),
		http.StatusForbidden,
		types.ErrOptionWithSkipRetry(),
	)
}

// NewCyberSessionBlockedAPIError 保留旧函数名，错误码采用跨来源的统一值。
func NewCyberSessionBlockedAPIError(c *gin.Context) *types.NewAPIError {
	return NewSecurityPolicySessionBlockedAPIError(c)
}

func CyberSessionBlockedFinalClientView(c *gin.Context) (types.OpenAIError, int) {
	apiErr := NewCyberSessionBlockedAPIError(nil)
	clientErr := apiErr.ToOpenAIError()
	if c != nil {
		clientErr.Message = common.MessageWithRequestId(clientErr.Message, c.GetString(common.RequestIdKey))
	}
	return clientErr, apiErr.StatusCode
}

func CyberSessionBlockedOpenAIError(c *gin.Context) types.OpenAIError {
	clientErr, _ := CyberSessionBlockedFinalClientView(c)
	return clientErr
}

func CyberSessionBlockedHTTPStatus(c *gin.Context) int {
	_, status := CyberSessionBlockedFinalClientView(c)
	return status
}

func isCyberSessionBlockKeyBlocked(c *gin.Context, key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	if common.RedisEnabled && common.RDB != nil {
		count, err := common.RDB.Exists(context.Background(), key).Result()
		if err != nil {
			logger.LogWarn(c, "读取 cyber_policy 会话屏蔽 Redis 失败，按 fail-open 处理: "+err.Error())
			return false
		}
		if count > 0 {
			return true
		}
		return cyberSessionBlocks.blocked(key, time.Now())
	}
	return cyberSessionBlocks.blocked(key, time.Now())
}

func (s *cyberSessionMemoryBlockStore) mark(key string, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[string]time.Time)
	}
	now := time.Now()
	s.entries[key] = now.Add(ttl)
	if len(s.entries) > 4096 {
		s.cleanupLocked(now)
	}
}

func (s *cyberSessionMemoryBlockStore) blocked(key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.entries) == 0 {
		return false
	}
	expiresAt, exists := s.entries[key]
	if !exists {
		return false
	}
	if !expiresAt.After(now) {
		delete(s.entries, key)
		return false
	}
	return true
}

func (s *cyberSessionMemoryBlockStore) cleanupLocked(now time.Time) {
	for key, expiresAt := range s.entries {
		if !expiresAt.After(now) {
			delete(s.entries, key)
		}
	}
}

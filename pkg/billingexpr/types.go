package billingexpr

import (
	"crypto/sha256"
	"fmt"

	"github.com/QuantumNous/new-api/common"
)

type RequestInput struct {
	Headers map[string]string
	Body    []byte
}

// TokenParams holds all token dimensions passed into an Expr evaluation.
// Fields beyond P and C are optional — when absent they default to 0,
// which means cache-unaware expressions keep working unchanged.
type TokenParams struct {
	P    float64 `json:"p,omitempty"`     // prompt tokens (text) — auto-excludes sub-categories priced separately
	C    float64 `json:"c,omitempty"`     // completion tokens (text) — auto-excludes sub-categories priced separately
	Len  float64 `json:"len,omitempty"`   // total input context length for tier conditions (non-Claude: raw prompt_tokens; Claude: text + cache read + cache creation)
	CR   float64 `json:"cr,omitempty"`    // cache read (hit) tokens
	CC   float64 `json:"cc,omitempty"`    // cache creation tokens (5-min TTL for Claude, generic for others)
	CC1h float64 `json:"cc1h,omitempty"`  // cache creation tokens — 1-hour TTL (Claude only)
	Img  float64 `json:"img,omitempty"`   // image input tokens
	ImgO float64 `json:"img_o,omitempty"` // image output tokens
	AI   float64 `json:"ai,omitempty"`    // audio input tokens
	AO   float64 `json:"ao,omitempty"`    // audio output tokens
}

// RequestRuleTrace describes one request-dependent multiplier detected at compile time.
type RequestRuleTrace struct {
	Cond       string  `json:"cond"`
	Multiplier float64 `json:"multiplier"`
	Matched    bool    `json:"matched"`
}

// TraceResult holds side-channel info captured while an expression runs.
type TraceResult struct {
	MatchedTier  string             `json:"matched_tier"`
	RequestRules []RequestRuleTrace `json:"request_rules,omitempty"`
	Cost         float64            `json:"cost"`
}

// BillingSnapshot captures billing state at pre-consume time. Expression and
// request fields stay frozen; group-dependent fields are refreshed before an
// auto-group retry and settlement. It is fully serializable and contains no
// compiled program pointers.
type BillingSnapshot struct {
	BillingMode               string  `json:"billing_mode"`
	ModelName                 string  `json:"model_name"`
	ExprString                string  `json:"expr_string"`
	ExprHash                  string  `json:"expr_hash"`
	GroupRatio                float64 `json:"group_ratio"`
	EstimatedPromptTokens     int     `json:"estimated_prompt_tokens"`
	EstimatedCompletionTokens int     `json:"estimated_completion_tokens"`
	EstimatedQuotaBeforeGroup float64 `json:"estimated_quota_before_group"`
	EstimatedQuotaAfterGroup  int     `json:"estimated_quota_after_group"`
	EstimatedTier             string  `json:"estimated_tier"`
	QuotaPerUnit              float64 `json:"quota_per_unit"`
	ExprVersion               int     `json:"expr_version"`
}

// TieredResult holds everything needed after running tiered settlement.
type TieredResult struct {
	ActualQuotaBeforeGroup float64            `json:"actual_quota_before_group"`
	ActualQuotaAfterGroup  int                `json:"actual_quota_after_group"`
	MatchedTier            string             `json:"matched_tier"`
	RequestRules           []RequestRuleTrace `json:"request_rules,omitempty"`
	TokenParams            TokenParams        `json:"token_params,omitempty"`
	CrossedTier            bool               `json:"crossed_tier"`
	// Clamp records an int32 saturation event during quota conversion so the
	// caller can surface it on the consume log for admin auditing. Nil when no
	// clamping occurred. Not serialized: the marker is attached separately via
	// the shared quota-saturation audit path.
	Clamp *common.QuotaClamp `json:"-"`
}

// ExprHashString returns the SHA-256 hex digest of an expression string.
func ExprHashString(expr string) string {
	h := sha256.Sum256([]byte(expr))
	return fmt.Sprintf("%x", h)
}

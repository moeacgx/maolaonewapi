package service

import (
	"context"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// PromptAuditRealtimeFrame 保存渠道分配前已经完成审计或确认不含 JSON 文本的客户端帧。
// MessageType 必须原样保留，避免在后续转发时改变 WebSocket 帧类型。
type PromptAuditRealtimeFrame struct {
	MessageType int
	Payload     []byte
}

// PromptAuditRealtimeFrames 保存渠道分配前按接收顺序缓冲的客户端帧。
// 原始二进制音频可能先于首个 JSON 控制帧到达，因此不能只缓存单帧。
type PromptAuditRealtimeFrames []PromptAuditRealtimeFrame

// IsPromptAuditRealtimeJSONFrame 判断负载是否为 JSON 对象。
// WebSocket 的二进制消息类型不等同于音频；二进制消息也可能携带应被审计的
// Realtime JSON 事件，只有无法解析为 JSON 对象的二进制负载才按原始音频处理。
func IsPromptAuditRealtimeJSONFrame(payload []byte) bool {
	var frame map[string]interface{}
	return common.Unmarshal(payload, &frame) == nil && frame != nil
}

// IsPromptAuditRealtimeControlFrame 要求客户端控制帧是带非空 type 的 JSON
// 对象。未知的新事件类型仍可继续转发，但空对象、数组和标量不能在渠道
// 分配前伪装成“已通过首轮审计”的控制帧。
func IsPromptAuditRealtimeControlFrame(payload []byte) bool {
	var frame map[string]interface{}
	if common.Unmarshal(payload, &frame) != nil || frame == nil {
		return false
	}
	eventType, ok := frame["type"].(string)
	return ok && strings.TrimSpace(eventType) != ""
}

// AuditPromptRealtimeFrame 提取并审核单个 Realtime 客户端帧。
// 返回的 hasText 用于区分无需审计的音频/控制帧与真正的文本帧。
func AuditPromptRealtimeFrame(ctx context.Context, req PromptAuditRequest) (decision PromptAuditDecision, hasText bool, err error) {
	if req.Protocol == "" {
		req.Protocol = "openai_realtime"
	}
	if req.Stage == "" {
		req.Stage = "realtime"
	}
	snapshot, err := ExtractPromptAuditSnapshot(req)
	if errors.Is(err, ErrPromptAuditNoText) {
		return PromptAuditDecision{Allow: true}, false, nil
	}
	if err != nil {
		cfg, _ := GetPromptAuditConfig(ctx)
		if PromptAuditEffectiveMode(cfg) != PromptAuditModeBlocking {
			RecordPromptAuditDropped()
			return PromptAuditDecision{Allow: true}, false, nil
		}
		return PromptAuditDecision{}, false, err
	}
	return AuditPromptSnapshot(ctx, snapshot), true, nil
}

package service

import (
	"encoding/base64"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	requestArchiveJSONVersion        = "json_v1"
	requestArchiveJSONEnvelopeFormat = "request_archive_json_v1"
	requestArchiveBodyEncodingUTF8   = "utf8"
	requestArchiveBodyEncodingBase64 = "base64"
	requestArchiveJSONReadableLimit  = 64 * 1024 * 1024
	requestArchiveJSONExpansionSlack = 64 * 1024
)

type requestArchiveJSONEnvelope struct {
	Format        string                      `json:"format"`
	ArchiveID     string                      `json:"archive_id"`
	AuditEventID  int64                       `json:"audit_event_id,omitempty"`
	TargetID      string                      `json:"target_id"`
	ConfigVersion int64                       `json:"config_version"`
	CreatedAt     int64                       `json:"created_at"`
	ExpiresAt     int64                       `json:"expires_at"`
	Request       requestArchiveJSONRequest   `json:"request"`
	Actor         requestArchiveJSONActor     `json:"actor"`
	Integrity     requestArchiveJSONIntegrity `json:"integrity"`
	BodyEncoding  string                      `json:"body_encoding"`
	Body          string                      `json:"body,omitempty"`
	BodyBase64    []byte                      `json:"body_base64,omitempty"`
}

type requestArchiveJSONRequest struct {
	RequestID   string `json:"request_id,omitempty"`
	Method      string `json:"method,omitempty"`
	Path        string `json:"path,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

type requestArchiveJSONActor struct {
	UserID    int    `json:"user_id,omitempty"`
	Username  string `json:"username,omitempty"`
	UserEmail string `json:"user_email,omitempty"`
	TokenID   int    `json:"token_id,omitempty"`
	TokenName string `json:"token_name,omitempty"`
	GroupID   int    `json:"group_id,omitempty"`
	GroupName string `json:"group_name,omitempty"`
}

type requestArchiveJSONIntegrity struct {
	SHA256   string `json:"sha256"`
	ByteSize int64  `json:"byte_size"`
}

func marshalRequestArchiveJSONPayload(job *model.RequestArchiveJob, body []byte) (string, error) {
	if job == nil || strings.TrimSpace(job.ArchiveId) == "" {
		return "", errors.New("请求归档 JSON 任务身份无效")
	}
	bodyEncoding := requestArchiveBodyEncodingBase64
	if requestArchiveJSONBodyIsReadable(body) {
		bodyEncoding = requestArchiveBodyEncodingUTF8
	}
	envelope := requestArchiveJSONEnvelope{
		Format: requestArchiveJSONEnvelopeFormat, ArchiveID: job.ArchiveId,
		AuditEventID: job.AuditEventId, TargetID: job.TargetId, ConfigVersion: job.ConfigVersion,
		CreatedAt: job.CreatedAt, ExpiresAt: job.ExpiresAt,
		Request: requestArchiveJSONRequest{
			RequestID: job.RequestId, Method: job.Method, Path: job.Path, ContentType: job.ContentType,
		},
		Actor: requestArchiveJSONActor{
			UserID: job.UserId, Username: job.Username, UserEmail: job.UserEmail,
			TokenID: job.TokenId, TokenName: job.TokenName, GroupID: job.GroupId, GroupName: job.GroupName,
		},
		Integrity:    requestArchiveJSONIntegrity{SHA256: job.SHA256, ByteSize: job.ByteSize},
		BodyEncoding: bodyEncoding,
	}
	metadata, err := common.Marshal(envelope)
	if err != nil {
		return "", err
	}
	if len(metadata) == 0 || metadata[len(metadata)-1] != '}' {
		return "", errors.New("请求归档 JSON 元数据无效")
	}

	var payload strings.Builder
	payload.Grow(requestArchiveJSONPayloadCapacity(len(metadata), body, bodyEncoding))
	_, _ = payload.Write(metadata[:len(metadata)-1])
	if bodyEncoding == requestArchiveBodyEncodingUTF8 {
		_, _ = payload.WriteString(`,"body":`)
		if err := common.WriteJsonStringBytes(&payload, body); err != nil {
			return "", err
		}
	} else {
		_, _ = payload.WriteString(`,"body_base64":"`)
		encoder := base64.NewEncoder(base64.StdEncoding, &payload)
		if _, err := encoder.Write(body); err != nil {
			return "", err
		}
		if err := encoder.Close(); err != nil {
			return "", err
		}
		_, _ = payload.WriteString(`"`)
	}
	_ = payload.WriteByte('}')
	return payload.String(), nil
}

func unmarshalRequestArchiveJSONPayload(job *model.RequestArchiveJob) ([]byte, error) {
	if job == nil || job.RequestCiphertext == "" {
		return nil, errors.New("请求归档 JSON 载荷为空")
	}
	var envelope requestArchiveJSONEnvelope
	if err := common.UnmarshalJsonStr(string(job.RequestCiphertext), &envelope); err != nil {
		return nil, errors.New("请求归档 JSON 载荷无效")
	}
	if envelope.Format != requestArchiveJSONEnvelopeFormat || envelope.ArchiveID != job.ArchiveId ||
		envelope.AuditEventID != job.AuditEventId || envelope.TargetID != job.TargetId ||
		envelope.ConfigVersion != job.ConfigVersion || envelope.CreatedAt != job.CreatedAt ||
		envelope.ExpiresAt != job.ExpiresAt || envelope.Request.RequestID != job.RequestId ||
		envelope.Request.Method != job.Method || envelope.Request.Path != job.Path ||
		envelope.Request.ContentType != job.ContentType || envelope.Actor.UserID != job.UserId ||
		envelope.Actor.Username != job.Username || envelope.Actor.UserEmail != job.UserEmail ||
		envelope.Actor.TokenID != job.TokenId || envelope.Actor.TokenName != job.TokenName ||
		envelope.Actor.GroupID != job.GroupId || envelope.Actor.GroupName != job.GroupName ||
		envelope.Integrity.ByteSize != job.ByteSize ||
		!strings.EqualFold(strings.TrimSpace(envelope.Integrity.SHA256), strings.TrimSpace(job.SHA256)) {
		return nil, errors.New("请求归档 JSON 元数据校验失败")
	}
	var body []byte
	switch envelope.BodyEncoding {
	case requestArchiveBodyEncodingUTF8:
		if len(envelope.BodyBase64) != 0 {
			return nil, errors.New("请求归档 JSON 正文编码冲突")
		}
		body = []byte(envelope.Body)
	case requestArchiveBodyEncodingBase64:
		if envelope.Body != "" {
			return nil, errors.New("请求归档 JSON 正文编码冲突")
		}
		body = envelope.BodyBase64
	default:
		return nil, errors.New("请求归档 JSON 正文编码无效")
	}
	digest, err := requestArchivePlaintextDigest(job, requestArchiveJSONVersion, body)
	if err != nil || int64(len(body)) != job.ByteSize ||
		!strings.EqualFold(digest, strings.TrimSpace(job.SHA256)) {
		return nil, errors.New("请求归档 JSON 正文校验失败")
	}
	return body, nil
}

func requestArchiveJSONBodyIsReadable(body []byte) bool {
	if len(body) > requestArchiveJSONReadableLimit || !utf8.Valid(body) {
		return false
	}
	remaining := body
	for len(remaining) > 0 {
		value, size := utf8.DecodeRune(remaining)
		if unicode.IsControl(value) && value != '\t' && value != '\n' && value != '\r' {
			return false
		}
		remaining = remaining[size:]
	}
	if len(body) == 0 {
		return true
	}
	// 转义结果明显大于 Base64 时，字符串已经不适合直接管理，也会放大数据库载荷。
	return requestArchiveJSONStringEncodedLen(body) <=
		base64.StdEncoding.EncodedLen(len(body))+requestArchiveJSONExpansionSlack
}

func requestArchiveJSONStringEncodedLen(body []byte) int {
	encodedSize := len(body) + 2
	for _, value := range body {
		switch value {
		case '\\', '"', '\b', '\f', '\n', '\r', '\t':
			encodedSize++
		default:
			if value < 0x20 {
				encodedSize += 5
			}
		}
	}
	return encodedSize
}

func requestArchiveJSONPayloadCapacity(metadataSize int, body []byte, bodyEncoding string) int {
	encodedSize := base64.StdEncoding.EncodedLen(len(body))
	if bodyEncoding == requestArchiveBodyEncodingUTF8 {
		encodedSize = requestArchiveJSONStringEncodedLen(body)
	}
	return metadataSize + encodedSize + 32
}

func requestArchiveJSONMemoryWeight(bodySize int64) int64 {
	if bodySize < 0 {
		bodySize = 0
	}
	// 队列序列化和 Worker 校验共用预算。四倍正文加固定余量覆盖最终 JSON、
	// Base64/字符串解码以及数据库驱动在扫描 TEXT 时可能保留的短暂副本。
	weight := bodySize*4 + 4*1024*1024
	if weight > requestArchiveEncryptionMemoryBudget {
		return requestArchiveEncryptionMemoryBudget
	}
	if weight < 1 {
		return 1
	}
	return weight
}

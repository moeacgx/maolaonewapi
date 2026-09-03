package types

import (
	"regexp"
	"strconv"
)

// upstreamReturnedHTTPStatusPattern 匹配上游网关在中继改写传输状态时使用的
// 明确状态表达（例如外层 HTTP 400、内容中包含上游 HTTP 403）。
// 不匹配通用 status_code/status 字段，避免把本地错误误判为上游响应。
var upstreamReturnedHTTPStatusPattern = regexp.MustCompile(`(?i)\bupstream\s+returned\s+HTTP\s+([1-5][0-9]{2})\b`)

func embeddedUpstreamHTTPStatus(message string) (int, bool) {
	match := upstreamReturnedHTTPStatusPattern.FindStringSubmatch(message)
	if len(match) != 2 {
		return 0, false
	}
	statusCode, err := strconv.Atoi(match[1])
	if err != nil || statusCode < 100 || statusCode > 599 {
		return 0, false
	}
	return statusCode, true
}

// EmbeddedUpstreamHTTPStatus 返回错误内容中明确声明的上游 HTTP 状态码。
func EmbeddedUpstreamHTTPStatus(message string) (int, bool) {
	return embeddedUpstreamHTTPStatus(message)
}

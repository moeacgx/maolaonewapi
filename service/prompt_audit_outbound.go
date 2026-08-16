package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// newPromptAuditSecureHTTPClient 为 Guard 节点提供独立传输层，不继承环境代理、不跟随重定向，
// 并在拨号前固定已经校验的 DNS 结果，防止请求落到云元数据或 link-local 地址。
func newPromptAuditSecureHTTPClient(endpoint PromptAuditEndpoint) (*http.Client, error) {
	if _, err := NormalizePromptAuditBaseURL(endpoint.BaseUrl); err != nil {
		return nil, err
	}
	timeout := time.Duration(endpoint.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = PromptAuditDefaultTimeoutMs * time.Millisecond
	}
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   16,
		MaxConnsPerHost:       16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.New("Guard 节点拨号地址无效")
		}
		if isForbiddenPromptAuditHostname(host) {
			return nil, errors.New("Guard 节点不能指向云元数据或 link-local 地址")
		}
		resolved, err := resolvePromptAuditHost(ctx, host)
		if err != nil {
			return nil, err
		}
		// 所有 DNS 结果都必须通过校验；混入危险地址时整体拒绝，避免轮询或重绑定绕过。
		for _, address := range resolved {
			if isForbiddenPromptAuditIP(address.IP) {
				return nil, errors.New("Guard 节点 DNS 结果包含云元数据或 link-local 地址")
			}
		}
		if len(resolved) == 0 {
			return nil, errors.New("Guard 节点 DNS 未返回可用地址")
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(resolved[0].IP.String(), port))
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

func resolvePromptAuditHost(ctx context.Context, host string) ([]net.IPAddr, error) {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if ip := net.ParseIP(host); ip != nil {
		return []net.IPAddr{{IP: ip}}, nil
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("Guard 节点 DNS 解析失败: %w", err)
	}
	return addresses, nil
}

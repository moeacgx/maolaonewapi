package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const promptAuditMaxGuardResponseBytes int64 = 256 * 1024

type promptAuditScannerDefinition struct {
	label string
}

var promptAuditScannerCatalog = map[string]promptAuditScannerDefinition{
	"violent":                       {label: "Violent"},
	"non_violent_illegal_acts":      {label: "Non-violent Illegal Acts"},
	"sexual_content_or_sexual_acts": {label: "Sexual Content or Sexual Acts"},
	"pii":                           {label: "PII"},
	"suicide_and_self_harm":         {label: "Suicide & Self-Harm"},
	"unethical_acts":                {label: "Unethical Acts"},
	"politically_sensitive_topics":  {label: "Politically Sensitive Topics"},
	"copyright_violation":           {label: "Copyright Violation"},
	"jailbreak":                     {label: "Jailbreak"},
}

var promptAuditCategoryAliases = map[string]string{
	"violent": "violent", "violence": "violent",
	"non violent illegal acts":      "non_violent_illegal_acts",
	"sexual content or sexual acts": "sexual_content_or_sexual_acts", "sexual": "sexual_content_or_sexual_acts",
	"pii": "pii", "personal identifying information": "pii", "personal identifiable information": "pii",
	"suicide self harm": "suicide_and_self_harm", "suicide and self harm": "suicide_and_self_harm",
	"unethical acts": "unethical_acts", "unethical": "unethical_acts",
	"politically sensitive topics": "politically_sensitive_topics", "political": "politically_sensitive_topics",
	"copyright violation": "copyright_violation", "copyright": "copyright_violation",
	"jailbreak": "jailbreak", "prompt injection": "jailbreak",
}

// NormalizePromptAuditCategory 将 Qwen3Guard 的类别别名归一为稳定的九类风险 ID。
func NormalizePromptAuditCategory(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.NewReplacer("_", " ", "&", " and ", "/", " ", "-", " ", "–", " ", "—", " ").Replace(normalized)
	normalized = strings.Join(strings.Fields(normalized), " ")
	if canonical, ok := promptAuditCategoryAliases[normalized]; ok {
		return canonical
	}
	return strings.ReplaceAll(normalized, " ", "_")
}

// ParseQwen3GuardResponse 严格要求两行 Safety/Categories 响应，拒绝多余说明或结构缺失。
func ParseQwen3GuardResponse(content string, enabledScanners []string) (*PromptAuditResult, error) {
	lines := make([]string, 0, 2)
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) != 2 {
		return nil, &PromptGuardError{Code: PromptGuardInvalidResponseCode}
	}
	var safety, categoryLine string
	var seenSafety, seenCategories bool
	for _, line := range lines {
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "safety:"):
			if seenSafety {
				return nil, &PromptGuardError{Code: PromptGuardInvalidResponseCode}
			}
			seenSafety = true
			safety = strings.TrimSpace(line[len("safety:"):])
		case strings.HasPrefix(lower, "categories:"):
			if seenCategories {
				return nil, &PromptGuardError{Code: PromptGuardInvalidResponseCode}
			}
			seenCategories = true
			categoryLine = strings.TrimSpace(line[len("categories:"):])
		default:
			return nil, &PromptGuardError{Code: PromptGuardInvalidResponseCode}
		}
	}
	switch strings.ToLower(safety) {
	case "safe":
		safety = "Safe"
	case "controversial":
		safety = "Controversial"
	case "unsafe":
		safety = "Unsafe"
	default:
		return nil, &PromptGuardError{Code: PromptGuardInvalidResponseCode}
	}
	if !seenSafety || !seenCategories || categoryLine == "" {
		return nil, &PromptGuardError{Code: PromptGuardInvalidResponseCode}
	}

	enabled := make(map[string]struct{}, len(enabledScanners))
	for _, scanner := range enabledScanners {
		enabled[NormalizePromptAuditCategory(scanner)] = struct{}{}
	}
	known := map[string]struct{}{}
	unknown := map[string]struct{}{}
	hasCategoryToken := false
	for _, raw := range strings.Split(categoryLine, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		hasCategoryToken = true
		if strings.EqualFold(raw, "none") || strings.EqualFold(raw, "n/a") {
			continue
		}
		category := NormalizePromptAuditCategory(raw)
		if _, ok := promptAuditScannerCatalog[category]; ok {
			known[category] = struct{}{}
		} else {
			unknown[promptAuditUnknownCategoryID(category)] = struct{}{}
		}
	}
	if !hasCategoryToken {
		return nil, &PromptGuardError{Code: PromptGuardInvalidResponseCode}
	}
	knownList := orderedPromptAuditKeys(known)
	unknownList := sortedPromptAuditKeys(unknown)
	matched := make([]string, 0, len(knownList))
	for _, category := range knownList {
		if _, ok := enabled[category]; ok {
			matched = append(matched, category)
		}
	}

	result := &PromptAuditResult{
		Decision: "pass", RiskLevel: "low", Action: "Allow", Safety: safety,
		Categories: knownList, MatchedScanners: matched, UnknownCategories: unknownList,
		ScannerScores: map[string]float64{}, ScannerEvidence: map[string]string{},
		ScannerVersion: "qwen3guard",
	}
	score := 0.0
	if safety == "Controversial" {
		score = 0.5
		result.Decision, result.RiskLevel, result.Action = "flag", "medium", "Warn"
	}
	if safety == "Unsafe" {
		score = 1
		if len(matched) > 0 || len(unknownList) > 0 || len(knownList) == 0 {
			result.Decision, result.RiskLevel, result.Action = "critical", "critical", "Block"
		} else {
			result.Decision, result.RiskLevel, result.Action = "flag", "high", "Warn"
		}
	}
	for _, category := range matched {
		result.ScannerScores[category] = score
		result.ScannerEvidence[category] = promptAuditScannerCatalog[category].label
		if safety == "Controversial" && isElevatedPromptAuditCategory(category) {
			result.Decision, result.RiskLevel, result.Action = "critical", "critical", "Block"
		}
	}
	return result, nil
}

func promptAuditUnknownCategoryID(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(strings.ToLower(value))))
	return fmt.Sprintf("unknown:%x", digest[:8])
}

func isElevatedPromptAuditCategory(category string) bool {
	return category == "jailbreak" || category == "pii" || category == "suicide_and_self_harm"
}

// SplitPromptAuditRunes 按 Unicode 字符分片，并确保优先文本始终排在普通上下文之前。
func SplitPromptAuditRunes(value string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	segments := strings.Split(value, promptAuditPrioritySeparator)
	chunks := make([]string, 0, len(segments))
	for _, segment := range segments {
		runes := []rune(segment)
		for start := 0; start < len(runes); start += limit {
			end := start + limit
			if end > len(runes) {
				end = len(runes)
			}
			chunks = append(chunks, string(runes[start:end]))
		}
	}
	return chunks
}

func aggregatePromptAuditResults(results []*PromptAuditResult, latency time.Duration) (*PromptAuditResult, error) {
	if len(results) == 0 {
		return nil, errors.New("Guard 未返回完整结果")
	}
	aggregated := &PromptAuditResult{
		Decision: "pass", RiskLevel: "low", Action: "Allow",
		Categories: []string{}, MatchedScanners: []string{}, UnknownCategories: []string{},
		ScannerScores: map[string]float64{}, ScannerEvidence: map[string]string{},
		ChunkTotal: len(results), LatencyMs: latency.Milliseconds(),
	}
	categories, matched, unknown := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
	for _, result := range results {
		if result == nil {
			return nil, errors.New("Guard 返回了不完整分片结果")
		}
		if promptAuditDecisionSeverity(result.Decision) > promptAuditDecisionSeverity(aggregated.Decision) {
			aggregated.Decision, aggregated.RiskLevel, aggregated.Action = result.Decision, result.RiskLevel, result.Action
			aggregated.Safety, aggregated.GuardEndpointId = result.Safety, result.GuardEndpointId
			aggregated.ScannerVersion = result.ScannerVersion
		}
		if aggregated.GuardEndpointId == "" {
			aggregated.GuardEndpointId, aggregated.ScannerVersion = result.GuardEndpointId, result.ScannerVersion
			aggregated.Safety = result.Safety
		}
		for _, value := range result.Categories {
			categories[value] = struct{}{}
		}
		for _, value := range result.MatchedScanners {
			matched[value] = struct{}{}
		}
		for _, value := range result.UnknownCategories {
			unknown[value] = struct{}{}
		}
		for scanner, score := range result.ScannerScores {
			if score > aggregated.ScannerScores[scanner] {
				aggregated.ScannerScores[scanner] = score
			}
		}
		for scanner, evidence := range result.ScannerEvidence {
			if _, exists := aggregated.ScannerEvidence[scanner]; !exists {
				aggregated.ScannerEvidence[scanner] = trimPromptAuditRunes(evidence, 160, true)
			}
		}
	}
	aggregated.Categories = orderedPromptAuditKeys(categories)
	aggregated.MatchedScanners = orderedPromptAuditKeys(matched)
	aggregated.UnknownCategories = sortedPromptAuditKeys(unknown)
	return aggregated, nil
}

func promptAuditDecisionSeverity(decision string) int {
	switch decision {
	case "critical":
		return 3
	case "flag":
		return 2
	default:
		return 1
	}
}

func sortedPromptAuditKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func orderedPromptAuditKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	remaining := make(map[string]struct{}, len(values))
	for key := range values {
		remaining[key] = struct{}{}
	}
	for _, scanner := range PromptAuditScannerIDs {
		if _, ok := remaining[scanner]; ok {
			result = append(result, scanner)
			delete(remaining, scanner)
		}
	}
	return append(result, sortedPromptAuditKeys(remaining)...)
}

type PromptAuditScanner interface {
	Scan(context.Context, PromptAuditEndpoint, string, []string) (*PromptAuditResult, error)
}

type openAICompatiblePromptAuditScanner struct {
	clients sync.Map
}

func (s *openAICompatiblePromptAuditScanner) Scan(ctx context.Context, endpoint PromptAuditEndpoint, chunk string, scanners []string) (*PromptAuditResult, error) {
	client, err := s.clientFor(endpoint)
	if err != nil {
		return nil, &PromptGuardError{Code: PromptGuardUnavailableCode, Cause: err}
	}
	requestURL, err := PromptAuditChatCompletionsURL(endpoint.BaseUrl)
	if err != nil {
		return nil, &PromptGuardError{Code: PromptGuardUnavailableCode, Cause: err}
	}
	payload := map[string]interface{}{
		"model":       endpoint.Model,
		"messages":    []map[string]string{{"role": "user", "content": chunk}},
		"temperature": 0, "max_tokens": 64, "seed": 42,
	}
	body, err := common.Marshal(payload)
	if err != nil {
		return nil, &PromptGuardError{Code: PromptGuardInvalidResponseCode, Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return nil, &PromptGuardError{Code: PromptGuardUnavailableCode, Cause: err}
	}
	req.Header.Set("Content-Type", "application/json")
	if endpoint.Token != "" {
		req.Header.Set("Authorization", "Bearer "+endpoint.Token)
	}
	resp, err := client.Do(req)
	if err != nil {
		timeout := errors.Is(err, context.DeadlineExceeded)
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			timeout = true
		}
		return nil, &PromptGuardError{Code: PromptGuardUnavailableCode, Retryable: true, Timeout: timeout, Cause: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, &PromptGuardError{
			Code: PromptGuardUnavailableCode, HTTPStatus: resp.StatusCode,
			Retryable: resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError,
		}
	}
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, promptAuditMaxGuardResponseBytes+1))
	if err != nil {
		return nil, &PromptGuardError{Code: PromptGuardUnavailableCode, Retryable: true, Cause: err}
	}
	if int64(len(responseBody)) > promptAuditMaxGuardResponseBytes {
		return nil, &PromptGuardError{Code: PromptGuardInvalidResponseCode}
	}
	content, err := extractPromptAuditOpenAIContent(responseBody)
	if err != nil {
		return nil, &PromptGuardError{Code: PromptGuardInvalidResponseCode, Cause: err}
	}
	result, err := ParseQwen3GuardResponse(content, scanners)
	if err != nil {
		return nil, err
	}
	result.GuardEndpointId = endpoint.Id
	result.ScannerVersion = endpoint.Model
	return result, nil
}

func (s *openAICompatiblePromptAuditScanner) clientFor(endpoint PromptAuditEndpoint) (*http.Client, error) {
	key := fmt.Sprintf("%s|%s|%d", endpoint.Id, endpoint.BaseUrl, endpoint.TimeoutMs)
	if cached, ok := s.clients.Load(key); ok {
		if client, valid := cached.(*http.Client); valid {
			return client, nil
		}
		s.clients.Delete(key)
	}
	client, err := newPromptAuditSecureHTTPClient(endpoint)
	if err != nil {
		return nil, err
	}
	actual, _ := s.clients.LoadOrStore(key, client)
	actualClient, ok := actual.(*http.Client)
	if !ok {
		s.clients.Delete(key)
		return nil, errors.New("Guard HTTP 客户端缓存无效")
	}
	return actualClient, nil
}

func extractPromptAuditOpenAIContent(body []byte) (string, error) {
	var response struct {
		Choices []struct {
			Message struct {
				Content interface{} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := common.Unmarshal(body, &response); err != nil || len(response.Choices) == 0 {
		return "", errors.New("Guard 响应封装无效")
	}
	switch typed := response.Choices[0].Message.Content.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return "", errors.New("Guard 响应内容为空")
		}
		return typed, nil
	case []interface{}:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			object, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if text, ok := object["text"].(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n"), nil
		}
	}
	return "", errors.New("Guard 响应内容无效")
}

type PromptAuditGuardEvaluator struct {
	scanner PromptAuditScanner
	global  chan struct{}

	perNodeLimit int
	nodeMu       sync.Mutex
	nodes        map[string]chan struct{}
}

func NewPromptAuditGuardEvaluator(scanner PromptAuditScanner, globalLimit, perNodeLimit int) *PromptAuditGuardEvaluator {
	if scanner == nil {
		scanner = &openAICompatiblePromptAuditScanner{}
	}
	if globalLimit < 1 {
		globalLimit = 64
	}
	if perNodeLimit < 1 {
		perNodeLimit = 16
	}
	return &PromptAuditGuardEvaluator{
		scanner: scanner, global: make(chan struct{}, globalLimit),
		perNodeLimit: perNodeLimit, nodes: map[string]chan struct{}{},
	}
}

var defaultPromptAuditGuardEvaluator = NewPromptAuditGuardEvaluator(nil, 64, 16)

func EvaluatePromptAuditGuard(ctx context.Context, cfg *PromptAuditConfig, snapshot PromptAuditSnapshot) (*PromptAuditResult, error) {
	return defaultPromptAuditGuardEvaluator.Evaluate(ctx, cfg, snapshot)
}

func (g *PromptAuditGuardEvaluator) Evaluate(ctx context.Context, cfg *PromptAuditConfig, snapshot PromptAuditSnapshot) (*PromptAuditResult, error) {
	if g == nil || g.scanner == nil || cfg == nil {
		return nil, &PromptGuardError{Code: PromptGuardUnavailableCode}
	}
	endpoints := make([]PromptAuditEndpoint, 0, len(cfg.Endpoints))
	for _, endpoint := range cfg.Endpoints {
		if endpoint.Enabled {
			endpoints = append(endpoints, endpoint)
		}
	}
	if len(endpoints) == 0 {
		return nil, &PromptGuardError{Code: PromptGuardUnavailableCode}
	}
	select {
	case g.global <- struct{}{}:
		defer func() { <-g.global }()
	default:
		promptAuditStats.bulkheadFull.Add(1)
		return nil, &PromptGuardError{Code: PromptGuardUnavailableCode}
	}
	// 与 sub2api 的 Guard 语义保持一致：一次同步评估使用首个启用节点
	// 的超时作为总预算，所有 Unicode 分片和节点故障切换共享同一 deadline。
	// 否则长提示词会按“每片×每节点”累积等待，造成阻断请求延迟失控。
	totalTimeout := time.Duration(endpoints[0].TimeoutMs) * time.Millisecond
	if totalTimeout <= 0 {
		totalTimeout = PromptAuditDefaultTimeoutMs * time.Millisecond
	}
	evaluationContext, cancelEvaluation := context.WithTimeout(ctx, totalTimeout)
	defer cancelEvaluation()
	inputLimit := minimumPromptAuditInputLimit(endpoints)
	chunks := SplitPromptAuditRunes(snapshot.ScanText, inputLimit)
	if len(chunks) == 0 {
		return &PromptAuditResult{
			Decision: "pass", RiskLevel: "low", Action: "Allow", Safety: "Safe",
			Categories: []string{}, MatchedScanners: []string{}, UnknownCategories: []string{},
			ScannerScores: map[string]float64{}, ScannerEvidence: map[string]string{},
		}, nil
	}
	started := time.Now()
	results := make([]*PromptAuditResult, 0, len(chunks))
	for _, chunk := range chunks {
		result, err := g.scanChunk(evaluationContext, endpoints, chunk, cfg.Scanners)
		if err != nil {
			return nil, err
		}
		result.ChunkTotal = len(chunks)
		results = append(results, result)
		if result.Action == "Block" {
			break
		}
	}
	aggregated, err := aggregatePromptAuditResults(results, time.Since(started))
	if err != nil {
		return nil, &PromptGuardError{Code: PromptGuardInvalidResponseCode, Cause: err}
	}
	aggregated.ChunkTotal = len(chunks)
	return aggregated, nil
}

func (g *PromptAuditGuardEvaluator) scanChunk(ctx context.Context, endpoints []PromptAuditEndpoint, chunk string, scanners []string) (*PromptAuditResult, error) {
	var lastErr error
	for index, endpoint := range endpoints {
		if err := ctx.Err(); err != nil {
			return nil, &PromptGuardError{Code: PromptGuardUnavailableCode, Retryable: true,
				Timeout: errors.Is(err, context.DeadlineExceeded), Cause: err}
		}
		timeout := time.Duration(endpoint.TimeoutMs) * time.Millisecond
		if timeout <= 0 {
			timeout = PromptAuditDefaultTimeoutMs * time.Millisecond
		}
		endpointContext, cancel := context.WithTimeout(ctx, timeout)
		semaphore := g.nodeSemaphore(endpoint.Id)
		select {
		case semaphore <- struct{}{}:
		case <-endpointContext.Done():
			endpointErr := endpointContext.Err()
			cancel()
			return nil, &PromptGuardError{Code: PromptGuardUnavailableCode, Retryable: true,
				Timeout: errors.Is(endpointErr, context.DeadlineExceeded), Cause: endpointErr}
		default:
			cancel()
			promptAuditStats.bulkheadFull.Add(1)
			lastErr = &PromptGuardError{Code: PromptGuardUnavailableCode, Retryable: true}
			recordPromptAuditEndpointHealth(endpoint, 0, lastErr)
			if index < len(endpoints)-1 {
				promptAuditStats.failovers.Add(1)
			}
			continue
		}
		started := time.Now()
		result, err := callPromptAuditScanner(endpointContext, g.scanner, endpoint, chunk, scanners)
		endpointErr := endpointContext.Err()
		cancel()
		<-semaphore
		if endpointErr != nil {
			result = nil
			err = &PromptGuardError{Code: PromptGuardUnavailableCode, Retryable: true,
				Timeout: errors.Is(endpointErr, context.DeadlineExceeded), Cause: endpointErr}
		}
		if err == nil && result != nil {
			recordPromptAuditEndpointHealth(endpoint, time.Since(started).Milliseconds(), nil)
			return result, nil
		}
		if err == nil {
			err = &PromptGuardError{Code: PromptGuardInvalidResponseCode}
		}
		lastErr = err
		recordPromptAuditEndpointHealth(endpoint, time.Since(started).Milliseconds(), err)
		var guardErr *PromptGuardError
		if errors.As(err, &guardErr) && guardErr.Timeout {
			promptAuditStats.timeouts.Add(1)
		}
		if !errors.As(err, &guardErr) || !guardErr.Retryable {
			return nil, err
		}
		if index < len(endpoints)-1 {
			promptAuditStats.failovers.Add(1)
		}
	}
	if lastErr == nil {
		lastErr = &PromptGuardError{Code: PromptGuardUnavailableCode}
	}
	return nil, lastErr
}

func recordPromptAuditEndpointHealth(endpoint PromptAuditEndpoint, latencyMs int64, err error) {
	result := PromptAuditProbeResult{
		EndpointId: endpoint.Id, Status: "healthy", Ok: true, Healthy: true,
		LatencyMs: latencyMs, CheckedAt: time.Now().Unix(), TokenApplied: endpoint.Token != "",
	}
	if err != nil {
		result.Ok, result.Healthy, result.Status = false, false, "failed"
		result.ErrorCode = promptAuditGuardErrorCode(err)
		var guardErr *PromptGuardError
		if errors.As(err, &guardErr) {
			result.HTTPStatus, result.Retryable = guardErr.HTTPStatus, guardErr.Retryable
		}
	}
	promptAuditEndpointHealth.Store(endpoint.Id, result)
}

func callPromptAuditScanner(ctx context.Context, scanner PromptAuditScanner, endpoint PromptAuditEndpoint, chunk string, scanners []string) (result *PromptAuditResult, err error) {
	defer func() {
		if recover() != nil {
			result = nil
			err = &PromptGuardError{Code: PromptGuardUnavailableCode}
		}
	}()
	return scanner.Scan(ctx, endpoint, chunk, scanners)
}

func (g *PromptAuditGuardEvaluator) nodeSemaphore(id string) chan struct{} {
	g.nodeMu.Lock()
	defer g.nodeMu.Unlock()
	semaphore := g.nodes[id]
	if semaphore == nil {
		semaphore = make(chan struct{}, g.perNodeLimit)
		g.nodes[id] = semaphore
	}
	return semaphore
}

func minimumPromptAuditInputLimit(endpoints []PromptAuditEndpoint) int {
	limit := PromptAuditDefaultInputLimit
	for index, endpoint := range endpoints {
		value := endpoint.InputLimit
		if value <= 0 {
			value = PromptAuditDefaultInputLimit
		}
		if index == 0 || value < limit {
			limit = value
		}
	}
	return limit
}

func promptAuditGuardErrorCode(err error) string {
	var guardErr *PromptGuardError
	if errors.As(err, &guardErr) && guardErr.Code != "" {
		return guardErr.Code
	}
	return PromptGuardUnavailableCode
}

type PromptAuditProbeResult struct {
	EndpointId   string `json:"endpoint_id"`
	Healthy      bool   `json:"healthy"`
	Ok           bool   `json:"ok"`
	Status       string `json:"status"`
	ErrorCode    string `json:"error_code,omitempty"`
	Message      string `json:"message"`
	LatencyMs    int64  `json:"latency_ms"`
	HTTPStatus   int    `json:"http_status"`
	Retryable    bool   `json:"retryable"`
	CheckedAt    int64  `json:"checked_at"`
	TokenApplied bool   `json:"token_applied"`
}

func ProbePromptAuditEndpoint(ctx context.Context, endpoint PromptAuditEndpoint) PromptAuditProbeResult {
	started := time.Now()
	result := PromptAuditProbeResult{
		EndpointId: endpoint.Id, Status: "failed", CheckedAt: started.Unix(), TokenApplied: endpoint.Token != "",
	}
	if endpoint.TimeoutMs <= 0 {
		endpoint.TimeoutMs = PromptAuditDefaultTimeoutMs
	}
	if endpoint.InputLimit <= 0 {
		endpoint.InputLimit = PromptAuditDefaultInputLimit
	}
	scanner := &openAICompatiblePromptAuditScanner{}
	client, err := scanner.clientFor(endpoint)
	if err != nil {
		result.ErrorCode = PromptGuardUnavailableCode
		result.Message = "Guard 节点探测失败"
		result.LatencyMs = time.Since(started).Milliseconds()
		promptAuditEndpointHealth.Store(endpoint.Id, result)
		return result
	}
	modelsURL, err := PromptAuditModelsURL(endpoint.BaseUrl)
	if err != nil {
		result.ErrorCode = PromptGuardUnavailableCode
		result.Message = "Guard 节点探测失败"
		result.LatencyMs = time.Since(started).Milliseconds()
		promptAuditEndpointHealth.Store(endpoint.Id, result)
		return result
	}
	modelsRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		result.ErrorCode = PromptGuardUnavailableCode
		result.Message = "Guard 节点探测失败"
		result.LatencyMs = time.Since(started).Milliseconds()
		promptAuditEndpointHealth.Store(endpoint.Id, result)
		return result
	}
	if endpoint.Token != "" {
		modelsRequest.Header.Set("Authorization", "Bearer "+endpoint.Token)
	}
	modelsResponse, requestErr := client.Do(modelsRequest)
	if requestErr == nil {
		responseBody, readErr := io.ReadAll(io.LimitReader(modelsResponse.Body, promptAuditMaxGuardResponseBytes+1))
		_ = modelsResponse.Body.Close()
		if readErr != nil {
			requestErr = &PromptGuardError{Code: PromptGuardUnavailableCode, Retryable: true, Cause: readErr}
		} else if int64(len(responseBody)) > promptAuditMaxGuardResponseBytes {
			requestErr = &PromptGuardError{Code: PromptGuardInvalidResponseCode}
		} else if modelsResponse.StatusCode >= http.StatusOK && modelsResponse.StatusCode < http.StatusMultipleChoices && promptAuditModelsResponseReady(responseBody, endpoint.Model) {
			result.Ok, result.Healthy, result.Status, result.Message = true, true, "healthy", "Guard 节点模型可用"
			result.HTTPStatus = modelsResponse.StatusCode
			result.LatencyMs = time.Since(started).Milliseconds()
			promptAuditEndpointHealth.Store(endpoint.Id, result)
			return result
		} else if modelsResponse.StatusCode >= http.StatusOK && modelsResponse.StatusCode < http.StatusMultipleChoices {
			// 模型列表可访问但封装或模型名不匹配时，继续用一次真实 Guard
			// 调用确认兼容性；这与上游探测行为一致。
		} else if modelsResponse.StatusCode != http.StatusNotFound && modelsResponse.StatusCode != http.StatusMethodNotAllowed {
			requestErr = &PromptGuardError{
				Code: PromptGuardUnavailableCode, HTTPStatus: modelsResponse.StatusCode,
				Retryable: modelsResponse.StatusCode == http.StatusTooManyRequests || modelsResponse.StatusCode >= http.StatusInternalServerError,
			}
		}
	}
	if requestErr != nil {
		// 只有模型列表不存在/不支持或可解析但未列出模型时才回落到
		// chat/completions；网络、认证和超大响应错误直接报告探测失败。
		var guardErr *PromptGuardError
		if errors.As(requestErr, &guardErr) && guardErr.Code == PromptGuardInvalidResponseCode {
			result.ErrorCode, result.Message = PromptGuardInvalidResponseCode, "Guard 节点探测响应无效"
			result.LatencyMs = time.Since(started).Milliseconds()
			promptAuditEndpointHealth.Store(endpoint.Id, result)
			return result
		}
		if modelsResponse == nil || (modelsResponse.StatusCode != http.StatusNotFound && modelsResponse.StatusCode != http.StatusMethodNotAllowed) {
			result.ErrorCode = PromptGuardUnavailableCode
			result.Message = "Guard 节点探测失败"
			if errors.As(requestErr, &guardErr) {
				result.HTTPStatus, result.Retryable = guardErr.HTTPStatus, guardErr.Retryable
			}
			result.LatencyMs = time.Since(started).Milliseconds()
			promptAuditEndpointHealth.Store(endpoint.Id, result)
			return result
		}
	}

	scanResult, scanErr := scanner.Scan(ctx, endpoint, "Hello", PromptAuditScannerIDs)
	result.LatencyMs = time.Since(started).Milliseconds()
	if scanErr == nil && scanResult != nil {
		result.Ok, result.Healthy, result.Status, result.Message = true, true, "healthy", "Guard 节点响应有效"
		// Chat fallback 只在上游返回 2xx 且严格解析成功时才会返回结果。
		// 探测接口对成功结果使用稳定的 200，避免内部扫描器未暴露
		// 原始响应状态时向管理端返回无意义的 0。
		result.HTTPStatus = http.StatusOK
		promptAuditEndpointHealth.Store(endpoint.Id, result)
		return result
	}
	result.ErrorCode = promptAuditGuardErrorCode(scanErr)
	result.Message = "Guard 节点探测失败"
	var guardErr *PromptGuardError
	if errors.As(scanErr, &guardErr) {
		result.HTTPStatus, result.Retryable = guardErr.HTTPStatus, guardErr.Retryable
	}
	promptAuditEndpointHealth.Store(endpoint.Id, result)
	return result
}

func promptAuditModelsResponseReady(body []byte, modelName string) bool {
	var response struct {
		Data []struct {
			Id string `json:"id"`
		} `json:"data"`
	}
	if err := common.Unmarshal(body, &response); err != nil || response.Data == nil {
		return false
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return true
	}
	for _, item := range response.Data {
		if strings.TrimSpace(item.Id) == modelName {
			return true
		}
	}
	return false
}

package common

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	ErrorMessageReplacementModeContains = "contains"
	ErrorMessageReplacementModeExact    = "exact"
	ErrorMessageReplacementModeRegex    = "regex"

	maxErrorMessageReplacementRules = 100
	maxErrorMessageMatchesPerRule   = 64
	maxErrorMessageMatchLength      = 4096
	maxErrorMessageReplaceLength    = 4096
)

type ErrorMessageReplacementRule struct {
	Match             string   `json:"match,omitempty"`
	Matches           []string `json:"matches,omitempty"`
	Mode              string   `json:"mode"`
	StatusCode        *int     `json:"status_code,omitempty"`
	Replace           string   `json:"replace"`
	ReplaceStatusCode *int     `json:"replace_status_code,omitempty"`
}

type compiledErrorMessageReplacementRule struct {
	ErrorMessageReplacementRule
	regularExpressions []*regexp.Regexp
}

var errorMessageReplacementState = struct {
	sync.RWMutex
	rules []compiledErrorMessageReplacementRule
}{}

// ValidateErrorMessageReplacementRules 校验客户端错误文案替换规则。
// 规则只用于最终响应序列化，不参与上游错误识别、渠道禁用或重试判断。
func ValidateErrorMessageReplacementRules(value string) error {
	_, err := parseErrorMessageReplacementRules(value)
	return err
}

func UpdateErrorMessageReplacementRules(value string) error {
	rules, err := parseErrorMessageReplacementRules(value)
	if err != nil {
		return err
	}
	errorMessageReplacementState.Lock()
	errorMessageReplacementState.rules = rules
	errorMessageReplacementState.Unlock()
	return nil
}

// ReplaceClientErrorCandidates 按状态码和文案共同匹配客户端错误规则。
// 返回的状态码只用于最终客户端响应，不能写回上游错误或参与重试等内部决策。
func ReplaceClientErrorCandidates(statusCode int, messages ...string) (string, int, bool) {
	errorMessageReplacementState.RLock()
	defer errorMessageReplacementState.RUnlock()
	for _, rule := range errorMessageReplacementState.rules {
		if rule.StatusCode != nil && *rule.StatusCode != statusCode {
			continue
		}
		for matchIndex, matchValue := range rule.Matches {
			for _, message := range messages {
				matched := false
				switch rule.Mode {
				case ErrorMessageReplacementModeExact:
					matched = message == matchValue
				case ErrorMessageReplacementModeRegex:
					matched = rule.regularExpressions[matchIndex].MatchString(message)
				default:
					matched = strings.Contains(message, matchValue)
				}
				if matched {
					replacedStatusCode := statusCode
					if rule.ReplaceStatusCode != nil {
						replacedStatusCode = *rule.ReplaceStatusCode
					}
					return rule.Replace, replacedStatusCode, true
				}
			}
		}
	}
	if len(messages) == 0 {
		return "", statusCode, false
	}
	return messages[len(messages)-1], statusCode, false
}

func parseErrorMessageReplacementRules(value string) ([]compiledErrorMessageReplacementRule, error) {
	var rawRules []ErrorMessageReplacementRule
	if err := UnmarshalJsonStr(value, &rawRules); err != nil {
		return nil, fmt.Errorf("错误消息替换规则必须是 JSON 数组: %w", err)
	}
	if rawRules == nil {
		return nil, fmt.Errorf("错误消息替换规则必须是 JSON 数组，不能是 null")
	}
	if len(rawRules) > maxErrorMessageReplacementRules {
		return nil, fmt.Errorf("错误消息替换规则最多允许 %d 条", maxErrorMessageReplacementRules)
	}

	rules := make([]compiledErrorMessageReplacementRule, 0, len(rawRules))
	for index, rawRule := range rawRules {
		legacyMatch := strings.TrimSpace(rawRule.Match)
		matchValues := rawRule.Matches
		if matchValues == nil {
			matchValues = []string{legacyMatch}
		} else if len(matchValues) == 0 {
			return nil, fmt.Errorf("第 %d 条错误消息替换规则至少需要一个匹配值", index+1)
		}
		if len(matchValues) > maxErrorMessageMatchesPerRule {
			return nil, fmt.Errorf("第 %d 条错误消息替换规则最多允许 %d 个匹配值", index+1, maxErrorMessageMatchesPerRule)
		}
		normalizedMatches := make([]string, len(matchValues))
		for matchIndex, matchValue := range matchValues {
			normalizedMatches[matchIndex] = strings.TrimSpace(matchValue)
			if normalizedMatches[matchIndex] == "" {
				return nil, fmt.Errorf("第 %d 条错误消息替换规则的第 %d 个匹配值不能为空", index+1, matchIndex+1)
			}
			if utf8.RuneCountInString(normalizedMatches[matchIndex]) > maxErrorMessageMatchLength {
				return nil, fmt.Errorf("第 %d 条错误消息替换规则的第 %d 个匹配值不能超过 %d 个字符", index+1, matchIndex+1, maxErrorMessageMatchLength)
			}
		}
		if legacyMatch != "" && legacyMatch != normalizedMatches[0] {
			return nil, fmt.Errorf("第 %d 条错误消息替换规则的 match 必须与 matches 首项一致", index+1)
		}
		rule := compiledErrorMessageReplacementRule{ErrorMessageReplacementRule: ErrorMessageReplacementRule{
			Match:             normalizedMatches[0],
			Matches:           normalizedMatches,
			Mode:              strings.ToLower(strings.TrimSpace(rawRule.Mode)),
			StatusCode:        rawRule.StatusCode,
			Replace:           strings.TrimSpace(rawRule.Replace),
			ReplaceStatusCode: rawRule.ReplaceStatusCode,
		}}
		if rule.Replace == "" {
			return nil, fmt.Errorf("第 %d 条错误消息替换规则缺少替换文案", index+1)
		}
		if utf8.RuneCountInString(rule.Replace) > maxErrorMessageReplaceLength {
			return nil, fmt.Errorf("第 %d 条错误消息替换规则的替换文案不能超过 %d 个字符", index+1, maxErrorMessageReplaceLength)
		}
		if rule.StatusCode != nil && (*rule.StatusCode < 100 || *rule.StatusCode > 599) {
			return nil, fmt.Errorf("第 %d 条错误消息替换规则的原状态码必须在 100 到 599 之间", index+1)
		}
		if rule.ReplaceStatusCode != nil && (*rule.ReplaceStatusCode < 400 || *rule.ReplaceStatusCode > 599) {
			return nil, fmt.Errorf("第 %d 条错误消息替换规则的替换状态码必须在 400 到 599 之间", index+1)
		}
		switch rule.Mode {
		case "", ErrorMessageReplacementModeContains:
			rule.Mode = ErrorMessageReplacementModeContains
		case ErrorMessageReplacementModeExact:
		case ErrorMessageReplacementModeRegex:
			rule.regularExpressions = make([]*regexp.Regexp, len(rule.Matches))
			for matchIndex, matchValue := range rule.Matches {
				compiled, err := regexp.Compile(matchValue)
				if err != nil {
					return nil, fmt.Errorf("第 %d 条错误消息替换规则的第 %d 个正则表达式无效: %w", index+1, matchIndex+1, err)
				}
				rule.regularExpressions[matchIndex] = compiled
			}
		default:
			return nil, fmt.Errorf("第 %d 条错误消息替换规则的匹配模式无效", index+1)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

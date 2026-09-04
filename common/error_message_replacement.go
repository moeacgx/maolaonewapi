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
	exactExpressions   []*regexp.Regexp
	foldedMatches      []string
}

var errorMessageReplacementState = struct {
	sync.RWMutex
	rules []compiledErrorMessageReplacementRule
}{}

// ValidateErrorMessageReplacementRules validates rules used only while serializing final client errors.
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

// ReplaceClientErrorCandidates applies the first matching rule to final client data.
// The input status and messages are authoritative internal values and are never mutated.
func ReplaceClientErrorCandidates(statusCode int, messages ...string) (string, int, bool) {
	errorMessageReplacementState.RLock()
	defer errorMessageReplacementState.RUnlock()
	var foldedMessages []string
	foldMessage := func(index int) string {
		if foldedMessages == nil {
			foldedMessages = make([]string, len(messages))
		}
		if foldedMessages[index] == "" && messages[index] != "" {
			foldedMessages[index] = strings.ToLower(messages[index])
		}
		return foldedMessages[index]
	}
	for _, rule := range errorMessageReplacementState.rules {
		if rule.StatusCode != nil && *rule.StatusCode != statusCode {
			continue
		}
		// exact/regex partial replacement must operate on the final client
		// candidate, which has already passed the caller's masking step. Raw
		// candidates remain available only for contains matching, whose output
		// is always the configured replacement text.
		clientMessage := ""
		if len(messages) > 0 {
			clientMessage = messages[len(messages)-1]
		}
		for matchIndex := range rule.Matches {
			switch rule.Mode {
			case ErrorMessageReplacementModeExact:
				if rule.exactExpressions[matchIndex].MatchString(clientMessage) {
					replacedMessage := rule.exactExpressions[matchIndex].ReplaceAllStringFunc(clientMessage, func(string) string {
						return rule.Replace
					})
					return replacedMessage, replacementStatusCode(statusCode, rule.ReplaceStatusCode), true
				}
			case ErrorMessageReplacementModeRegex:
				if rule.regularExpressions[matchIndex].MatchString(clientMessage) {
					replacedMessage := rule.regularExpressions[matchIndex].ReplaceAllString(clientMessage, rule.Replace)
					return replacedMessage, replacementStatusCode(statusCode, rule.ReplaceStatusCode), true
				}
			default:
				for messageIndex := range messages {
					if strings.Contains(foldMessage(messageIndex), rule.foldedMatches[matchIndex]) {
						return rule.Replace, replacementStatusCode(statusCode, rule.ReplaceStatusCode), true
					}
				}
			}
		}
	}
	if len(messages) == 0 {
		return "", statusCode, false
	}
	return messages[len(messages)-1], statusCode, false
}

func replacementStatusCode(statusCode int, replaceStatusCode *int) int {
	if replaceStatusCode != nil {
		return *replaceStatusCode
	}
	return statusCode
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
			Match: normalizedMatches[0], Matches: normalizedMatches, Mode: strings.ToLower(strings.TrimSpace(rawRule.Mode)),
			StatusCode: rawRule.StatusCode, Replace: strings.TrimSpace(rawRule.Replace), ReplaceStatusCode: rawRule.ReplaceStatusCode,
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
			rule.foldedMatches = foldErrorMessageMatches(rule.Matches)
		case ErrorMessageReplacementModeExact:
			rule.exactExpressions = make([]*regexp.Regexp, len(rule.Matches))
			for matchIndex, matchValue := range rule.Matches {
				compiled, err := regexp.Compile("(?i)" + regexp.QuoteMeta(matchValue))
				if err != nil {
					return nil, fmt.Errorf("第 %d 条错误消息替换规则的第 %d 个精确匹配值无效: %w", index+1, matchIndex+1, err)
				}
				rule.exactExpressions[matchIndex] = compiled
			}
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

func foldErrorMessageMatches(matches []string) []string {
	folded := make([]string, len(matches))
	for index, match := range matches {
		folded[index] = strings.ToLower(match)
	}
	return folded
}

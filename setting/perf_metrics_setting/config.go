package perf_metrics_setting

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	MaxFailureFilterRules     = 100
	MaxFailureFilterRuleID    = 64
	MaxFailureFilterRuleName  = 128
	MaxFailureFilterRuleValue = 4096

	FailureFilterFieldStatusCode = "status_code"
	FailureFilterFieldErrorCode  = "error_code"
	FailureFilterFieldMessage    = "message"
	FailureFilterFieldFullError  = "full_error"

	FailureFilterModeContains = "contains"
	FailureFilterModeExact    = "exact"
	FailureFilterModeRegex    = "regex"
)

var failureFilterRuleIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type FailureFilterRule struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Field   string `json:"field"`
	Mode    string `json:"mode"`
	Value   string `json:"value"`
}

type PerfMetricsSetting struct {
	Enabled            bool                `json:"enabled"`
	FlushInterval      int                 `json:"flush_interval"`
	BucketTime         string              `json:"bucket_time"`
	RetentionDays      int                 `json:"retention_days"`
	FailureFilterRules []FailureFilterRule `json:"failure_filter_rules"`
}

var perfMetricsSetting = PerfMetricsSetting{
	Enabled:            true,
	FlushInterval:      5,
	BucketTime:         "hour",
	RetentionDays:      0,
	FailureFilterRules: []FailureFilterRule{},
}

func init() {
	config.GlobalConfig.Register("perf_metrics_setting", &perfMetricsSetting)
}

func GetSetting() PerfMetricsSetting {
	return perfMetricsSetting
}

func GetBucketSeconds() int64 {
	switch perfMetricsSetting.BucketTime {
	case "minute":
		return 60
	case "5min":
		return 300
	case "hour":
		return 3600
	default:
		return 3600
	}
}

func GetFlushIntervalMinutes() int {
	if perfMetricsSetting.FlushInterval < 1 {
		return 1
	}
	return perfMetricsSetting.FlushInterval
}

func ParseAndValidateFailureFilterRules(value string) ([]FailureFilterRule, error) {
	var rules []FailureFilterRule
	if err := common.UnmarshalJsonStr(value, &rules); err != nil {
		return nil, fmt.Errorf("模型广场失败过滤规则必须是 JSON 数组: %w", err)
	}
	if rules == nil {
		return nil, fmt.Errorf("模型广场失败过滤规则必须是 JSON 数组，不能是 null")
	}
	if len(rules) > MaxFailureFilterRules {
		return nil, fmt.Errorf("模型广场失败过滤规则最多允许 %d 条", MaxFailureFilterRules)
	}

	ids := make(map[string]struct{}, len(rules))
	for index := range rules {
		rule := &rules[index]
		rule.ID = strings.TrimSpace(rule.ID)
		rule.Name = strings.TrimSpace(rule.Name)
		rule.Field = strings.TrimSpace(rule.Field)
		rule.Mode = strings.TrimSpace(rule.Mode)

		if rule.ID == "" {
			return nil, fmt.Errorf("第 %d 条模型广场失败过滤规则缺少 ID", index+1)
		}
		if len(rule.ID) > MaxFailureFilterRuleID {
			return nil, fmt.Errorf("第 %d 条模型广场失败过滤规则 ID 不能超过 %d 个字符", index+1, MaxFailureFilterRuleID)
		}
		if !failureFilterRuleIDPattern.MatchString(rule.ID) {
			return nil, fmt.Errorf("第 %d 条模型广场失败过滤规则 ID 只能包含 ASCII 字母、数字、点、下划线和连字符", index+1)
		}
		if _, exists := ids[rule.ID]; exists {
			return nil, fmt.Errorf("第 %d 条模型广场失败过滤规则 ID 重复", index+1)
		}
		ids[rule.ID] = struct{}{}
		if rule.Name == "" {
			return nil, fmt.Errorf("第 %d 条模型广场失败过滤规则缺少名称", index+1)
		}
		if utf8.RuneCountInString(rule.Name) > MaxFailureFilterRuleName {
			return nil, fmt.Errorf("第 %d 条模型广场失败过滤规则名称不能超过 %d 个字符", index+1, MaxFailureFilterRuleName)
		}
		if utf8.RuneCountInString(rule.Value) > MaxFailureFilterRuleValue {
			return nil, fmt.Errorf("第 %d 条模型广场失败过滤规则匹配值不能超过 %d 个字符", index+1, MaxFailureFilterRuleValue)
		}
		if rule.Value == "" {
			return nil, fmt.Errorf("第 %d 条模型广场失败过滤规则缺少匹配值", index+1)
		}
		switch rule.Field {
		case FailureFilterFieldStatusCode, FailureFilterFieldErrorCode,
			FailureFilterFieldMessage, FailureFilterFieldFullError:
		default:
			return nil, fmt.Errorf("第 %d 条模型广场失败过滤规则的匹配字段无效", index+1)
		}
		switch rule.Mode {
		case FailureFilterModeContains, FailureFilterModeExact:
		case FailureFilterModeRegex:
			if _, err := regexp.Compile(rule.Value); err != nil {
				return nil, fmt.Errorf("第 %d 条模型广场失败过滤规则的正则表达式无效: %w", index+1, err)
			}
		default:
			return nil, fmt.Errorf("第 %d 条模型广场失败过滤规则的匹配模式无效", index+1)
		}
	}
	return rules, nil
}

func ValidateFailureFilterRules(value string) error {
	_, err := ParseAndValidateFailureFilterRules(value)
	return err
}

package reasoning

import (
	"strings"

	"github.com/samber/lo"
)

var EffortSuffixes = []string{"-ultra", "-max", "-xhigh", "-high", "-medium", "-low", "-minimal"}

var OpenAIEffortSuffixes = []string{"-high", "-minimal", "-low", "-medium", "-none", "-xhigh"}

var DeepSeekV4EffortSuffixes = []string{"-none", "-max"}

// TrimEffortSuffix -> modelName level(low) exists
func TrimEffortSuffix(modelName string) (string, string, bool) {
	return TrimEffortSuffixWithSuffixes(modelName, EffortSuffixes)
}

func TrimEffortSuffixWithSuffixes(modelName string, suffixes []string) (string, string, bool) {
	suffix, found := lo.Find(suffixes, func(s string) bool {
		return strings.HasSuffix(modelName, s)
	})
	if !found {
		return modelName, "", false
	}
	return strings.TrimSuffix(modelName, suffix), strings.TrimPrefix(suffix, "-"), true
}

// ParseGeminiReasoningEffortFromModelSuffix parses effort suffixes only for
// Gemini text reasoning model families. Other provider model IDs can be routed
// through Gemini-compatible channels and must remain byte-for-byte unchanged.
func ParseGeminiReasoningEffortFromModelSuffix(modelName string) (string, string, bool) {
	baseModel, effort, ok := TrimEffortSuffix(modelName)
	if !ok || !IsGeminiReasoningModel(baseModel) {
		return modelName, "", false
	}
	return baseModel, effort, true
}

// IsGeminiReasoningModel reports whether modelName belongs to a supported
// Gemini text reasoning family. Family boundaries are segment-aware so an
// unrelated model such as gemini-2.5-flashlike is not confused with a
// supported family solely because it shares a textual prefix.
func IsGeminiReasoningModel(modelName string) bool {
	if strings.Contains(modelName, "-image") ||
		strings.Contains(modelName, "-native-audio") ||
		strings.Contains(modelName, "-tts") ||
		strings.HasPrefix(modelName, "gemini-embedding") {
		return false
	}
	return hasModelFamily(modelName, "gemini-2.5-flash") ||
		hasModelFamily(modelName, "gemini-2.5-pro") ||
		strings.HasPrefix(modelName, "gemini-3-") ||
		strings.HasPrefix(modelName, "gemini-3.") ||
		modelName == "gemini-flash-latest" ||
		modelName == "gemini-flash-lite-latest" ||
		modelName == "gemini-pro-latest"
}

// IsClaudeOpusReasoningModel reports whether modelName is a supported Claude
// Opus 4.x reasoning family. The segment boundary keeps 4-60-like IDs intact.
func IsClaudeOpusReasoningModel(modelName string) bool {
	return hasModelFamily(modelName, "claude-opus-4-6") ||
		hasModelFamily(modelName, "claude-opus-4-7") ||
		hasModelFamily(modelName, "claude-opus-4-8")
}

// IsClaudeOpus47Or48 reports whether a Claude Opus model is in a family that
// rejects legacy sampling parameters.
func IsClaudeOpus47Or48(modelName string) bool {
	return hasModelFamily(modelName, "claude-opus-4-7") ||
		hasModelFamily(modelName, "claude-opus-4-8")
}

// IsClaudeThinkingModel reports whether modelName is an exact supported
// Claude thinking family or its eight-digit dated alias.
func IsClaudeThinkingModel(modelName string) bool {
	return isExactOrDatedClaudeFamily(modelName, "claude-sonnet-4-6") ||
		isExactOrDatedClaudeFamily(modelName, "claude-sonnet-4-5") ||
		isExactOrDatedClaudeFamily(modelName, "claude-3-7-sonnet") ||
		isExactOrDatedClaudeFamily(modelName, "claude-opus-4-6") ||
		isExactOrDatedClaudeFamily(modelName, "claude-opus-4-7") ||
		isExactOrDatedClaudeFamily(modelName, "claude-opus-4-8")
}

func isExactOrDatedClaudeFamily(modelName, family string) bool {
	if modelName == family {
		return true
	}
	if len(modelName) != len(family)+9 || !strings.HasPrefix(modelName, family) || modelName[len(family)] != '-' {
		return false
	}
	for _, digit := range modelName[len(family)+1:] {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func hasModelFamily(modelName, family string) bool {
	return modelName == family || strings.HasPrefix(modelName, family+"-")
}

func ParseOpenAIReasoningEffortFromModelSuffix(modelName string) (string, string) {
	baseModel, effort, ok := TrimEffortSuffixWithSuffixes(modelName, OpenAIEffortSuffixes)
	if !ok {
		return "", modelName
	}
	return effort, baseModel
}

func NormalizeOpenAIReasoningEffort(effort string) string {
	normalized := strings.TrimSpace(strings.ToLower(effort))
	normalized = strings.ReplaceAll(normalized, "_", " ")
	normalized = strings.ReplaceAll(normalized, "-", " ")
	normalized = strings.Join(strings.Fields(normalized), " ")
	switch normalized {
	case "low", "medium", "high", "minimal", "none", "xhigh", "max", "ultra":
		return normalized
	case "extra high", "extra":
		return "xhigh"
	case "maximum":
		return "max"
	case "ultra high", "ultrahigh":
		return "ultra"
	default:
		return strings.TrimSpace(strings.ToLower(effort))
	}
}

func ParseDeepSeekV4ThinkingSuffix(modelName string) (baseModel string, thinkingType string, effort string, ok bool) {
	baseModel, suffix, ok := TrimEffortSuffixWithSuffixes(modelName, DeepSeekV4EffortSuffixes)
	if !ok || !strings.HasPrefix(baseModel, "deepseek-v4-") {
		return modelName, "", "", false
	}
	switch suffix {
	case "none":
		return baseModel, "disabled", "", true
	case "max":
		return baseModel, "enabled", "max", true
	default:
		return modelName, "", "", false
	}
}

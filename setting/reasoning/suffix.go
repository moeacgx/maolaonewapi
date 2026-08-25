// Package reasoning re-exports the pure model-name effort-suffix helpers,
// which moved to the conversion kit (service/relayconvert/reasoning) as part
// of the relaykit extraction. Host code keeps importing this path unchanged.
package reasoning

import kitreasoning "github.com/QuantumNous/new-api/relaykit/relayconvert/reasoning"

var (
	EffortSuffixes           = kitreasoning.EffortSuffixes
	OpenAIEffortSuffixes     = kitreasoning.OpenAIEffortSuffixes
	DeepSeekV4EffortSuffixes = kitreasoning.DeepSeekV4EffortSuffixes
)

var (
	TrimEffortSuffix                          = kitreasoning.TrimEffortSuffix
	TrimEffortSuffixWithSuffixes              = kitreasoning.TrimEffortSuffixWithSuffixes
	ParseGeminiReasoningEffortFromModelSuffix = kitreasoning.ParseGeminiReasoningEffortFromModelSuffix
	TrimGeminiThinkingSuffix                  = kitreasoning.TrimGeminiThinkingSuffix
	HasGeminiThinkingSuffix                   = kitreasoning.HasGeminiThinkingSuffix
	IsGeminiReasoningModel                    = kitreasoning.IsGeminiReasoningModel
	IsClaudeOpusReasoningModel                = kitreasoning.IsClaudeOpusReasoningModel
	IsClaudeOpus47Or48                        = kitreasoning.IsClaudeOpus47Or48
	IsClaudeThinkingModel                     = kitreasoning.IsClaudeThinkingModel
	ParseOpenAIReasoningEffortFromModelSuffix = kitreasoning.ParseOpenAIReasoningEffortFromModelSuffix
	NormalizeOpenAIReasoningEffort            = kitreasoning.NormalizeOpenAIReasoningEffort
	ParseDeepSeekV4ThinkingSuffix             = kitreasoning.ParseDeepSeekV4ThinkingSuffix
)

package claude

import (
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

var samplingUnsupportedClaudeModelFamilies = [...]string{
	"claude-opus-4-7",
	"claude-opus-4-8",
}

// NormalizeClaudeSamplingParameters removes sampling fields rejected by the
// configured Claude Opus 4.7+ Messages model families.
func NormalizeClaudeSamplingParameters(request *dto.ClaudeRequest) {
	if request == nil || !isClaudeSamplingUnsupportedModel(request.Model) {
		return
	}
	request.Temperature = nil
	request.TopP = nil
	request.TopK = nil
}

func isClaudeSamplingUnsupportedModel(model string) bool {
	model = strings.TrimSpace(model)
	for _, family := range samplingUnsupportedClaudeModelFamilies {
		if model == family || strings.HasPrefix(model, family+"-") {
			return true
		}
	}
	return false
}

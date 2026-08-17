package claude

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeClaudeSamplingParametersPreservesSupportedFamiliesAndExplicitZeros(t *testing.T) {
	for _, model := range []string{
		"claude-sonnet-4-6",
		"claude-sonnet-4-6-20260217",
		"claude-opus-4-6",
		"claude-opus-4-6-high",
	} {
		t.Run(model, func(t *testing.T) {
			temperature := 0.0
			topP := 0.0
			topK := 0
			request := &dto.ClaudeRequest{Model: model, Temperature: &temperature, TopP: &topP, TopK: &topK}

			NormalizeClaudeSamplingParameters(request)

			assert.Same(t, &temperature, request.Temperature)
			assert.Same(t, &topP, request.TopP)
			assert.Same(t, &topK, request.TopK)
			assert.Zero(t, *request.Temperature)
			assert.Zero(t, *request.TopP)
			assert.Zero(t, *request.TopK)
		})
	}
}

func TestNormalizeClaudeSamplingParametersStripsUnsupportedFamilies(t *testing.T) {
	for _, model := range []string{
		"claude-opus-4-7",
		"claude-opus-4-7-20260217",
		"claude-opus-4-8",
		"claude-opus-4-8-20260801",
	} {
		t.Run(model, func(t *testing.T) {
			temperature := 0.7
			topP := 0.9
			topK := 40
			request := &dto.ClaudeRequest{Model: model, Temperature: &temperature, TopP: &topP, TopK: &topK}

			NormalizeClaudeSamplingParameters(request)

			assert.Nil(t, request.Temperature)
			assert.Nil(t, request.TopP)
			assert.Nil(t, request.TopK)
		})
	}
}

func TestNormalizeClaudeSamplingParametersPreservesOlderAndUnrelatedModels(t *testing.T) {
	for _, model := range []string{"claude-sonnet-4-5", "claude-haiku-4-5", "claude-opus-4-60"} {
		t.Run(model, func(t *testing.T) {
			temperature := 0.7
			topP := 0.9
			topK := 40
			request := &dto.ClaudeRequest{Model: model, Temperature: &temperature, TopP: &topP, TopK: &topK}

			NormalizeClaudeSamplingParameters(request)

			assert.Same(t, &temperature, request.Temperature)
			assert.Same(t, &topP, request.TopP)
			assert.Same(t, &topK, request.TopK)
		})
	}
}

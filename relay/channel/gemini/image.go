package gemini

import (
	"encoding/json"
	"errors"
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/samber/lo"
)

func convertGeminiNativeImageRequest(info *relaycommon.RelayInfo, request dto.ImageRequest) (*dto.GeminiChatRequest, error) {
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return nil, errors.New("prompt is required")
	}

	parts := []dto.GeminiPart{{Text: prompt}}
	var err error
	parts, err = appendGeminiReferenceImageParts(parts, request.Images)
	if err != nil {
		return nil, err
	}
	parts, err = appendGeminiReferenceImageParts(parts, request.Image)
	if err != nil {
		return nil, err
	}

	geminiRequest := &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{{
			Role:  "user",
			Parts: parts,
		}},
		GenerationConfig: dto.GeminiChatGenerationConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
		},
	}

	if n := int(lo.FromPtrOr(request.N, uint(1))); n > 1 {
		geminiRequest.GenerationConfig.CandidateCount = &n
	}
	if imageConfig := geminiNativeImageConfig(request); len(imageConfig) > 0 {
		encoded, err := json.Marshal(imageConfig)
		if err != nil {
			return nil, err
		}
		geminiRequest.GenerationConfig.ImageConfig = encoded
	}
	if info != nil {
		geminiRequest.SafetySettings = geminiImageSafetySettings(info)
	}
	return geminiRequest, nil
}

func geminiImageSafetySettings(info *relaycommon.RelayInfo) []dto.GeminiChatSafetySettings {
	opts := info.ConvOptions()
	settings := make([]dto.GeminiChatSafetySettings, 0, len(SafetySettingList))
	for _, category := range SafetySettingList {
		threshold := opts.Gemini.SafetySettingFor(category)
		if threshold == "" {
			continue
		}
		settings = append(settings, dto.GeminiChatSafetySettings{
			Category:  category,
			Threshold: threshold,
		})
	}
	return settings
}

func geminiNativeImageConfig(request dto.ImageRequest) map[string]string {
	config := make(map[string]string)
	if aspectRatio := geminiAspectRatioFromSize(request.Size); aspectRatio != "" {
		config["aspectRatio"] = aspectRatio
	}
	if imageSize := geminiImageSizeFromQuality(request.Quality); imageSize != "" {
		config["imageSize"] = imageSize
	}
	return config
}

func geminiAspectRatioFromSize(size string) string {
	size = strings.TrimSpace(size)
	if size == "" || strings.EqualFold(size, "auto") {
		return ""
	}
	if strings.Contains(size, ":") {
		return size
	}
	switch size {
	case "256x256", "512x512", "1024x1024":
		return "1:1"
	case "1536x1024":
		return "3:2"
	case "1024x1536":
		return "2:3"
	case "1024x1792":
		return "9:16"
	case "1792x1024":
		return "16:9"
	default:
		return ""
	}
}

func geminiImageSizeFromQuality(quality string) string {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "":
		return ""
	case "hd", "high", "2k":
		return "2K"
	case "4k", "ultra":
		return "4K"
	case "standard", "medium", "low", "auto", "1k":
		return "1K"
	default:
		return "1K"
	}
}

func appendGeminiReferenceImageParts(parts []dto.GeminiPart, raw json.RawMessage) ([]dto.GeminiPart, error) {
	values, err := parseGeminiImageValues(raw)
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		part, ok := geminiPartFromImageValue(value)
		if !ok {
			continue
		}
		parts = append(parts, part)
	}
	return parts, nil
}

func parseGeminiImageValues(raw json.RawMessage) ([]string, error) {
	if len(bytesTrimSpace(raw)) == 0 {
		return nil, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil, nil
		}
		return []string{text}, nil
	}
	var array []json.RawMessage
	if err := json.Unmarshal(raw, &array); err == nil {
		values := make([]string, 0, len(array))
		for _, item := range array {
			nested, err := parseGeminiImageValues(item)
			if err != nil {
				return nil, err
			}
			values = append(values, nested...)
		}
		return values, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, nil
	}
	for _, key := range []string{"url", "image_url", "b64_json", "data"} {
		switch value := obj[key].(type) {
		case string:
			if trimmed := strings.TrimSpace(value); trimmed != "" {
				return []string{trimmed}, nil
			}
		case map[string]any:
			if url, ok := value["url"].(string); ok {
				if trimmed := strings.TrimSpace(url); trimmed != "" {
					return []string{trimmed}, nil
				}
			}
		}
	}
	return nil, nil
}

func geminiPartFromImageValue(value string) (dto.GeminiPart, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return dto.GeminiPart{}, false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return dto.GeminiPart{FileData: &dto.GeminiFileData{FileUri: value}}, true
	}
	mimeType, data := parseGeminiInlineImage(value)
	if data == "" {
		return dto.GeminiPart{}, false
	}
	return dto.GeminiPart{InlineData: &dto.GeminiInlineData{MimeType: mimeType, Data: data}}, true
}

func parseGeminiInlineImage(value string) (string, string) {
	if !strings.HasPrefix(strings.ToLower(value), "data:") {
		return "image/png", value
	}
	header, data, ok := strings.Cut(value, ",")
	if !ok || data == "" {
		return "", ""
	}
	mimeType := "image/png"
	header = strings.TrimPrefix(header, "data:")
	if mediaType, _, found := strings.Cut(header, ";"); found && strings.TrimSpace(mediaType) != "" {
		mimeType = strings.TrimSpace(mediaType)
	}
	return mimeType, data
}

func bytesTrimSpace(raw json.RawMessage) json.RawMessage {
	return json.RawMessage(strings.TrimSpace(string(raw)))
}

package gemini

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func convertGeminiNativeImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (*dto.GeminiChatRequest, error) {
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
	parts, err = appendGeminiMultipartImageParts(c, parts)
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

	modelName := request.Model
	if info != nil && info.UpstreamModelName != "" {
		modelName = info.UpstreamModelName
	}
	if imageConfig := geminiNativeImageConfig(modelName, request); len(imageConfig) > 0 {
		encoded, err := common.Marshal(imageConfig)
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

func geminiNativeImageConfig(model string, request dto.ImageRequest) map[string]string {
	config := make(map[string]string)
	if aspectRatio := geminiAspectRatioFromSize(request.Size); aspectRatio != "" {
		config["aspectRatio"] = aspectRatio
	}
	if geminiNativeImageSupportsImageSize(model) {
		if imageSize := geminiImageSizeFromQuality(request.Quality); imageSize != "" {
			config["imageSize"] = imageSize
		}
	}
	return config
}

func geminiNativeImageSupportsImageSize(model string) bool {
	model = strings.ToLower(model)
	return strings.Contains(model, "gemini-3-pro-image") ||
		strings.Contains(model, "gemini-3.1-flash-image")
}

func geminiAspectRatioFromSize(size string) string {
	size = strings.TrimSpace(size)
	if size == "" || strings.EqualFold(size, "auto") {
		return ""
	}
	if strings.Contains(size, ":") {
		return size
	}
	switch strings.ToLower(size) {
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
	case "", "auto":
		return ""
	case "hd", "high", "2k":
		return "2K"
	case "4k", "ultra":
		return "4K"
	case "standard", "medium", "low", "1k":
		return "1K"
	default:
		return ""
	}
}

func appendGeminiReferenceImageParts(parts []dto.GeminiPart, raw json.RawMessage) ([]dto.GeminiPart, error) {
	values, err := parseGeminiImageValues(raw)
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		part, err := geminiPartFromImageValue(value)
		if err != nil {
			return nil, err
		}
		if part.InlineData == nil && part.FileData == nil {
			continue
		}
		parts = append(parts, part)
	}
	return parts, nil
}

func appendGeminiMultipartImageParts(c *gin.Context, parts []dto.GeminiPart) ([]dto.GeminiPart, error) {
	if c == nil || c.Request == nil || !strings.Contains(c.ContentType(), "multipart/form-data") {
		return parts, nil
	}
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image form: %w", err)
	}
	if form == nil || form.File == nil {
		return parts, nil
	}
	var headers []*multipart.FileHeader
	seen := make(map[*multipart.FileHeader]struct{})
	add := func(files []*multipart.FileHeader) {
		for _, file := range files {
			if file == nil {
				continue
			}
			if _, ok := seen[file]; ok {
				continue
			}
			seen[file] = struct{}{}
			headers = append(headers, file)
		}
	}
	add(form.File["image"])
	add(form.File["image[]"])
	for fieldName, files := range form.File {
		if strings.HasPrefix(fieldName, "image[") {
			add(files)
		}
	}
	for _, header := range headers {
		file, err := header.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open reference image: %w", err)
		}
		payload, err := io.ReadAll(file)
		_ = file.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read reference image: %w", err)
		}
		if len(payload) == 0 {
			continue
		}
		mimeType := header.Header.Get("Content-Type")
		if mimeType == "" || mimeType == "application/octet-stream" {
			mimeType = http.DetectContentType(payload)
		}
		if !strings.HasPrefix(mimeType, "image/") {
			mimeType = "image/png"
		}
		parts = append(parts, dto.GeminiPart{
			InlineData: &dto.GeminiInlineData{
				MimeType: mimeType,
				Data:     base64.StdEncoding.EncodeToString(payload),
			},
		})
	}
	return parts, nil
}

func parseGeminiImageValues(raw json.RawMessage) ([]string, error) {
	if len(bytesTrimSpace(raw)) == 0 {
		return nil, nil
	}
	var text string
	if err := common.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil, nil
		}
		return []string{text}, nil
	}
	var array []json.RawMessage
	if err := common.Unmarshal(raw, &array); err == nil {
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
	if err := common.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("invalid image field")
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
	return nil, fmt.Errorf("invalid image field")
}

func geminiPartFromImageValue(value string) (dto.GeminiPart, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return dto.GeminiPart{}, nil
	}
	if isGeminiHostedMediaURI(value) {
		return dto.GeminiPart{
			FileData: &dto.GeminiFileData{
				MimeType: mimeFromGeminiMediaURI(value),
				FileUri:  value,
			},
		}, nil
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		mimeType, data, err := service.GetImageFromUrl(value)
		if err != nil {
			return dto.GeminiPart{}, fmt.Errorf("failed to load reference image: %w", err)
		}
		if mimeType == "" {
			mimeType = "image/png"
		}
		return dto.GeminiPart{InlineData: &dto.GeminiInlineData{MimeType: mimeType, Data: data}}, nil
	}
	mimeType, data := parseGeminiInlineImage(value)
	if data == "" {
		return dto.GeminiPart{}, errors.New("invalid reference image")
	}
	return dto.GeminiPart{InlineData: &dto.GeminiInlineData{MimeType: mimeType, Data: data}}, nil
}

func isGeminiHostedMediaURI(value string) bool {
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "gs://") {
		return true
	}
	if strings.Contains(lower, "generativelanguage.googleapis.com/files/") {
		return true
	}
	if strings.Contains(lower, "youtube.com/") || strings.Contains(lower, "youtu.be/") {
		return true
	}
	return false
}

func mimeFromGeminiMediaURI(value string) string {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "youtube.com/") || strings.Contains(lower, "youtu.be/") {
		return "video/webm"
	}
	switch strings.ToLower(path.Ext(strings.Split(value, "?")[0])) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".heic":
		return "image/heic"
	case ".heif":
		return "image/heif"
	default:
		return "image/png"
	}
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

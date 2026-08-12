package atlascloud

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	rootconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func BuildAPIURL(baseURL, path string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = rootconstant.ChannelBaseURLs[rootconstant.ChannelTypeAtlasCloud]
	}
	if strings.HasSuffix(strings.ToLower(baseURL), "/api/v1") {
		path = strings.TrimPrefix(path, "/api/v1")
	}
	return baseURL + path
}

func EnsureChannelMeta(info *relaycommon.RelayInfo) {
	if info != nil && info.ChannelMeta == nil {
		info.ChannelMeta = &relaycommon.ChannelMeta{}
	}
}

func UpstreamImageModelName(modelName string, edit bool) string {
	modelName = strings.TrimSpace(modelName)
	if edit {
		return upstreamImageEditModelName(modelName)
	}
	return modelName
}

func upstreamImageEditModelName(modelName string) string {
	modelName = strings.TrimSpace(modelName)
	if strings.HasSuffix(strings.ToLower(modelName), "/edit") {
		return modelName
	}
	if strings.HasSuffix(strings.ToLower(modelName), "/text-to-image") {
		return strings.TrimSpace(modelName[:len(modelName)-len("/text-to-image")]) + "/edit"
	}
	return modelName
}

func imageEditUsesImageField(modelName string) bool {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	return strings.HasPrefix(modelName, "openai/gpt-image-") && strings.HasSuffix(modelName, "/edit")
}

func normalizeImageQuality(modelName string, edit bool, quality string) string {
	quality = strings.TrimSpace(quality)
	if quality == "" {
		return ""
	}
	if edit && imageEditUsesImageField(modelName) && strings.EqualFold(quality, "standard") {
		return "auto"
	}
	return quality
}

type ImageParameterDefaults struct {
	Size        string
	Quality     string
	Resolution  string
	AspectRatio string
}

func ImageParameterDefaultsForModel(modelName string, edit bool) (ImageParameterDefaults, bool) {
	modelName = strings.ToLower(strings.TrimSpace(UpstreamImageModelName(modelName, edit)))
	switch {
	case isAtlasOpenAIGPTImageModel(modelName):
		return ImageParameterDefaults{Size: "1024x1024", Quality: "medium"}, true
	case modelName == "xai/grok-imagine-image/text-to-image":
		return ImageParameterDefaults{Resolution: "1k", AspectRatio: "1:1"}, true
	case modelName == "xai/grok-imagine-image/edit":
		return ImageParameterDefaults{Resolution: "1k", AspectRatio: "1:1"}, true
	default:
		return ImageParameterDefaults{}, false
	}
}

func isAtlasOpenAIGPTImageModel(modelName string) bool {
	return strings.HasPrefix(modelName, "openai/gpt-image-") &&
		(strings.HasSuffix(modelName, "/text-to-image") || strings.HasSuffix(modelName, "/edit"))
}

func ApplyImageRequestDefaults(c *gin.Context, request *dto.ImageRequest, modelName string, edit bool) {
	if request == nil {
		return
	}
	defaults, ok := ImageParameterDefaultsForModel(modelName, edit)
	if !ok {
		return
	}
	if defaults.Size != "" && strings.TrimSpace(request.Size) == "" && imageRequestExtraString(request, "size") == "" {
		request.Size = defaults.Size
		setMultipartFormValue(c, "size", defaults.Size)
	}
	if defaults.Quality != "" && strings.TrimSpace(request.Quality) == "" && imageRequestExtraString(request, "quality") == "" {
		request.Quality = defaults.Quality
		setMultipartFormValue(c, "quality", defaults.Quality)
	}
}

func ApplyImagePayloadDefaults(payload map[string]any, modelName string, edit bool) {
	if payload == nil {
		return
	}
	defaults, ok := ImageParameterDefaultsForModel(modelName, edit)
	if !ok {
		return
	}
	setPayloadDefault(payload, "size", defaults.Size)
	setPayloadDefault(payload, "quality", defaults.Quality)
	setPayloadDefault(payload, "resolution", defaults.Resolution)
	setPayloadDefault(payload, "aspect_ratio", defaults.AspectRatio)
}

func ApplyImageBillingDefaults(meta *types.TokenCountMeta, request *dto.ImageRequest, modelName string, edit bool) {
	if meta == nil || request == nil {
		return
	}
	defaults, ok := ImageParameterDefaultsForModel(modelName, edit)
	if !ok {
		return
	}
	if meta.BillingDimensions == nil {
		meta.BillingDimensions = make(map[string]string, 2)
	}
	resolution := firstNonEmptyString(
		meta.BillingDimensions[ratio_setting.ModelPriceVariantResolution],
		strings.TrimSpace(request.Size),
		imageRequestExtraString(request, "size"),
		imageRequestExtraString(request, "resolution"),
		defaults.Size,
		defaults.Resolution,
	)
	if resolution != "" {
		meta.BillingDimensions[ratio_setting.ModelPriceVariantResolution] = resolution
	}
	quality := firstNonEmptyString(
		meta.BillingDimensions[ratio_setting.ModelPriceVariantQuality],
		strings.TrimSpace(request.Quality),
		imageRequestExtraString(request, "quality"),
		defaults.Quality,
	)
	if quality != "" {
		meta.BillingDimensions[ratio_setting.ModelPriceVariantQuality] = quality
	}
}

func isMultipartFormRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	contentType := strings.TrimSpace(c.Request.Header.Get("Content-Type"))
	if contentType == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.SplitN(contentType, ";", 2)[0]
	}
	return strings.EqualFold(strings.TrimSpace(mediaType), gin.MIMEMultipartPOSTForm)
}

func setPayloadDefault(payload map[string]any, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if _, ok := payload[key]; ok {
		return
	}
	payload[key] = value
}

func setMultipartFormValue(c *gin.Context, key, value string) {
	if c == nil || c.Request == nil || c.Request.MultipartForm == nil || strings.TrimSpace(value) == "" {
		return
	}
	if c.Request.MultipartForm.Value == nil {
		c.Request.MultipartForm.Value = make(map[string][]string)
	}
	if _, exists := c.Request.MultipartForm.Value[key]; !exists {
		c.Request.MultipartForm.Value[key] = []string{value}
	}
}

func imageRequestExtraString(request *dto.ImageRequest, key string) string {
	if request == nil {
		return ""
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if value := rawMessageMapString(request.Extra, key); value != "" {
		return value
	}
	if len(request.ExtraFields) == 0 {
		return ""
	}
	var extraFields map[string]any
	if err := common.Unmarshal(request.ExtraFields, &extraFields); err != nil {
		return ""
	}
	return interfaceString(extraFields[key])
}

func rawMessageMapString(values map[string]json.RawMessage, key string) string {
	if len(values) == 0 {
		return ""
	}
	raw, ok := values[key]
	if !ok || len(raw) == 0 {
		return ""
	}
	var text string
	if err := common.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return interfaceString(value)
}

func interfaceString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func imagePollTimeout(modelName string) time.Duration {
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	if modelName == ModelGPTImage2 ||
		strings.HasPrefix(modelName, "openai/gpt-image-2/") ||
		strings.Contains(modelName, "/gpt-image-2/") {
		return time.Duration(gptImage2PollTimeoutSec) * time.Second
	}
	return time.Duration(imagePollTimeoutSec) * time.Second
}

func PredictionID(resp apiResponse) string {
	if strings.TrimSpace(resp.Data.ID) != "" {
		return strings.TrimSpace(resp.Data.ID)
	}
	return strings.TrimSpace(resp.Data.TaskID)
}

func ErrorText(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		data, err := common.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return strings.TrimSpace(string(data))
	}
}

func FirstOutput(outputs []string) string {
	for _, output := range outputs {
		if strings.TrimSpace(output) != "" {
			return strings.TrimSpace(output)
		}
	}
	return ""
}

func MergeExtraFields(payload map[string]any, extraFields []byte, extra map[string]json.RawMessage) error {
	if len(extraFields) > 0 {
		var decoded map[string]any
		if err := common.Unmarshal(extraFields, &decoded); err != nil {
			return fmt.Errorf("decode extra_fields failed: %w", err)
		}
		for key, value := range decoded {
			if isInternalImagePayloadField(key) {
				continue
			}
			payload[key] = value
		}
	}
	for key, raw := range extra {
		if isInternalImagePayloadField(key) {
			continue
		}
		if raw == nil {
			continue
		}
		var value any
		if err := common.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("decode extra field %s failed: %w", key, err)
		}
		payload[key] = value
	}
	return nil
}

func isInternalImagePayloadField(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "group":
		return true
	default:
		return false
	}
}

func NormalizeImageURL(c *gin.Context, info *relaycommon.RelayInfo, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if strings.HasPrefix(strings.ToLower(raw), "data:") {
		return UploadDataURL(c, info, raw)
	}
	return raw, nil
}

func UploadDataURL(c *gin.Context, info *relaycommon.RelayInfo, dataURL string) (string, error) {
	header, data, ok := strings.Cut(dataURL, ",")
	if !ok || !strings.Contains(strings.ToLower(header), ";base64") {
		return "", fmt.Errorf("atlascloud: image data URL must be base64 encoded")
	}
	mimeType := "application/octet-stream"
	if strings.HasPrefix(strings.ToLower(header), "data:") {
		mimeType = strings.TrimPrefix(strings.SplitN(header, ";", 2)[0], "data:")
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", fmt.Errorf("atlascloud: decode image data URL failed: %w", err)
	}
	if len(decoded) > maxUploadMediaBytes {
		return "", fmt.Errorf("atlascloud: upload media exceeds %d MiB", maxUploadMediaBytes/(1024*1024))
	}
	exts, _ := mimeExtensions(mimeType)
	filename := "upload"
	if len(exts) > 0 {
		filename += exts[0]
	}
	return uploadMedia(c, info, bytes.NewReader(decoded), filename, mimeType)
}

func UploadFirstFormFile(c *gin.Context, info *relaycommon.RelayInfo, fieldCandidates ...string) (string, error) {
	urls, err := UploadFormFiles(c, info, fieldCandidates...)
	if err != nil || len(urls) == 0 {
		return "", err
	}
	return urls[0], nil
}

func UploadFormFiles(c *gin.Context, info *relaycommon.RelayInfo, fieldCandidates ...string) ([]string, error) {
	if c == nil || c.Request == nil {
		return nil, nil
	}
	mf := c.Request.MultipartForm
	if mf == nil {
		if _, err := c.MultipartForm(); err != nil {
			return nil, fmt.Errorf("atlascloud: parse multipart form failed: %w", err)
		}
		mf = c.Request.MultipartForm
	}
	if mf == nil || len(mf.File) == 0 {
		return nil, nil
	}
	fileHeaders := make([]*multipart.FileHeader, 0)
	for _, key := range fieldCandidates {
		fileHeaders = append(fileHeaders, mf.File[key]...)
	}
	if len(fileHeaders) == 0 {
		for _, files := range mf.File {
			fileHeaders = append(fileHeaders, files...)
		}
	}
	if len(fileHeaders) == 0 {
		return nil, nil
	}
	urls := make([]string, 0, len(fileHeaders))
	for _, fileHeader := range fileHeaders {
		uploaded, err := uploadFormFileHeader(c, info, fileHeader)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(uploaded) != "" {
			urls = append(urls, uploaded)
		}
	}
	return urls, nil
}

func uploadFormFileHeader(c *gin.Context, info *relaycommon.RelayInfo, fileHeader *multipart.FileHeader) (string, error) {
	if fileHeader == nil {
		return "", nil
	}
	if fileHeader.Size > maxUploadMediaBytes {
		return "", fmt.Errorf("atlascloud: upload media exceeds %d MiB", maxUploadMediaBytes/(1024*1024))
	}
	file, err := fileHeader.Open()
	if err != nil {
		return "", fmt.Errorf("atlascloud: open upload media failed: %w", err)
	}
	defer file.Close()
	contentType := strings.TrimSpace(fileHeader.Header.Get("Content-Type"))
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = detectContentType(file, fileHeader.Filename)
	}
	if seeker, ok := file.(io.Seeker); ok {
		_, _ = seeker.Seek(0, io.SeekStart)
	}
	return uploadMedia(c, info, io.LimitReader(file, maxUploadMediaBytes+1), fileHeader.Filename, contentType)
}

func uploadMedia(c *gin.Context, info *relaycommon.RelayInfo, reader io.Reader, filename, contentType string) (string, error) {
	if info == nil {
		return "", fmt.Errorf("atlascloud: relay info is nil")
	}
	EnsureChannelMeta(info)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		_ = writer.Close()
		return "", fmt.Errorf("atlascloud: create upload form failed: %w", err)
	}
	if _, err := io.Copy(part, reader); err != nil {
		_ = writer.Close()
		return "", fmt.Errorf("atlascloud: copy upload media failed: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("atlascloud: close upload form failed: %w", err)
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, BuildAPIURL(info.ChannelBaseUrl, "/api/v1/model/uploadMedia"), &body)
	if err != nil {
		return "", fmt.Errorf("atlascloud: create upload request failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	client, err := service.GetHttpClientWithProxy(info.ChannelSetting.Proxy)
	if err != nil {
		return "", fmt.Errorf("atlascloud: create upload http client failed: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("atlascloud: upload media failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("atlascloud: read upload response failed: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("atlascloud: upload media failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var uploadResp uploadMediaResponse
	if err := common.Unmarshal(respBody, &uploadResp); err != nil {
		return "", fmt.Errorf("atlascloud: decode upload response failed: %w", err)
	}
	mediaURL := uploadMediaURL(uploadResp)
	if mediaURL == "" {
		return "", fmt.Errorf("atlascloud: upload response missing url")
	}
	return mediaURL, nil
}

func uploadMediaURL(resp uploadMediaResponse) string {
	for _, value := range []string{
		resp.URL,
		resp.Data.URL,
		resp.Data.DownloadURL,
		resp.Data.FileURL,
	} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func detectContentType(reader io.Reader, filename string) string {
	if ext := strings.ToLower(filepath.Ext(filename)); ext != "" {
		switch ext {
		case ".jpg", ".jpeg":
			return "image/jpeg"
		case ".png":
			return "image/png"
		case ".webp":
			return "image/webp"
		case ".mp4":
			return "video/mp4"
		}
	}
	buffer := make([]byte, 512)
	n, _ := io.ReadFull(reader, buffer)
	if n > 0 {
		return http.DetectContentType(buffer[:n])
	}
	return "application/octet-stream"
}

func mimeExtensions(mimeType string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg":
		return []string{".jpg"}, nil
	case "image/png":
		return []string{".png"}, nil
	case "image/webp":
		return []string{".webp"}, nil
	default:
		return nil, nil
	}
}

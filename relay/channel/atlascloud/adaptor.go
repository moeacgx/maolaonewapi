package atlascloud

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type Adaptor struct{}

type predictionRateLimitError struct {
	StatusCode int
	Body       string
	Delay      time.Duration
}

func (e *predictionRateLimitError) Error() string {
	return fmt.Sprintf("atlascloud: prediction fetch failed with status %d: %s", e.StatusCode, strings.TrimSpace(e.Body))
}

func (a *Adaptor) Init(_ *relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info == nil {
		return "", errors.New("atlascloud: relay info is nil")
	}
	EnsureChannelMeta(info)
	if info.RequestURLPath == "" {
		return "", errors.New("atlascloud: request path is empty")
	}
	return BuildAPIURL(info.ChannelBaseUrl, info.RequestURLPath), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	if info == nil {
		return errors.New("atlascloud: relay info is nil")
	}
	EnsureChannelMeta(info)
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	req.Set("Content-Type", "application/json")
	req.Set("Accept", "application/json")
	return nil
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info == nil {
		return nil, errors.New("atlascloud: relay info is nil")
	}
	EnsureChannelMeta(info)
	modelName := strings.TrimSpace(info.UpstreamModelName)
	if modelName == "" {
		modelName = strings.TrimSpace(request.Model)
	}
	if modelName == "" {
		modelName = defaultImageModel
	}
	isEdit := info.RelayMode == relayconstant.RelayModeImagesEdits
	modelName = UpstreamImageModelName(modelName, isEdit)
	info.UpstreamModelName = modelName
	info.RequestURLPath = "/api/v1/model/generateImage"

	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" && c != nil {
		prompt = strings.TrimSpace(c.PostForm("prompt"))
	}
	if prompt == "" {
		return nil, errors.New("atlascloud: prompt is required")
	}

	payload := map[string]any{
		"model":  modelName,
		"prompt": prompt,
	}
	if request.Size != "" {
		payload["size"] = request.Size
	}
	if quality := normalizeImageQuality(modelName, isEdit, request.Quality); quality != "" {
		payload["quality"] = quality
	}
	ApplyImagePayloadDefaults(payload, modelName, isEdit)
	if n := lo.FromPtrOr(request.N, uint(0)); n > 1 {
		payload["num_images"] = int(n)
	}

	imageURLs, err := collectImageRequestURLs(c, info, request, isEdit)
	if err != nil {
		return nil, err
	}
	if isEdit {
		if len(imageURLs) == 0 {
			return nil, errors.New("atlascloud: image is required for edits")
		}
		if len(imageURLs) > maxAtlasCloudEditImages {
			return nil, fmt.Errorf("atlascloud: images must contain at most %d items", maxAtlasCloudEditImages)
		}
		if imageEditUsesImageField(modelName) {
			payload["images"] = imageURLs
		} else {
			payload["image_urls"] = imageURLs
		}
	} else if len(imageURLs) > 0 {
		payload["image_url"] = imageURLs[0]
	}

	if err := MergeExtraFields(payload, request.ExtraFields, request.Extra); err != nil {
		return nil, err
	}
	if isEdit {
		applyImageEditPayloadImages(payload, modelName, imageURLs)
	}
	return payload, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	EnsureChannelMeta(info)
	if resp == nil {
		return nil, types.NewError(errors.New("atlascloud: empty response"), types.ErrorCodeBadResponse)
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeReadResponseBodyFailed)
	}
	_ = resp.Body.Close()

	var submitResp apiResponse
	if err := common.Unmarshal(responseBody, &submitResp); err != nil {
		return nil, types.NewError(fmt.Errorf("atlascloud: decode response failed: %w", err), types.ErrorCodeBadResponseBody)
	}
	result := submitResp.Data
	if strings.EqualFold(result.Status, "failed") {
		return nil, types.NewError(fmt.Errorf("atlascloud: %s", ErrorText(result.Error)), types.ErrorCodeBadResponse)
	}
	if !strings.EqualFold(result.Status, "completed") || len(result.Outputs) == 0 {
		predictionID := PredictionID(submitResp)
		if predictionID == "" {
			return nil, types.NewError(errors.New("atlascloud: response missing prediction id"), types.ErrorCodeBadResponseBody)
		}
		polled, pollErr := pollPrediction(c, info, predictionID, imagePollIntervalSec*time.Second, imagePollTimeout(info.UpstreamModelName))
		if pollErr != nil {
			return nil, types.NewError(pollErr, types.ErrorCodeBadResponse)
		}
		result = polled
	}

	imageResponse, err := buildOpenAIImageResponse(result.Outputs, info)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	responseBytes, err := common.Marshal(imageResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(responseBytes)
	return &dto.Usage{}, nil
}

func pollPrediction(c *gin.Context, info *relaycommon.RelayInfo, predictionID string, interval, timeout time.Duration) (predictionData, error) {
	deadline := time.Now().Add(timeout)
	var lastRateLimitErr error
	for {
		if c != nil && c.Request != nil && c.Request.Context().Err() != nil {
			return predictionData{}, c.Request.Context().Err()
		}
		result, err := fetchPrediction(c, info, predictionID)
		if err != nil {
			var rateLimitErr *predictionRateLimitError
			if errors.As(err, &rateLimitErr) {
				lastRateLimitErr = rateLimitErr
				delay := rateLimitErr.Delay
				if delay <= 0 {
					delay = interval
				}
				if delay > atlasCloudPredictionRateLimitMaxDelay {
					delay = atlasCloudPredictionRateLimitMaxDelay
				}
				if !time.Now().Add(delay).Before(deadline) {
					return predictionData{}, fmt.Errorf("atlascloud: prediction %s timed out after rate limit: %w", predictionID, rateLimitErr)
				}
				if sleepErr := sleepWithRequestContext(c, delay); sleepErr != nil {
					return predictionData{}, sleepErr
				}
				continue
			}
			return predictionData{}, err
		}
		switch strings.ToLower(strings.TrimSpace(result.Status)) {
		case "completed":
			if len(result.Outputs) == 0 {
				return predictionData{}, errors.New("atlascloud: completed prediction has no outputs")
			}
			return result, nil
		case "failed", "cancelled", "canceled":
			msg := ErrorText(result.Error)
			if msg == "" {
				msg = "prediction failed"
			}
			return predictionData{}, fmt.Errorf("atlascloud: %s", msg)
		}
		if time.Now().After(deadline) {
			if lastRateLimitErr != nil {
				return predictionData{}, fmt.Errorf("atlascloud: prediction %s timed out after rate limit: %w", predictionID, lastRateLimitErr)
			}
			return predictionData{}, fmt.Errorf("atlascloud: prediction %s timed out", predictionID)
		}
		if err := sleepWithRequestContext(c, interval); err != nil {
			return predictionData{}, err
		}
	}
}

func fetchPrediction(c *gin.Context, info *relaycommon.RelayInfo, predictionID string) (predictionData, error) {
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, BuildAPIURL(info.ChannelBaseUrl, "/api/v1/model/prediction/"+url.PathEscape(predictionID)), nil)
	if err != nil {
		return predictionData{}, err
	}
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	req.Header.Set("Accept", "application/json")
	client, err := service.GetHttpClientWithProxy(info.ChannelSetting.Proxy)
	if err != nil {
		return predictionData{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return predictionData{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return predictionData{}, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if resp.StatusCode == http.StatusTooManyRequests {
			return predictionData{}, &predictionRateLimitError{
				StatusCode: resp.StatusCode,
				Body:       strings.TrimSpace(string(body)),
				Delay:      atlasCloudPredictionRetryDelay(resp.Header, string(body)),
			}
		}
		return predictionData{}, fmt.Errorf("atlascloud: prediction fetch failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var apiResp apiResponse
	if err := common.Unmarshal(body, &apiResp); err != nil {
		return predictionData{}, err
	}
	return apiResp.Data, nil
}

func buildOpenAIImageResponse(outputs []string, info *relaycommon.RelayInfo) (*dto.ImageResponse, error) {
	imageResponse := &dto.ImageResponse{
		Created: common.GetTimestamp(),
		Data:    make([]dto.ImageData, 0, len(outputs)),
	}
	var imageReq *dto.ImageRequest
	if info != nil {
		imageReq, _ = info.Request.(*dto.ImageRequest)
	}
	wantsBase64 := imageReq != nil && strings.EqualFold(imageReq.ResponseFormat, "b64_json")
	for _, output := range outputs {
		output = strings.TrimSpace(output)
		if output == "" {
			continue
		}
		if wantsBase64 {
			data, err := downloadAtlasCloudImageOutput(output, info)
			if err != nil {
				return nil, fmt.Errorf("atlascloud: download image failed: %w", err)
			}
			imageResponse.Data = append(imageResponse.Data, dto.ImageData{B64Json: data})
		} else {
			imageResponse.Data = append(imageResponse.Data, dto.ImageData{Url: output})
		}
	}
	if len(imageResponse.Data) == 0 {
		return nil, errors.New("atlascloud: no usable image output")
	}
	if info != nil {
		info.PriceData.AddOtherRatio("n", float64(len(imageResponse.Data)))
	}
	return imageResponse, nil
}

func sleepWithRequestContext(c *gin.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	if c == nil || c.Request == nil {
		time.Sleep(delay)
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-c.Request.Context().Done():
		return c.Request.Context().Err()
	case <-timer.C:
		return nil
	}
}

func atlasCloudPredictionRetryDelay(headers http.Header, body string) time.Duration {
	if value := strings.TrimSpace(headers.Get("Retry-After")); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil {
			return time.Duration(seconds) * time.Second
		}
		if when, err := http.ParseTime(value); err == nil {
			delay := time.Until(when)
			if delay > 0 {
				return delay
			}
		}
	}
	if seconds, ok := retryAfterSecondsFromText(body); ok {
		return time.Duration(seconds) * time.Second
	}
	return atlasCloudPredictionRateLimitDefaultDelay
}

func retryAfterSecondsFromText(text string) (int, bool) {
	lower := strings.ToLower(text)
	index := strings.Index(lower, "retry after")
	if index < 0 {
		return 0, false
	}
	rest := lower[index+len("retry after"):]
	start := -1
	for i, r := range rest {
		if r >= '0' && r <= '9' {
			start = i
			break
		}
	}
	if start < 0 {
		return 0, false
	}
	end := start
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	seconds, err := strconv.Atoi(rest[start:end])
	if err != nil || seconds <= 0 {
		return 0, false
	}
	return seconds, true
}

func downloadAtlasCloudImageOutput(output string, info *relaycommon.RelayInfo) (string, error) {
	resp, err := service.DoDownloadRequestWithHeaders(output, atlasCloudMediaHeaders(output, info), "atlascloud_image_output")
	if err != nil {
		return "", fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	_, data, err := service.ImageResponseToBase64(resp)
	return data, err
}

func atlasCloudMediaHeaders(output string, info *relaycommon.RelayInfo) map[string]string {
	headers := map[string]string{
		"Accept":     "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8",
		"User-Agent": "Mozilla/5.0 (compatible; NewAPI/AtlasCloudImageFetcher)",
	}
	if isAtlasCloudURL(output) {
		headers["Referer"] = "https://www.atlascloud.ai/"
		if info != nil && strings.TrimSpace(info.ApiKey) != "" {
			headers["Authorization"] = "Bearer " + strings.TrimSpace(info.ApiKey)
		}
	}
	return headers
}

func isAtlasCloudURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	return host == "atlascloud.ai" || strings.HasSuffix(host, ".atlascloud.ai")
}

func collectImageRequestURLs(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest, isEdit bool) ([]string, error) {
	rawURLs, err := imageURLsFromImageRequest(request)
	if err != nil {
		return nil, err
	}
	if isEdit {
		rawURLs = append(rawURLs, imageURLsFromMultipartValues(c)...)
		if isMultipartFormRequest(c) {
			uploaded, err := UploadFormFiles(c, info, "image", "image[]", "images", "images[]")
			if err != nil {
				return nil, err
			}
			rawURLs = append(rawURLs, uploaded...)
		}
	}
	urls := make([]string, 0, len(rawURLs))
	seen := make(map[string]struct{}, len(rawURLs))
	for _, rawURL := range rawURLs {
		imageURL, err := NormalizeImageURL(c, info, rawURL)
		if err != nil {
			return nil, err
		}
		imageURL = strings.TrimSpace(imageURL)
		if imageURL == "" {
			continue
		}
		if _, ok := seen[imageURL]; ok {
			continue
		}
		seen[imageURL] = struct{}{}
		urls = append(urls, imageURL)
	}
	return urls, nil
}

func applyImageEditPayloadImages(payload map[string]any, modelName string, imageURLs []string) {
	delete(payload, "image")
	delete(payload, "image_url")
	if imageEditUsesImageField(modelName) {
		delete(payload, "image_urls")
		payload["images"] = imageURLs
		return
	}
	delete(payload, "images")
	payload["image_urls"] = imageURLs
}

func imageURLsFromImageRequest(request dto.ImageRequest) ([]string, error) {
	urls, err := imageURLsFromRaw(request.Images)
	if err != nil {
		return nil, err
	}
	legacy, err := imageURLsFromRaw(request.Image)
	if err != nil {
		return nil, err
	}
	urls = append(urls, legacy...)
	extraURLs, err := imageURLsFromExtraFields(request.ExtraFields, "images", "image_urls", "image")
	if err != nil {
		return nil, err
	}
	urls = append(urls, extraURLs...)
	for _, key := range []string{"images", "image_urls", "image"} {
		values, err := imageURLsFromRaw(request.Extra[key])
		if err != nil {
			return nil, err
		}
		urls = append(urls, values...)
	}
	return urls, nil
}

func imageURLsFromMultipartValues(c *gin.Context) []string {
	if c == nil || c.Request == nil || c.Request.MultipartForm == nil {
		return nil
	}
	urls := make([]string, 0)
	for _, key := range []string{"image", "image[]", "images", "images[]", "image_urls", "image_urls[]"} {
		for _, value := range c.Request.MultipartForm.Value[key] {
			value = strings.TrimSpace(value)
			if value != "" {
				urls = append(urls, value)
			}
		}
	}
	return urls
}

func imageURLsFromExtraFields(raw json.RawMessage, keys ...string) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	urls := make([]string, 0)
	for _, key := range keys {
		values, err := imageURLsFromRaw(fields[key])
		if err != nil {
			return nil, err
		}
		urls = append(urls, values...)
	}
	return urls, nil
}

func imageURLsFromRaw(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
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
		urls := make([]string, 0, len(array))
		for _, item := range array {
			values, err := imageURLsFromRaw(item)
			if err != nil {
				return nil, err
			}
			urls = append(urls, values...)
		}
		return urls, nil
	}
	var obj map[string]any
	if err := common.Unmarshal(raw, &obj); err != nil {
		return nil, nil
	}
	for _, key := range []string{"url", "image_url"} {
		if value, ok := obj[key].(string); ok && strings.TrimSpace(value) != "" {
			return []string{strings.TrimSpace(value)}, nil
		}
	}
	return nil, nil
}

func (a *Adaptor) GetModelList() []string { return ModelList }
func (a *Adaptor) GetChannelName() string { return ChannelName }

func (a *Adaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("atlascloud: chat completions are not supported")
}
func (a *Adaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("atlascloud: responses are not supported")
}
func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errors.New("atlascloud: rerank is not supported")
}
func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("atlascloud: embeddings are not supported")
}
func (a *Adaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("atlascloud: audio is not supported")
}
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("atlascloud: claude conversion is not supported")
}
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("atlascloud: gemini conversion is not supported")
}

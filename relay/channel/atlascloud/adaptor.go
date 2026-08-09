package atlascloud

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	if n := lo.FromPtrOr(request.N, uint(0)); n > 1 {
		payload["num_images"] = int(n)
	}

	imageURL, err := imageURLFromRaw(request.Image)
	if err != nil {
		return nil, err
	}
	imageURL, err = NormalizeImageURL(c, info, imageURL)
	if err != nil {
		return nil, err
	}
	if imageURL == "" && info.RelayMode == relayconstant.RelayModeImagesEdits {
		imageURL, err = UploadFirstFormFile(c, info, "image", "image[]")
		if err != nil {
			return nil, err
		}
	}
	if imageURL != "" {
		if isEdit {
			if imageEditUsesImageField(modelName) {
				payload["image"] = imageURL
			} else {
				payload["image_urls"] = []string{imageURL}
			}
		} else {
			payload["image_url"] = imageURL
		}
	}
	if isEdit && imageURL == "" {
		return nil, errors.New("atlascloud: image is required for edits")
	}

	if err := MergeExtraFields(payload, request.ExtraFields, request.Extra); err != nil {
		return nil, err
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
	for {
		if c != nil && c.Request != nil && c.Request.Context().Err() != nil {
			return predictionData{}, c.Request.Context().Err()
		}
		result, err := fetchPrediction(c, info, predictionID)
		if err != nil {
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
			return predictionData{}, fmt.Errorf("atlascloud: prediction %s timed out", predictionID)
		}
		time.Sleep(interval)
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
			_, data, err := service.GetImageFromUrl(output)
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

func imageURLFromRaw(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var text string
	if err := common.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text), nil
	}
	var obj map[string]any
	if err := common.Unmarshal(raw, &obj); err != nil {
		return "", nil
	}
	for _, key := range []string{"url", "image_url"} {
		if value, ok := obj[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), nil
		}
	}
	return "", nil
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

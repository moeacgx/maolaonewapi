package atlascloud

type apiResponse struct {
	Code    any            `json:"code,omitempty"`
	Message string         `json:"message,omitempty"`
	Error   any            `json:"error,omitempty"`
	URL     string         `json:"url,omitempty"`
	Data    predictionData `json:"data,omitempty"`
}

type uploadMediaResponse struct {
	URL  string          `json:"url,omitempty"`
	Data uploadMediaData `json:"data,omitempty"`
}

type uploadMediaData struct {
	URL         string `json:"url,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
	FileURL     string `json:"file_url,omitempty"`
}

type predictionData struct {
	ID       string   `json:"id,omitempty"`
	TaskID   string   `json:"task_id,omitempty"`
	Status   string   `json:"status,omitempty"`
	Outputs  []string `json:"outputs,omitempty"`
	Error    any      `json:"error,omitempty"`
	Progress any      `json:"progress,omitempty"`
	Model    string   `json:"model,omitempty"`
}

type APIResponse = apiResponse
type PredictionData = predictionData

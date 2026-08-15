package atlascloud

import "time"

const (
	ChannelName = "atlascloud"

	ModelGrokImage                            = "grok-imagine-image"
	ModelGrokImagePro                         = "grok-imagine-image-pro"
	ModelGrok2Image                           = "grok-2-image-1212"
	ModelGrokVideo                            = "grok-imagine-video"
	ModelGrokVideo15                          = "grok-imagine-video-1.5"
	ModelGPTImage1                            = "gpt-image-1"
	ModelGPTImage15                           = "gpt-image-1.5"
	ModelGPTImage2                            = "gpt-image-2"
	ModelSora2                                = "sora-2"
	ModelSora2Pro                             = "sora-2-pro"
	defaultImageModel                         = "seedream-3.0"
	imagePollIntervalSec                      = 2
	imagePollTimeoutSec                       = 120
	gptImage2PollTimeoutSec                   = 300
	atlasCloudPredictionRateLimitDefaultDelay = 4 * time.Second
	atlasCloudPredictionRateLimitMaxDelay     = 30 * time.Second
	maxUploadMediaBytes                       = 25 * 1024 * 1024
	maxAtlasCloudEditImages                   = 10
	maxAtlasCloudImageOutputs                 = 10
)

var ModelList = []string{
	ModelGrokImage,
	ModelGrokVideo,
	ModelGrokVideo15,
	ModelGPTImage1,
	ModelGPTImage15,
	ModelGPTImage2,
}

package priceformula

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/expr-lang/expr"
)

type ImageDimension struct {
	Width  int
	Height int
}

type Config struct {
	Expression string
	Variables  map[string]float64
	Defaults   map[string]string
}

type Input struct {
	BasePrice             float64
	Dimensions            map[string]string
	Params                map[string]float64
	InputImages           []ImageDimension
	EstimatedPromptTokens int
	PromptChars           int
}

type Result struct {
	Price     float64
	Variables map[string]float64
	Quality   string
}

var formulaVariableNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func Validate(config Config) error {
	_, err := evaluate(config, Input{Dimensions: map[string]string{"resolution": "1024x1024", "quality": "medium"}})
	return err
}

func Evaluate(config Config, input Input) (Result, error) {
	return evaluate(config, input)
}

func evaluate(config Config, input Input) (Result, error) {
	expression := strings.TrimSpace(config.Expression)
	if expression == "" {
		return Result{}, fmt.Errorf("formula expression cannot be empty")
	}
	env, numericVars, quality, err := buildEnv(config, input)
	if err != nil {
		return Result{}, err
	}
	program, err := expr.Compile(expression, expr.Env(env), expr.AsFloat64())
	if err != nil {
		return Result{}, fmt.Errorf("formula compile error: %w", err)
	}
	output, err := expr.Run(program, env)
	if err != nil {
		return Result{}, fmt.Errorf("formula run error: %w", err)
	}
	price, ok := output.(float64)
	if !ok {
		return Result{}, fmt.Errorf("formula result is %T, want float64", output)
	}
	if math.IsNaN(price) || math.IsInf(price, 0) || price < 0 {
		return Result{}, fmt.Errorf("formula result is invalid: %v", price)
	}
	return Result{Price: price, Variables: numericVars, Quality: quality}, nil
}

func buildEnv(config Config, input Input) (map[string]any, map[string]float64, string, error) {
	resolution := firstNonEmpty(
		input.Dimensions["resolution"],
		config.Defaults["size"],
		config.Defaults["resolution"],
	)
	width, height := parseResolution(resolution)
	if width <= 0 || height <= 0 {
		width, height = parseResolution(config.Defaults["output_resolution"])
	}
	shortSide := math.Min(float64(width), float64(height))
	longSide := math.Max(float64(width), float64(height))
	quality := strings.ToLower(strings.TrimSpace(firstNonEmpty(input.Dimensions["quality"], config.Defaults["quality"])))

	inputImages := sanitizeImages(input.InputImages)
	inputImageCount := input.Params["input_images"]
	if inputImageCount < float64(len(inputImages)) {
		inputImageCount = float64(len(inputImages))
	}
	if inputImageCount > float64(len(inputImages)) {
		fallbackW, fallbackH := parseResolution(firstNonEmpty(config.Defaults["input_image_fallback_resolution"], config.Defaults["fallback_resolution"]))
		for len(inputImages) < int(math.Ceil(inputImageCount)) && fallbackW > 0 && fallbackH > 0 {
			inputImages = append(inputImages, ImageDimension{Width: fallbackW, Height: fallbackH})
		}
	}

	numericVars := map[string]float64{
		"base_price":              input.BasePrice,
		"width":                   float64(width),
		"height":                  float64(height),
		"short_side":              shortSide,
		"long_side":               longSide,
		"pixels":                  float64(width * height),
		"input_images":            inputImageCount,
		"input_image_count":       inputImageCount,
		"prompt_tokens_estimated": float64(input.EstimatedPromptTokens),
		"prompt_chars":            float64(input.PromptChars),
	}
	for key, value := range input.Params {
		key = normalizeName(key)
		if key == "" || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		numericVars[key] = value
	}
	for key, value := range config.Variables {
		key = normalizeName(key)
		if key == "" {
			return nil, nil, "", fmt.Errorf("formula variable name cannot be empty")
		}
		if !formulaVariableNamePattern.MatchString(key) {
			return nil, nil, "", fmt.Errorf("formula variable %s is invalid", key)
		}
		if _, reserved := reservedEnvNames()[key]; reserved {
			return nil, nil, "", fmt.Errorf("formula variable %s is reserved", key)
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, nil, "", fmt.Errorf("formula variable %s is invalid", key)
		}
		numericVars[key] = value
	}

	env := make(map[string]any, len(numericVars)+16)
	for key, value := range numericVars {
		env[key] = value
	}
	env["quality"] = quality
	env["max"] = math.Max
	env["min"] = math.Min
	env["abs"] = math.Abs
	env["ceil"] = math.Ceil
	env["floor"] = math.Floor
	env["round"] = math.Round
	env["area_tokens"] = func(base float64, w float64, h float64) float64 {
		return areaTokens(base, w, h)
	}
	env["output_tokens"] = func(base float64) float64 {
		return areaTokens(base, float64(width), float64(height))
	}
	env["input_image_tokens"] = func(base float64) float64 {
		var total float64
		for _, img := range inputImages {
			total += areaTokens(base, float64(img.Width), float64(img.Height))
		}
		return total
	}
	return env, numericVars, quality, nil
}

func areaTokens(base, width, height float64) float64 {
	if base <= 0 || width <= 0 || height <= 0 {
		return 0
	}
	shortSide := math.Min(width, height)
	longSide := math.Max(width, height)
	if longSide <= 0 {
		return 0
	}
	return math.Ceil(base * math.Round(base*shortSide/longSide) * (2_000_000 + width*height) / 4_000_000)
}

func parseResolution(value string) (int, int) {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "*", "x")
	parts := strings.Split(value, "x")
	if len(parts) != 2 {
		return 0, 0
	}
	var width, height int
	_, _ = fmt.Sscanf(strings.TrimSpace(parts[0]), "%d", &width)
	_, _ = fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &height)
	if width <= 0 || height <= 0 {
		return 0, 0
	}
	return width, height
}

func sanitizeImages(images []ImageDimension) []ImageDimension {
	if len(images) == 0 {
		return nil
	}
	sanitized := make([]ImageDimension, 0, len(images))
	for _, image := range images {
		if image.Width <= 0 || image.Height <= 0 {
			continue
		}
		sanitized = append(sanitized, image)
	}
	return sanitized
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, ".", "_")
	return value
}

func reservedEnvNames() map[string]struct{} {
	return map[string]struct{}{
		"base_price":              {},
		"width":                   {},
		"height":                  {},
		"short_side":              {},
		"long_side":               {},
		"pixels":                  {},
		"input_images":            {},
		"input_image_count":       {},
		"prompt_tokens_estimated": {},
		"prompt_chars":            {},
		"quality":                 {},
		"max":                     {},
		"min":                     {},
		"abs":                     {},
		"ceil":                    {},
		"floor":                   {},
		"round":                   {},
		"area_tokens":             {},
		"output_tokens":           {},
		"input_image_tokens":      {},
	}
}

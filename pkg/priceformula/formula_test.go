package priceformula

import (
	"math"
	"testing"
)

func TestEvaluateAtlasCloudGPTImage2EditSample(t *testing.T) {
	result, err := Evaluate(Config{
		Expression: `(input_image_tokens(input_base) * input_image_token_price + output_tokens(quality == "high" ? high_output_base : quality == "low" ? low_output_base : medium_output_base) * output_token_price + text_input_price) * currency_rate`,
		Variables: map[string]float64{
			"input_base":              48,
			"low_output_base":         16,
			"medium_output_base":      48,
			"high_output_base":        96,
			"input_image_token_price": 0.000008,
			"output_token_price":      0.00003,
			"text_input_price":        0.005,
			"currency_rate":           6.74,
		},
		Defaults: map[string]string{
			"size":    "1024x1024",
			"quality": "medium",
		},
	}, Input{
		Dimensions: map[string]string{
			"resolution": "2304x3072",
			"quality":    "high",
		},
		InputImages: []ImageDimension{{Width: 1196, Height: 1200}},
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if math.Abs(result.Price-3.31231908) > 1e-9 {
		t.Fatalf("price = %.12f, want 3.31231908", result.Price)
	}
	if result.Variables["width"] != 2304 || result.Variables["height"] != 3072 {
		t.Fatalf("dimensions = %#v", result.Variables)
	}
	if result.Quality != "high" {
		t.Fatalf("quality = %q, want high", result.Quality)
	}
	inputTokens := areaTokens(48, 1196, 1200)
	outputTokens := areaTokens(96, 2304, 3072)
	subtotal := inputTokens*0.000008 + outputTokens*0.00003 + 0.005
	breakdownWants := map[string]float64{
		"input_image_tokens": inputTokens,
		"input_image_cost":   inputTokens * 0.000008,
		"output_base":        96,
		"output_tokens":      outputTokens,
		"output_cost":        outputTokens * 0.00003,
		"text_input_cost":    0.005,
		"subtotal":           subtotal,
		"currency_rate":      6.74,
		"converted_total":    subtotal * 6.74,
	}
	for key, want := range breakdownWants {
		if math.Abs(result.Breakdown[key]-want) > 1e-9 {
			t.Fatalf("breakdown[%s] = %.12f, want %.12f", key, result.Breakdown[key], want)
		}
	}
}

func TestEvaluateBreakdownForInputImageSurchargeFormula(t *testing.T) {
	result, err := Evaluate(Config{
		Expression: `base_price + max(input_images - input_base, 0) * input_image_unit_price`,
		Variables: map[string]float64{
			"input_base":             1,
			"input_image_unit_price": 0.01,
		},
		Defaults: map[string]string{
			"size":    "1024x1024",
			"quality": "medium",
		},
	}, Input{
		BasePrice: 0.32,
		Params: map[string]float64{
			"input_images": 3,
		},
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if math.Abs(result.Price-0.34) > 1e-9 {
		t.Fatalf("price = %.12f, want 0.34", result.Price)
	}
	if result.Breakdown["base_price"] != 0.32 {
		t.Fatalf("base_price = %v, want 0.32", result.Breakdown["base_price"])
	}
	if result.Breakdown["input_image_extra_units"] != 2 {
		t.Fatalf("input_image_extra_units = %v, want 2", result.Breakdown["input_image_extra_units"])
	}
	if math.Abs(result.Breakdown["input_image_surcharge"]-0.02) > 1e-9 {
		t.Fatalf("input_image_surcharge = %v, want 0.02", result.Breakdown["input_image_surcharge"])
	}
	if math.Abs(result.Breakdown["subtotal"]-0.34) > 1e-9 {
		t.Fatalf("subtotal = %v, want 0.34", result.Breakdown["subtotal"])
	}
}

func TestEvaluateUsesDefaultsAndFallbackInputImageResolution(t *testing.T) {
	result, err := Evaluate(Config{
		Expression: `output_tokens(48) + input_image_tokens(48)`,
		Defaults: map[string]string{
			"size":                            "1024x1024",
			"quality":                         "medium",
			"input_image_fallback_resolution": "2048x2048",
		},
	}, Input{
		Params: map[string]float64{
			"input_images": 2,
		},
	})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	want := areaTokens(48, 1024, 1024) + 2*areaTokens(48, 2048, 2048)
	if math.Abs(result.Price-want) > 1e-9 {
		t.Fatalf("price = %.12f, want %.12f", result.Price, want)
	}
	if result.Variables["input_images"] != 2 {
		t.Fatalf("input_images = %v, want 2", result.Variables["input_images"])
	}
}

func TestValidateRejectsInvalidFormula(t *testing.T) {
	tests := []Config{
		{Expression: `unknown_func(1)`},
		{
			Expression: `width`,
			Variables:  map[string]float64{"width": 1},
		},
		{
			Expression: `custom`,
			Variables:  map[string]float64{"bad-name!": 1},
		},
	}
	for _, config := range tests {
		if err := Validate(config); err == nil {
			t.Fatalf("Validate(%#v) error = nil", config)
		}
	}
}

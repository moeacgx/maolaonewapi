package types

import (
	"math"
	"testing"
)

func TestApplyTaskRatiosToFloatUsesSecondsForPerSecondPrice(t *testing.T) {
	price := PriceData{
		UsePrice:       true,
		ModelPriceUnit: ModelPriceUnitSecond,
	}
	price.AddOtherRatio("seconds", 8)
	price.AddOtherRatio("resolution", 1.4)

	if got := price.ApplyTaskRatiosToFloat(100); got != 1120 {
		t.Fatalf("ApplyTaskRatiosToFloat() = %v, want 1120", got)
	}
}

func TestApplyTaskRatiosToFloatSkipsSecondsForPerRequestPrice(t *testing.T) {
	price := PriceData{
		UsePrice:       true,
		ModelPriceUnit: ModelPriceUnitRequest,
	}
	price.AddOtherRatio("seconds", 8)
	price.AddOtherRatio("resolution", 1.4)

	if got := price.ApplyTaskRatiosToFloat(100); got != 140 {
		t.Fatalf("ApplyTaskRatiosToFloat() = %v, want 140", got)
	}
}

func TestApplyTaskRatiosToFloatKeepsLegacyRatioBilling(t *testing.T) {
	price := PriceData{
		UsePrice: false,
	}
	price.AddOtherRatio("seconds", 8)

	if got := price.ApplyTaskRatiosToFloat(100); got != 800 {
		t.Fatalf("ApplyTaskRatiosToFloat() = %v, want 800", got)
	}
}

func TestOtherRatiosSnapshotFiltersInvalidAndIsImmutable(t *testing.T) {
	price := PriceData{}
	price.AddOtherRatio("n", 2)
	price.AddOtherRatio("zero", 0)
	price.AddOtherRatio("nan", math.NaN())

	snapshot := price.OtherRatios()
	if len(snapshot) != 1 || snapshot["n"] != 2 {
		t.Fatalf("snapshot = %#v, want only n=2", snapshot)
	}
	snapshot["n"] = 99
	if got, _ := price.GetOtherRatio("n"); got != 2 {
		t.Fatalf("internal ratio mutated through snapshot: got %v, want 2", got)
	}

	if price.ReplaceOtherRatios(map[string]float64{"bad": -1}) {
		t.Fatal("ReplaceOtherRatios should reject all-invalid input")
	}
	if got := price.OtherRatios(); got != nil {
		t.Fatalf("OtherRatios() = %#v, want nil", got)
	}
}

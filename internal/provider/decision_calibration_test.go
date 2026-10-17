package provider

import (
	"math"
	"testing"
)

// A model whose winner is overconfident: it says 95/5 where the truth is 70/30.
// The fit should recover the T that maps a logit gap of ln(19) onto ln(7/3):
// T = ln(19) / ln(7/3) ≈ 3.476.
func TestFitTemperatureRecoversT(t *testing.T) {
	over := map[string]float64{"a": 0.95, "b": 0.05}
	var points []CalibPoint
	for i := 0; i < 100; i++ {
		expected := "a"
		if i%10 >= 7 { // exactly 30 of the labels are the runner-up
			expected = "b"
		}
		points = append(points, CalibPoint{Probabilities: over, Expected: expected})
	}

	temp, before, after := FitTemperature(points)
	want := math.Log(0.95/0.05) / math.Log(0.7/0.3)
	if math.Abs(temp-want) > 0.2 {
		t.Errorf("fitted T = %.3f, want ≈ %.3f", temp, want)
	}
	if after > before {
		t.Errorf("NLL should improve with the fit: before %f after %f", before, after)
	}
}

func TestFitTemperatureKeepsOneOnCalibratedInput(t *testing.T) {
	// 50/50 predictions with 50% correct labels: already calibrated.
	points := []CalibPoint{
		{Probabilities: map[string]float64{"a": 0.5, "b": 0.5}, Expected: "a"},
		{Probabilities: map[string]float64{"a": 0.5, "b": 0.5}, Expected: "b"},
	}
	temp, _, _ := FitTemperature(points)
	if temp != 1 {
		t.Errorf("an already-calibrated model should keep T=1, got %f", temp)
	}
}

func TestReliabilityBinning(t *testing.T) {
	points := []CalibPoint{
		// Confidence 0.9, both right → the 0.9 bin reads 100% accuracy.
		{Probabilities: map[string]float64{"a": 0.9, "b": 0.1}, Expected: "a"},
		{Probabilities: map[string]float64{"a": 0.9, "b": 0.1}, Expected: "a"},
		// Confidence 0.9, wrong → third of the bin wrong.
		{Probabilities: map[string]float64{"a": 0.9, "b": 0.1}, Expected: "b"},
		// Confidence 0.6, right.
		{Probabilities: map[string]float64{"a": 0.6, "b": 0.4}, Expected: "a"},
	}
	bins := Reliability(points, 1, 10)
	var hi, mid *ReliabilityBin
	for i := range bins {
		if bins[i].Count == 0 {
			continue
		}
		if bins[i].Lo >= 0.9 {
			hi = &bins[i]
		} else if bins[i].Lo >= 0.5 {
			mid = &bins[i]
		}
	}
	if hi == nil || mid == nil {
		t.Fatalf("expected filled bins at 0.9 and 0.6, got %+v", bins)
	}
	if hi.Accuracy != 2.0/3.0 {
		t.Errorf("0.9-bin accuracy = %f, want 2/3", hi.Accuracy)
	}
	if mid.Accuracy != 1 {
		t.Errorf("0.6-bin accuracy = %f, want 1", mid.Accuracy)
	}

	if acc := AccuracyAt(points, 1); acc != 0.75 {
		t.Errorf("accuracy = %f, want 0.75", acc)
	}
}

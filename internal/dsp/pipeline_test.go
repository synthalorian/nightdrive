package dsp

import (
	"math"
	"testing"
)

func TestNewPipeline(t *testing.T) {
	p := NewPipeline()
	if p == nil {
		t.Fatal("NewPipeline returned nil")
	}
	if !p.Enabled {
		t.Error("expected pipeline to be enabled by default")
	}
	if p.EQ == nil {
		t.Error("expected EQ to be initialized")
	}
	if p.Compressor == nil {
		t.Error("expected Compressor to be initialized")
	}
	if p.Loudness == nil {
		t.Error("expected Loudness to be initialized")
	}
}

func TestPipelineProcessDisabled(t *testing.T) {
	p := NewPipeline()
	p.Enabled = false

	samples := []float32{0.5, -0.5, 0.5, -0.5}
	result := p.Process(samples, 44100)

	if len(result) != len(samples) {
		t.Errorf("expected %d samples, got %d", len(samples), len(result))
	}
	for i := range samples {
		if result[i] != samples[i] {
			t.Errorf("sample %d: expected %f, got %f", i, samples[i], result[i])
		}
	}
}

func TestPipelineProcessEmpty(t *testing.T) {
	p := NewPipeline()
	result := p.Process([]float32{}, 44100)
	if len(result) != 0 {
		t.Errorf("expected 0 samples, got %d", len(result))
	}
}

func TestPipelineApplyPreset(t *testing.T) {
	p := NewPipeline()
	preset := &Preset{
		Name:                "Test",
		EQLow:               3.0,
		EQMid:               -2.0,
		EQHigh:              1.0,
		CompressorThreshold: -15,
		CompressorRatio:     3,
		LoudnessTarget:      -16,
	}

	p.ApplyPreset(preset)
	// Preset is applied - processing should work without panic
	samples := make([]float32, 1024)
	for i := range samples {
		samples[i] = float32(math.Sin(float64(i) * 0.1))
	}
	result := p.Process(samples, 44100)
	if len(result) != len(samples) {
		t.Errorf("expected %d samples, got %d", len(samples), len(result))
	}
}

func TestPipelineApplyPresetNil(t *testing.T) {
	p := NewPipeline()
	p.ApplyPreset(nil) // Should not panic
}

func TestEqualizer(t *testing.T) {
	eq := NewEqualizer()
	if eq == nil {
		t.Fatal("NewEqualizer returned nil")
	}

	// Test gain setting with clamping
	eq.SetLowGain(20)
	if eq.lowGain != 12 {
		t.Errorf("expected low gain clamped to 12, got %f", eq.lowGain)
	}

	eq.SetLowGain(-20)
	if eq.lowGain != -12 {
		t.Errorf("expected low gain clamped to -12, got %f", eq.lowGain)
	}

	eq.SetMidGain(5)
	if eq.midGain != 5 {
		t.Errorf("expected mid gain 5, got %f", eq.midGain)
	}

	eq.SetHighGain(-3)
	if eq.highGain != -3 {
		t.Errorf("expected high gain -3, got %f", eq.highGain)
	}
}

func TestEqualizerProcess(t *testing.T) {
	eq := NewEqualizer()
	eq.SetLowGain(6)
	eq.SetMidGain(-3)
	eq.SetHighGain(2)

	// Create stereo samples
	samples := []float32{0.5, -0.5, 0.3, -0.3, 0.1, -0.1}
	result := eq.Process(samples, 44100)

	if len(result) != len(samples) {
		t.Errorf("expected %d samples, got %d", len(samples), len(result))
	}

	// All output should be in valid range
	for i, s := range result {
		if s > 1.0 || s < -1.0 {
			t.Errorf("sample %d out of range: %f", i, s)
		}
	}
}

func TestEqualizerProcessZeroSampleRate(t *testing.T) {
	eq := NewEqualizer()
	samples := []float32{0.5, -0.5}
	result := eq.Process(samples, 0)

	// Should return original samples when sampleRate is 0
	for i := range samples {
		if result[i] != samples[i] {
			t.Errorf("sample %d: expected %f, got %f", i, samples[i], result[i])
		}
	}
}

func TestCompressor(t *testing.T) {
	c := NewCompressor()
	if c == nil {
		t.Fatal("NewCompressor returned nil")
	}

	// Test default values
	if c.thresholdDB != -20 {
		t.Errorf("expected threshold -20, got %f", c.thresholdDB)
	}
	if c.ratio != 4 {
		t.Errorf("expected ratio 4, got %f", c.ratio)
	}
}

func TestCompressorSetters(t *testing.T) {
	c := NewCompressor()

	c.SetThreshold(-30)
	if c.thresholdDB != -30 {
		t.Errorf("expected threshold -30, got %f", c.thresholdDB)
	}

	c.SetRatio(25)
	if c.ratio != 20 {
		t.Errorf("expected ratio clamped to 20, got %f", c.ratio)
	}

	c.SetRatio(0.5)
	if c.ratio != 1 {
		t.Errorf("expected ratio clamped to 1, got %f", c.ratio)
	}

	c.SetMakeupGain(6)
	if c.makeupGain != 6 {
		t.Errorf("expected makeup gain 6, got %f", c.makeupGain)
	}
}

func TestCompressorProcess(t *testing.T) {
	c := NewCompressor()

	// Create samples with varying amplitude
	samples := []float32{0.1, -0.1, 0.5, -0.5, 0.9, -0.9}
	result := c.Process(samples)

	if len(result) != len(samples) {
		t.Errorf("expected %d samples, got %d", len(samples), len(result))
	}

	// High amplitude samples should be compressed
	// (output should be less than input for loud signals)
	for i := range result {
		if result[i] > 1.0 || result[i] < -1.0 {
			t.Errorf("sample %d out of range: %f", i, result[i])
		}
	}
}

func TestLoudnessNormalizer(t *testing.T) {
	ln := NewLoudnessNormalizer()
	if ln == nil {
		t.Fatal("NewLoudnessNormalizer returned nil")
	}
	if ln.targetLUFS != -14 {
		t.Errorf("expected target -14 LUFS, got %f", ln.targetLUFS)
	}
}

func TestLoudnessNormalizerSetTarget(t *testing.T) {
	ln := NewLoudnessNormalizer()
	ln.SetTarget(-20)
	if ln.targetLUFS != -20 {
		t.Errorf("expected target -20 LUFS, got %f", ln.targetLUFS)
	}
}

func TestLoudnessNormalizerProcess(t *testing.T) {
	ln := NewLoudnessNormalizer()

	// Create stereo samples
	samples := []float32{0.5, 0.5, 0.5, 0.5, 0.5, 0.5}
	result := ln.Process(samples)

	if len(result) != len(samples) {
		t.Errorf("expected %d samples, got %d", len(samples), len(result))
	}

	// Output should be in valid range
	for i, s := range result {
		if s > 1.0 || s < -1.0 {
			t.Errorf("sample %d out of range: %f", i, s)
		}
	}
}

func TestLoudnessNormalizerProcessEmpty(t *testing.T) {
	ln := NewLoudnessNormalizer()
	result := ln.Process([]float32{})
	if len(result) != 0 {
		t.Errorf("expected 0 samples, got %d", len(result))
	}
}

func TestMeasureLoudness(t *testing.T) {
	// Empty samples
	lufs := MeasureLoudness([]float32{})
	if lufs != -70 {
		t.Errorf("expected -70 for empty, got %f", lufs)
	}

	// Silent samples
	lufs = MeasureLoudness([]float32{0.0, 0.0, 0.0, 0.0})
	if lufs != -70 {
		t.Errorf("expected -70 for silence, got %f", lufs)
	}

	// Loud samples
	samples := []float32{0.5, 0.5, 0.5, 0.5}
	lufs = MeasureLoudness(samples)
	if lufs <= -70 {
		t.Error("expected loudness above -70 for loud signal")
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		v, min, max, want float64
	}{
		{5, 0, 10, 5},
		{-5, 0, 10, 0},
		{15, 0, 10, 10},
		{5, 5, 5, 5},
	}

	for _, tt := range tests {
		got := clamp(tt.v, tt.min, tt.max)
		if got != tt.want {
			t.Errorf("clamp(%f, %f, %f) = %f, want %f", tt.v, tt.min, tt.max, got, tt.want)
		}
	}
}

func TestClampFloat(t *testing.T) {
	// clampFloat is an alias for clamp
	if clampFloat(15, 0, 10) != 10 {
		t.Error("clampFloat should behave like clamp")
	}
}

func TestPipelineFullChain(t *testing.T) {
	p := NewPipeline()
	preset := &Preset{
		Name:                "Full Test",
		EQLow:               6,
		EQMid:               -3,
		EQHigh:              2,
		CompressorThreshold: -20,
		CompressorRatio:     4,
		LoudnessTarget:      -14,
	}
	p.ApplyPreset(preset)

	// Generate a sine wave
	sampleRate := 44100
	numSamples := 4096
	samples := make([]float32, numSamples)
	for i := 0; i < numSamples; i += 2 {
		val := float32(math.Sin(float64(i) * 2 * math.Pi * 440 / float64(sampleRate)))
		samples[i] = val
		samples[i+1] = val
	}

	result := p.Process(samples, sampleRate)

	if len(result) != len(samples) {
		t.Fatalf("expected %d samples, got %d", len(samples), len(result))
	}

	// Measure output loudness
	outputLUFS := MeasureLoudness(result)
	if outputLUFS > 0 {
		t.Errorf("output loudness should not be positive, got %f", outputLUFS)
	}

	// All samples should be in valid range
	for i, s := range result {
		if s > 1.0 || s < -1.0 {
			t.Fatalf("sample %d out of range: %f", i, s)
		}
	}
}

func BenchmarkPipelineProcess(b *testing.B) {
	p := NewPipeline()
	preset := &Preset{
		EQLow:               3,
		EQMid:               -2,
		EQHigh:              1,
		CompressorThreshold: -20,
		CompressorRatio:     4,
		LoudnessTarget:      -14,
	}
	p.ApplyPreset(preset)

	sampleRate := 44100
	numSamples := 4096
	samples := make([]float32, numSamples)
	for i := 0; i < numSamples; i += 2 {
		val := float32(math.Sin(float64(i) * 2 * math.Pi * 440 / float64(sampleRate)))
		samples[i] = val
		samples[i+1] = val
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Process(samples, sampleRate)
	}
}

func BenchmarkEqualizerProcess(b *testing.B) {
	eq := NewEqualizer()
	eq.SetLowGain(6)
	eq.SetMidGain(-3)
	eq.SetHighGain(2)

	sampleRate := 44100
	numSamples := 4096
	samples := make([]float32, numSamples)
	for i := range samples {
		samples[i] = float32(math.Sin(float64(i) * 0.1))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eq.Process(samples, sampleRate)
	}
}

func BenchmarkCompressorProcess(b *testing.B) {
	c := NewCompressor()

	numSamples := 4096
	samples := make([]float32, numSamples)
	for i := range samples {
		samples[i] = float32(math.Sin(float64(i) * 0.1))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Process(samples)
	}
}

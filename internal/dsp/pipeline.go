package dsp

import (
	"math"
)

// Preset represents a DSP preset with EQ, compression, and loudness settings.
type Preset struct {
	ID                int64   `json:"id"`
	Name              string  `json:"name"`
	UserID            int64   `json:"userId"`
	EQLow             float64 `json:"eqLow"`
	EQMid             float64 `json:"eqMid"`
	EQHigh            float64 `json:"eqHigh"`
	CompressorThreshold float64 `json:"compressorThreshold"`
	CompressorRatio   float64 `json:"compressorRatio"`
	LoudnessTarget    float64 `json:"loudnessTarget"`
	IsDefault         bool    `json:"isDefault"`
}

// Pipeline represents the DSP processing chain.
type Pipeline struct {
	EQ         *Equalizer
	Compressor *Compressor
	Loudness   *LoudnessNormalizer
	Enabled    bool
}

// NewPipeline creates a DSP pipeline with default settings.
func NewPipeline() *Pipeline {
	return &Pipeline{
		EQ:         NewEqualizer(),
		Compressor: NewCompressor(),
		Loudness:   NewLoudnessNormalizer(),
		Enabled:    true,
	}
}

// ApplyPreset configures the pipeline from a preset.
func (p *Pipeline) ApplyPreset(preset *Preset) {
	if preset == nil {
		return
	}
	p.EQ.SetLowGain(preset.EQLow)
	p.EQ.SetMidGain(preset.EQMid)
	p.EQ.SetHighGain(preset.EQHigh)
	p.Compressor.SetThreshold(preset.CompressorThreshold)
	p.Compressor.SetRatio(preset.CompressorRatio)
	p.Loudness.SetTarget(preset.LoudnessTarget)
}

// Process applies the full DSP chain to a sample buffer.
// Samples should be interleaved float32 stereo data: [L, R, L, R, ...]
func (p *Pipeline) Process(samples []float32, sampleRate int) []float32 {
	if !p.Enabled || len(samples) == 0 {
		return samples
	}

	// Apply EQ
	samples = p.EQ.Process(samples, sampleRate)

	// Apply compression
	samples = p.Compressor.Process(samples)

	// Apply loudness normalization
	samples = p.Loudness.Process(samples)

	return samples
}

// Equalizer implements a 3-band parametric EQ (low, mid, high).
type Equalizer struct {
	lowGain  float64
	midGain  float64
	highGain float64

	// Filter state for low shelf
	lowX1, lowX2, lowY1, lowY2 float64
	// Filter state for peaking EQ (mid)
	midX1, midX2, midY1, midY2 float64
	// Filter state for high shelf
	highX1, highX2, highY1, highY2 float64
}

// NewEqualizer creates a new 3-band equalizer.
func NewEqualizer() *Equalizer {
	return &Equalizer{
		lowGain:  0,
		midGain:  0,
		highGain: 0,
	}
}

// SetLowGain sets the low shelf gain in dB (-12 to +12).
func (e *Equalizer) SetLowGain(gainDB float64) {
	e.lowGain = clamp(gainDB, -12, 12)
}

// SetMidGain sets the mid peaking gain in dB (-12 to +12).
func (e *Equalizer) SetMidGain(gainDB float64) {
	e.midGain = clamp(gainDB, -12, 12)
}

// SetHighGain sets the high shelf gain in dB (-12 to +12).
func (e *Equalizer) SetHighGain(gainDB float64) {
	e.highGain = clamp(gainDB, -12, 12)
}

// Process applies EQ to interleaved float32 stereo samples.
func (e *Equalizer) Process(samples []float32, sampleRate int) []float32 {
	if sampleRate == 0 {
		return samples
	}

	// Low shelf: 250 Hz cutoff
	lowCoeffs := e.shelfCoeffs(250, e.lowGain, sampleRate, true)
	// Peaking EQ: 1000 Hz center
	midCoeffs := e.peakingCoeffs(1000, e.midGain, sampleRate, 1.0)
	// High shelf: 4000 Hz cutoff
	highCoeffs := e.shelfCoeffs(4000, e.highGain, sampleRate, false)

	out := make([]float32, len(samples))
	for i := 0; i < len(samples); i++ {
		s := float64(samples[i])

		// Low shelf
		lowY := lowCoeffs.a0*s + lowCoeffs.a1*e.lowX1 + lowCoeffs.a2*e.lowX2 -
			lowCoeffs.b1*e.lowY1 - lowCoeffs.b2*e.lowY2
		e.lowX2, e.lowX1 = e.lowX1, s
		e.lowY2, e.lowY1 = e.lowY1, lowY

		// Mid peaking
		midY := midCoeffs.a0*lowY + midCoeffs.a1*e.midX1 + midCoeffs.a2*e.midX2 -
			midCoeffs.b1*e.midY1 - midCoeffs.b2*e.midY2
		e.midX2, e.midX1 = e.midX1, lowY
		e.midY2, e.midY1 = e.midY1, midY

		// High shelf
		highY := highCoeffs.a0*midY + highCoeffs.a1*e.highX1 + highCoeffs.a2*e.highX2 -
			highCoeffs.b1*e.highY1 - highCoeffs.b2*e.highY2
		e.highX2, e.highX1 = e.highX1, midY
		e.highY2, e.highY1 = e.highY1, highY

		out[i] = float32(clampFloat(highY, -1, 1))
	}
	return out
}

type biquadCoeffs struct {
	a0, a1, a2, b1, b2 float64
}

func (e *Equalizer) shelfCoeffs(freq, gainDB float64, sampleRate int, isLow bool) biquadCoeffs {
	A := math.Pow(10, gainDB/40)
	w0 := 2 * math.Pi * freq / float64(sampleRate)
	cosw0 := math.Cos(w0)
	sinw0 := math.Sin(w0)
	sqrt2A := math.Sqrt(2) * A

	var a0, a1, a2, b0, b1, b2 float64
	if isLow {
		s := math.Sqrt((A*A+1)*(1/A-1) + 2*sqrt2A*sinw0/2)
		if s < 0.0001 {
			s = 0.0001
		}
		alpha := sinw0 / 2 * math.Sqrt((A*A+1)*(1/A-1)+2*sqrt2A*sinw0/2)
		b0 = A * ((A + 1) - (A-1)*cosw0 + 2*sqrt2A*alpha)
		b1 = 2 * A * ((A - 1) - (A+1)*cosw0)
		b2 = A * ((A + 1) - (A-1)*cosw0 - 2*sqrt2A*alpha)
		a0 = (A + 1) + (A-1)*cosw0 + 2*sqrt2A*alpha
		a1 = -2 * ((A - 1) + (A+1)*cosw0)
		a2 = (A + 1) + (A-1)*cosw0 - 2*sqrt2A*alpha
	} else {
		alpha := sinw0 / 2 * math.Sqrt((A*A+1)*(1/A-1)+2*sqrt2A*sinw0/2)
		b0 = A * ((A + 1) + (A-1)*cosw0 + 2*sqrt2A*alpha)
		b1 = -2 * A * ((A - 1) + (A+1)*cosw0)
		b2 = A * ((A + 1) + (A-1)*cosw0 - 2*sqrt2A*alpha)
		a0 = (A + 1) - (A-1)*cosw0 + 2*sqrt2A*alpha
		a1 = 2 * ((A - 1) - (A+1)*cosw0)
		a2 = (A + 1) - (A-1)*cosw0 - 2*sqrt2A*alpha
	}

	return biquadCoeffs{
		a0: b0 / a0,
		a1: b1 / a0,
		a2: b2 / a0,
		b1: a1 / a0,
		b2: a2 / a0,
	}
}

func (e *Equalizer) peakingCoeffs(freq, gainDB float64, sampleRate int, Q float64) biquadCoeffs {
	A := math.Pow(10, gainDB/40)
	w0 := 2 * math.Pi * freq / float64(sampleRate)
	cosw0 := math.Cos(w0)
	sinw0 := math.Sin(w0)
	alpha := sinw0 / (2 * Q)

	b0 := 1 + alpha*A
	b1 := -2 * cosw0
	b2 := 1 - alpha*A
	a0 := 1 + alpha/A
	a1 := -2 * cosw0
	a2 := 1 - alpha/A

	return biquadCoeffs{
		a0: b0 / a0,
		a1: b1 / a0,
		a2: b2 / a0,
		b1: a1 / a0,
		b2: a2 / a0,
	}
}

// Compressor implements a simple feedforward dynamic range compressor.
type Compressor struct {
	thresholdDB float64
	ratio       float64
	attackMs    float64
	releaseMs   float64
	makeupGain  float64

	envelope    float64
	sampleRate  int
}

// NewCompressor creates a compressor with sensible defaults.
func NewCompressor() *Compressor {
	return &Compressor{
		thresholdDB: -20,
		ratio:       4,
		attackMs:    5,
		releaseMs:   50,
		makeupGain:  0,
	}
}

// SetThreshold sets the threshold in dB.
func (c *Compressor) SetThreshold(thresholdDB float64) {
	c.thresholdDB = thresholdDB
}

// SetRatio sets the compression ratio.
func (c *Compressor) SetRatio(ratio float64) {
	c.ratio = clamp(ratio, 1, 20)
}

// SetMakeupGain sets the makeup gain in dB.
func (c *Compressor) SetMakeupGain(gainDB float64) {
	c.makeupGain = gainDB
}

// Process applies compression to interleaved float32 stereo samples.
func (c *Compressor) Process(samples []float32) []float32 {
	if c.sampleRate == 0 {
		c.sampleRate = 44100 // Default
	}

	attackCoeff := math.Exp(-1.0 / (float64(c.sampleRate) * c.attackMs / 1000.0))
	releaseCoeff := math.Exp(-1.0 / (float64(c.sampleRate) * c.releaseMs / 1000.0))
	thresholdLinear := math.Pow(10, c.thresholdDB/20)
	makeupLinear := math.Pow(10, c.makeupGain/20)

	out := make([]float32, len(samples))
	for i := 0; i < len(samples); i++ {
		s := float64(samples[i])
		absS := math.Abs(s)

		// Update envelope
		if absS > c.envelope {
			c.envelope = attackCoeff*c.envelope + (1-attackCoeff)*absS
		} else {
			c.envelope = releaseCoeff*c.envelope + (1-releaseCoeff)*absS
		}

		// Calculate gain reduction
		gain := 1.0
		if c.envelope > thresholdLinear {
			excess := c.envelope / thresholdLinear
			gain = math.Pow(excess, 1.0/c.ratio-1.0)
		}

		out[i] = float32(s * gain * makeupLinear)
	}
	return out
}

// LoudnessNormalizer implements EBU R128-style loudness normalization.
type LoudnessNormalizer struct {
	targetLUFS float64
	currentLU  float64
	gain       float64
}

// NewLoudnessNormalizer creates a loudness normalizer targeting -14 LUFS.
func NewLoudnessNormalizer() *LoudnessNormalizer {
	return &LoudnessNormalizer{
		targetLUFS: -14,
		gain:       1.0,
	}
}

// SetTarget sets the target loudness in LUFS.
func (l *LoudnessNormalizer) SetTarget(targetLUFS float64) {
	l.targetLUFS = targetLUFS
}

// Process applies loudness normalization to interleaved float32 stereo samples.
func (l *LoudnessNormalizer) Process(samples []float32) []float32 {
	if len(samples) == 0 {
		return samples
	}

	// Estimate short-term loudness (simplified)
	var sumSquares float64
	for i := 0; i < len(samples); i += 2 {
		lSample := float64(samples[i])
		rSample := float64(samples[i+1])
		// K-weighted approximation (simplified)
		sumSquares += lSample*lSample + rSample*rSample
	}

	frames := float64(len(samples) / 2)
	if frames > 0 {
		rms := math.Sqrt(sumSquares / frames)
		if rms > 0.00001 {
			currentLUFS := 20 * math.Log10(rms)
			diff := l.targetLUFS - currentLUFS
			// Smooth gain adjustment
			targetGain := math.Pow(10, diff/20)
			l.gain = 0.9*l.gain + 0.1*targetGain
		}
	}

	out := make([]float32, len(samples))
	for i := 0; i < len(samples); i++ {
		out[i] = float32(float64(samples[i]) * l.gain)
		if out[i] > 1.0 {
			out[i] = 1.0
		} else if out[i] < -1.0 {
			out[i] = -1.0
		}
	}
	return out
}

// MeasureLoudness returns the estimated LUFS of a sample buffer.
func MeasureLoudness(samples []float32) float64 {
	if len(samples) == 0 {
		return -70
	}
	var sumSquares float64
	for _, s := range samples {
		s64 := float64(s)
		sumSquares += s64 * s64
	}
	rms := math.Sqrt(sumSquares / float64(len(samples)))
	if rms < 0.00001 {
		return -70
	}
	return 20 * math.Log10(rms)
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func clampFloat(v, min, max float64) float64 {
	return clamp(v, min, max)
}

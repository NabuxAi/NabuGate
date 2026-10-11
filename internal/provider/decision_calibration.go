package provider

import "math"

// Temperature-scaling calibration for the local decision engine, as in the
// article: raw token probabilities are overconfident (99.8% "confidence" on an
// ambiguous question), so a single temperature T is fitted on labelled examples
// by minimising the negative log-likelihood of the correct option under
// softmax(logits / T), and the fitted T is applied to every later decision.

// CalibPoint is one labelled observation: the option probabilities a decision
// produced (already softmaxed, so logits are recovered as ln p) and the key
// that was actually correct.
type CalibPoint struct {
	Probabilities map[string]float64
	Expected      string
}

// logitOf recovers the (unnormalised) logit of an option from its probability.
func logitOf(p float64) float64 {
	return math.Log(math.Max(p, decisionEpsilon))
}

// probabilityAt applies temperature T to the point's implied logits and returns
// the scaled probability of key.
func probabilityAt(p CalibPoint, key string, t float64) float64 {
	var sum float64
	for _, pk := range p.Probabilities {
		sum += math.Exp(logitOf(pk) / t)
	}
	if sum <= 0 {
		return 0
	}
	return math.Exp(logitOf(p.Probabilities[key])/t) / sum
}

// nllAt is the mean negative log-likelihood of the correct options at T.
func nllAt(points []CalibPoint, t float64) float64 {
	var sum float64
	for _, p := range points {
		sum -= math.Log(math.Max(probabilityAt(p, p.Expected, t), decisionEpsilon))
	}
	return sum / float64(len(points))
}

// goldenSection minimises a unimodal f over [lo, hi]. Interior points are
// recomputed from the shrinking interval every iteration: reusing stale
// coordinates from before a shrink is the classic way this search quietly
// probes outside [a, b].
func goldenSection(lo, hi float64, f func(float64) float64) float64 {
	const invPhi = 0.6180339887498949
	a, b := lo, hi
	for b-a > 1e-3 {
		c := b - invPhi*(b-a)
		d := a + invPhi*(b-a)
		if f(c) < f(d) {
			b = d
		} else {
			a = c
		}
	}
	return (a + b) / 2
}

// FitTemperature finds the calibration temperature that best aligns the
// engine's confidence with its accuracy on the labelled points. It returns the
// fitted T (≥ 1) plus the mean NLL before and after scaling, so the caller can
// show how much the fit improved.
func FitTemperature(points []CalibPoint) (t, nllBefore, nllAfter float64) {
	if len(points) == 0 {
		return 1, 0, 0
	}
	before := nllAt(points, 1)
	t = goldenSection(1, 20, func(x float64) float64 { return nllAt(points, x) })
	// A fit that does not beat T=1 (possible on an already-calibrated model)
	// is not a fit: keep T=1 rather than flattening for nothing.
	after := nllAt(points, t)
	if after >= before {
		return 1, before, before
	}
	return t, before, after
}

// ReliabilityBin is one bucket of the reliability diagram: of the decisions
// whose confidence fell in [lo, hi), how many were right.
type ReliabilityBin struct {
	Lo            float64 `json:"lo"`
	Hi            float64 `json:"hi"`
	Count         int     `json:"count"`
	AvgConfidence float64 `json:"avg_confidence"`
	Accuracy      float64 `json:"accuracy"`
}

// Reliability bins confidence against correctness. T rescales the
// probabilities first (T=1 is the raw engine output).
func Reliability(points []CalibPoint, t float64, nbins int) []ReliabilityBin {
	if nbins < 1 {
		nbins = 10
	}
	scaled := make([]CalibPoint, len(points))
	for i, p := range points {
		sp := make(map[string]float64, len(p.Probabilities))
		var sum float64
		for k, pk := range p.Probabilities {
			sp[k] = math.Exp(logitOf(pk) / t)
			sum += sp[k]
		}
		for k := range sp {
			sp[k] /= sum
		}
		scaled[i] = CalibPoint{Probabilities: sp, Expected: p.Expected}
	}

	bins := make([]ReliabilityBin, nbins)
	for i := range bins {
		bins[i].Lo = float64(i) / float64(nbins)
		bins[i].Hi = float64(i+1) / float64(nbins)
	}
	var confSum, accSum []float64
	confSum = make([]float64, nbins)
	accSum = make([]float64, nbins)

	for _, p := range scaled {
		conf, correct := -1.0, false
		for k, pk := range p.Probabilities {
			if pk > conf {
				conf, correct = pk, k == p.Expected
			}
		}
		if conf < 0 {
			continue
		}
		idx := int(conf * float64(nbins))
		if idx >= nbins {
			idx = nbins - 1
		}
		bins[idx].Count++
		confSum[idx] += conf
		if correct {
			accSum[idx]++
		}
	}
	for i := range bins {
		if bins[i].Count > 0 {
			bins[i].AvgConfidence = confSum[i] / float64(bins[i].Count)
			bins[i].Accuracy = accSum[i] / float64(bins[i].Count)
		}
	}
	return bins
}

// AccuracyAt is the fraction of points whose argmax option is the expected one
// at temperature T (argmax order changes only for T→0/∞, but report it anyway).
func AccuracyAt(points []CalibPoint, t float64) float64 {
	if len(points) == 0 {
		return 0
	}
	var right int
	for _, p := range points {
		best, bestP := "", -1.0
		for k := range p.Probabilities {
			if pk := probabilityAt(p, k, t); pk > bestP {
				bestP, best = pk, k
			}
		}
		if best == p.Expected {
			right++
		}
	}
	return float64(right) / float64(len(points))
}

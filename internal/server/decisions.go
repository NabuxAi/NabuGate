package server

import (
	"encoding/json"
	"net/http"

	"nabugate/internal/provider"
)

type decisionRequestBody struct {
	Model       string          `json:"model"`
	State       json.RawMessage `json:"state"`
	Questions   json.RawMessage `json:"questions"`
	Temperature *float64        `json:"temperature"`
	Engine      string          `json:"engine"`
}

func (s *Server) handleDecisions(w http.ResponseWriter, r *http.Request) {
	var body decisionRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Model == "" {
		writeError(w, http.StatusBadRequest, "field 'model' (alias) is required")
		return
	}
	if len(body.State) == 0 {
		writeError(w, http.StatusBadRequest, "field 'state' is required")
		return
	}
	if len(body.Questions) == 0 {
		writeError(w, http.StatusBadRequest, "field 'questions' is required")
		return
	}
	if body.Engine != "" && body.Engine != "auto" && body.Engine != "local" && body.Engine != "native" {
		writeError(w, http.StatusBadRequest, "field 'engine' must be one of: auto, local, native")
		return
	}
	if body.Temperature != nil && (*body.Temperature <= 0 || *body.Temperature > 100) {
		writeError(w, http.StatusBadRequest, "field 'temperature' must be in (0, 100]")
		return
	}

	if !s.aliasAllowed(w, r, body.Model) {
		return
	}

	result, err := s.router.Decide(r.Context(), body.Model, provider.DecisionRequest{
		Model:       body.Model,
		State:       body.State,
		Questions:   body.Questions,
		Temperature: body.Temperature,
		Engine:      body.Engine,
	})
	if err != nil {
		writeError(w, aliasErrStatus(err, "unknown decision alias"), err.Error())
		return
	}

	// The Decide route has no adapter-side fallback: an upstream that answered
	// without a usage object would otherwise be metered at an indifferent
	// zero. Estimate from the payload actually sent — state and questions —
	// and the answers that came back, flagged as a guess.
	usage := result.Usage
	if usage.TotalTokens == 0 {
		pTok := provider.EstimateTokens(len(body.State) + len(body.Questions))
		cTok := provider.EstimateTokens(len(result.Answers))
		usage = provider.Usage{
			PromptTokens:     pTok,
			CompletionTokens: cTok,
			TotalTokens:      pTok + cTok,
			Estimated:        true,
		}
	}
	cost := s.record(r, result.Provider, result.Model, usage)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Nabu-Provider", result.Provider)
	w.Header().Set("X-Nabu-Model", result.Model)
	w.Header().Set("X-Nabu-Engine", result.Engine)
	// The charged amount rides the same header every other metered response
	// sets, so a caller reconciling their spend reads one place.
	w.Header().Set(costHeader, formatCost(cost))
	usageOut := map[string]any{
		"input_tokens":  usage.PromptTokens,
		"output_tokens": usage.CompletionTokens,
		"total_tokens":  usage.TotalTokens,
		// Additive: what the caller was charged, alongside the token counts.
		"cost_usd": cost,
	}
	if usage.Estimated {
		usageOut["estimated"] = true
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"model":   result.Model,
		"engine":  result.Engine,
		"answers": result.Answers,
		"usage":   usageOut,
	})
}

// calibrateSample is one labelled decision: the state/questions pair plus the
// answer key that was actually correct for each question.
type calibrateSample struct {
	State     json.RawMessage   `json:"state"`
	Questions json.RawMessage   `json:"questions"`
	Expected  map[string]string `json:"expected"`
}

type calibrateRequestBody struct {
	Model   string            `json:"model"`
	Engine  string            `json:"engine"`
	Samples []calibrateSample `json:"samples"`
}

// decisionAnswer is the slice of the engine's per-question answer the fit needs.
type decisionAnswer struct {
	Choice        *string            `json:"choice"`
	Noul          *float64           `json:"noul"`
	Probabilities map[string]float64 `json:"probabilities"`
	Error         string             `json:"error"`
}

// maxCalibrateSamples bounds a calibration run: every sample is a full
// decision (one forward pass per question), so a runaway list is real money.
const maxCalibrateSamples = 300

// handleDecisionCalibrate — POST /v1/decisions/calibrate. Runs the requested
// alias over labelled samples at temperature 1, fits the calibration
// temperature that minimises the correct option's NLL, and returns the
// reliability diagram before and after so the caller can see the improvement.
// The fitted temperature is then sent as "temperature" on normal decisions.
func (s *Server) handleDecisionCalibrate(w http.ResponseWriter, r *http.Request) {
	var body calibrateRequestBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Model == "" {
		writeError(w, http.StatusBadRequest, "field 'model' (alias) is required")
		return
	}
	if len(body.Samples) == 0 {
		writeError(w, http.StatusBadRequest, "field 'samples' must be a non-empty array of labelled decisions")
		return
	}
	if len(body.Samples) > maxCalibrateSamples {
		writeError(w, http.StatusBadRequest, "too many samples (max 300)")
		return
	}
	if body.Engine != "" && body.Engine != "auto" && body.Engine != "local" && body.Engine != "native" {
		writeError(w, http.StatusBadRequest, "field 'engine' must be one of: auto, local, native")
		return
	}

	if !s.aliasAllowed(w, r, body.Model) {
		return
	}

	one := 1.0
	var points []provider.CalibPoint
	var usage provider.Usage
	engine, upstreamModel := "", ""
	for _, sample := range body.Samples {
		if len(sample.State) == 0 || len(sample.Questions) == 0 || len(sample.Expected) == 0 {
			writeError(w, http.StatusBadRequest, "every sample needs state, questions and expected")
			return
		}
		sampleCtx := freshKeySource(r.Context())
		result, err := s.router.Decide(sampleCtx, body.Model, provider.DecisionRequest{
			Model:       body.Model,
			State:       sample.State,
			Questions:   sample.Questions,
			Temperature: &one,
			Engine:      body.Engine,
		})
		if err != nil {
			writeError(w, aliasErrStatus(err, "unknown decision alias"), err.Error())
			return
		}
		engine, upstreamModel = result.Engine, result.Model
		usage.PromptTokens += result.Usage.PromptTokens
		usage.CompletionTokens += result.Usage.CompletionTokens
		usage.TotalTokens += result.Usage.TotalTokens
		s.record(servedAs(r, servedByCaller(sampleCtx)), result.Provider, result.Model, result.Usage)

		var answers map[string]decisionAnswer
		if err := json.Unmarshal(result.Answers, &answers); err != nil {
			continue
		}
		for qid, expected := range sample.Expected {
			a, ok := answers[qid]
			if !ok || len(a.Probabilities) < 2 || a.Error != "" {
				continue
			}
			if _, ok := a.Probabilities[expected]; !ok {
				continue
			}
			points = append(points, provider.CalibPoint{Probabilities: a.Probabilities, Expected: expected})
		}
	}

	if len(points) == 0 {
		writeError(w, http.StatusBadRequest, "no usable labelled answers came back (each sample needs a question whose expected key appears in its probabilities)")
		return
	}

	t, nllBefore, nllAfter := provider.FitTemperature(points)

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, http.StatusOK, map[string]any{
		"model":       upstreamModel,
		"engine":      engine,
		"samples":     len(body.Samples),
		"points":      len(points),
		"temperature": t,
		"nll_before":  nllBefore,
		"nll_after":   nllAfter,
		"accuracy":    provider.AccuracyAt(points, t),
		"bins_before": provider.Reliability(points, 1, 10),
		"bins_after":  provider.Reliability(points, t, 10),
		"usage": map[string]any{
			"input_tokens":  usage.PromptTokens,
			"output_tokens": usage.CompletionTokens,
			"total_tokens":  usage.TotalTokens,
		},
	})
}

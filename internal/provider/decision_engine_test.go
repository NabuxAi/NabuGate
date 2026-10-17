package provider

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// logprobUpstream fakes an OpenAI-wire /chat/completions that answers with the
// given top logprobs for its single generated token.
func logprobUpstream(t *testing.T, top map[string]float64, requests *[]map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests != nil {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			*requests = append(*requests, body)
		}
		toks := make([]string, 0, len(top))
		for tok := range top {
			toks = append(toks, tok)
		}
		entries := make([]string, 0, len(top))
		for _, tok := range toks {
			entries = append(entries, `{"token":`+quoteJSON(tok)+`,"logprob":`+jsonNumber(top[tok])+`}`)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"content": "A"}, "logprobs": {"content": [{"top_logprobs": [` + strings.Join(entries, ",") + `]}]}}],
			"usage": {"prompt_tokens": 7, "completion_tokens": 1}
		}`))
	}))
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func jsonNumber(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}

func TestDecideLocalConstrainedSoftmax(t *testing.T) {
	var bodies []map[string]any
	ts := logprobUpstream(t, map[string]float64{"A": -0.001, "B": -4.0, "C": -9.0, "the": -0.5}, &bodies)
	defer ts.Close()
	a := NewOpenAIAdapter("fake", ts.URL, "k", nil)

	resp, err := a.DecideLocal(context.Background(), DecisionRequest{
		Model:     "m",
		State:     json.RawMessage(`"What color is the sky?"`),
		Questions: json.RawMessage(`{"color": {"type": "choice", "instructions": "pick", "criteria": {"red": "stop signs", "blue": "sky", "green": "grass"}}}`),
	})
	if err != nil {
		t.Fatalf("DecideLocal: %v", err)
	}

	// Exactly one forward pass for one question, letters-only.
	if len(bodies) != 1 {
		t.Fatalf("expected 1 upstream call, got %d", len(bodies))
	}
	if bodies[0]["logprobs"] != true || bodies[0]["top_logprobs"] != float64(20) {
		t.Errorf("expected logprobs+top_logprobs on the request, got %v", bodies[0])
	}

	var answers map[string]struct {
		Choice        string             `json:"choice"`
		Probabilities map[string]float64 `json:"probabilities"`
		Confidence    float64            `json:"confidence"`
	}
	if err := json.Unmarshal(resp.Answers, &answers); err != nil {
		t.Fatalf("decode answers: %v", err)
	}
	a1 := answers["color"]
	// Criteria keys are sorted, so the letters run blue→A, green→B, red→C.
	if a1.Choice != "blue" {
		t.Errorf("expected choice blue, got %s", a1.Choice)
	}
	// Constrained softmax over A,B,C only: "the" must not count.
	pA, pB, pC := a1.Probabilities["blue"], a1.Probabilities["green"], a1.Probabilities["red"]
	sum := pA + pB + pC
	if math.Abs(sum-1) > 1e-9 {
		t.Errorf("renormalised probabilities must sum to 1, got %f", sum)
	}
	expA := math.Exp(-0.001) / (math.Exp(-0.001) + math.Exp(-4.0) + math.Exp(-9.0))
	if math.Abs(pA-expA) > 1e-9 {
		t.Errorf("p(blue) = %f, want %f", pA, expA)
	}
	if math.Abs(a1.Confidence-pA) > 1e-9 {
		t.Errorf("confidence should equal the winner's probability")
	}
	if resp.Usage.PromptTokens != 7 || resp.Usage.CompletionTokens != 1 {
		t.Errorf("usage not carried: %+v", resp.Usage)
	}
}

func TestDecideLocalTemperatureFlattens(t *testing.T) {
	ts := logprobUpstream(t, map[string]float64{"A": -0.001, "B": -4.0}, nil)
	defer ts.Close()
	a := NewOpenAIAdapter("fake", ts.URL, "k", nil)

	tt := 3.8
	resp, err := a.DecideLocal(context.Background(), DecisionRequest{
		Model:       "m",
		State:       json.RawMessage(`"s"`),
		Questions:   json.RawMessage(`{"q": {"type": "choice", "criteria": {"x": "one", "y": "two"}}}`),
		Temperature: &tt,
	})
	if err != nil {
		t.Fatalf("DecideLocal: %v", err)
	}
	var answers map[string]struct {
		Probabilities map[string]float64 `json:"probabilities"`
	}
	_ = json.Unmarshal(resp.Answers, &answers)
	px, py := answers["q"].Probabilities["x"], answers["q"].Probabilities["y"]
	// Logit gap ln(p) = 3.999 divided by 3.8 ≈ 1.052 → ratio e^1.052 ≈ 2.86.
	want := math.Exp((-0.001 + 4.0) / 3.8)
	if math.Abs(px/py-want) > 1e-6 {
		t.Errorf("temperature not applied: ratio %f, want %f", px/py, want)
	}
	if px < 0.6 || px > 0.8 {
		t.Errorf("flattened winner probability out of the expected band: %f", px)
	}
}

func TestDecideLocalNoulAndScore(t *testing.T) {
	// " B" (leading space) and lowercase "b" variants must map to option B.
	ts := logprobUpstream(t, map[string]float64{" B": -0.7, "b": -1.0, "A": -0.9}, nil)
	defer ts.Close()
	a := NewOpenAIAdapter("fake", ts.URL, "k", nil)

	resp, err := a.DecideLocal(context.Background(), DecisionRequest{
		Model: "m",
		State: json.RawMessage(`{"note": "json state"}`),
		Questions: json.RawMessage(`{
			"fact": {"type": "noul", "instructions": "true or false?"},
			"sev": {"type": "score", "criteria": ["Low", "Mid", "High"]}
		}`),
	})
	if err != nil {
		t.Fatalf("DecideLocal: %v", err)
	}
	var answers map[string]json.RawMessage
	_ = json.Unmarshal(resp.Answers, &answers)

	var fact struct {
		Noul          float64            `json:"noul"`
		Probabilities map[string]float64 `json:"probabilities"`
	}
	if err := json.Unmarshal(answers["fact"], &fact); err != nil {
		t.Fatal(err)
	}
	// Noul's letters are fixed: true→A, false→B. Best logprobs put A at -0.9
	// and B (best of " B"/"b") at -0.7, so false wins narrowly:
	// p_true = 1/(1+e^{0.2}) ≈ 0.45.
	if fact.Noul < 0.4 || fact.Noul > 0.5 {
		t.Errorf("noul = %f, want ≈ 0.45", fact.Noul)
	}
	if len(fact.Probabilities) != 2 {
		t.Errorf("noul must have exactly true/false probabilities, got %v", fact.Probabilities)
	}

	var sev struct {
		Score  float64           `json:"score"`
		Legend map[string]string `json:"legend"`
	}
	if err := json.Unmarshal(answers["sev"], &sev); err != nil {
		t.Fatal(err)
	}
	// Mid (B) carries e^-0.7 against Low's e^-0.9; High never appears in the
	// top tokens, so the score is the renormalised mass on index 1 ≈ 0.55.
	if sev.Score < 0.5 || sev.Score > 0.6 {
		t.Errorf("score = %f, want ≈ 0.55", sev.Score)
	}
	if sev.Legend["1"] != "Mid" {
		t.Errorf("legend should name the levels: %v", sev.Legend)
	}
}

func TestDecideLocalNoLetterIsPerQuestionError(t *testing.T) {
	ts := logprobUpstream(t, map[string]float64{"yes": -0.1, "no": -0.3}, nil)
	defer ts.Close()
	a := NewOpenAIAdapter("fake", ts.URL, "k", nil)

	resp, err := a.DecideLocal(context.Background(), DecisionRequest{
		Model:     "m",
		State:     json.RawMessage(`"s"`),
		Questions: json.RawMessage(`{"q": {"type": "choice", "criteria": {"x": "one", "y": "two"}}}`),
	})
	if err == nil {
		t.Fatalf("all questions failed: expected the underlying error, got %+v", resp)
	}
}

func TestDecideLocalBadQuestionShape(t *testing.T) {
	ts := logprobUpstream(t, map[string]float64{"A": -0.1}, nil)
	defer ts.Close()
	a := NewOpenAIAdapter("fake", ts.URL, "k", nil)

	resp, err := a.DecideLocal(context.Background(), DecisionRequest{
		Model:     "m",
		State:     json.RawMessage(`"s"`),
		Questions: json.RawMessage(`{"q": {"type": "choice", "criteria": {"x": "one"}},"weird": {"type": "essay"}}`),
	})
	if err != nil {
		t.Fatalf("one bad question must not fail the request: %v", err)
	}
	var answers map[string]struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(resp.Answers, &answers)
	if answers["weird"].Error == "" {
		t.Errorf("unsupported type should carry a per-question error")
	}
}

func TestFirstLetterOf(t *testing.T) {
	cases := map[string]rune{
		"A": 'A', " A": 'A', "**A": 'A', "a": 'A', " b": 'B', "B:": 'B', "A)": 'A',
		"the": 0, "Because": 0, "12": 0, "": 0,
	}
	for tok, want := range cases {
		if got, _ := firstLetterOf(tok); got != want {
			t.Errorf("firstLetterOf(%q) = %q, want %q", tok, got, want)
		}
	}
}

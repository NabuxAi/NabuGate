package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"nabugate/internal/adminstore"
	"nabugate/internal/flow"
	"nabugate/internal/provider"
)

// ─────────────────────────────── flows ──────────────────────────────────────
//
// The console half of flows, mirroring the sub-agent handlers above it. Baked
// flows (config/YAML) are read-only here; console-managed ones are editable and
// persist across restarts.
//
// This is what makes a flow something the person who runs a business builds,
// rather than something that needs a deploy: the whole point of chaining
// specialists is that which specialists, and in what order, is the business's
// question and not the gateway's.

// loadManagedFlows registers the console-created flows into the live registry
// (upsert, so a console flow wins over a same-named baked one). Called when the
// admin store is attached and after every mutation, so a newly saved flow is
// callable immediately rather than after the next restart.
func (s *Server) loadManagedFlows() {
	if s.admin == nil || s.flows == nil {
		return
	}
	for _, rec := range s.admin.Flows() {
		_ = s.flows.Set(recordToFlow(rec))
	}
}

func recordToFlow(rec adminstore.FlowRecord) flow.Flow {
	steps := make([]flow.Step, 0, len(rec.Steps))
	for _, st := range rec.Steps {
		steps = append(steps, flow.Step{
			Agent:    st.Agent,
			Label:    st.Label,
			Input:    st.Input,
			Optional: st.Optional,
		})
	}

	return flow.Flow{Name: rec.Name, Description: rec.Description, Steps: steps}
}

// listFlows surfaces every flow — baked and console-managed — with the steps
// the editor needs, plus what a step is allowed to name, so the editor can
// offer a list rather than a free-text box somebody will typo.
func (s *Server) listFlows(w http.ResponseWriter, _ *http.Request) {
	managed := map[string]bool{}
	if s.admin != nil {
		for _, rec := range s.admin.Flows() {
			managed[rec.Name] = true
		}
	}

	type stepInfo struct {
		Agent    string `json:"agent"`
		Label    string `json:"label,omitempty"`
		Input    string `json:"input,omitempty"`
		Optional bool   `json:"optional,omitempty"`
	}
	type flowInfo struct {
		Name        string     `json:"name"`
		Description string     `json:"description"`
		Steps       []stepInfo `json:"steps"`
		Editable    bool       `json:"editable"`
	}

	out := make([]flowInfo, 0)
	for _, name := range s.flows.Names() {
		f, ok := s.flows.Lookup(name)
		if !ok {
			continue
		}
		steps := make([]stepInfo, 0, len(f.Steps))
		for _, st := range f.Steps {
			steps = append(steps, stepInfo{Agent: st.Agent, Label: st.Label, Input: st.Input, Optional: st.Optional})
		}
		out = append(out, flowInfo{
			Name: f.Name, Description: f.Description, Steps: steps, Editable: managed[f.Name],
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"flows":      out,
		"selectable": s.selectableStepTargets(),
	})
}

// selectableStepTargets is everything a step may legally name today.
func (s *Server) selectableStepTargets() []map[string]string {
	out := make([]map[string]string, 0)
	for _, name := range s.agents.Names() {
		out = append(out, map[string]string{"id": name, "kind": "agent"})
	}
	for _, name := range s.flows.Names() {
		out = append(out, map[string]string{"id": name, "kind": "flow"})
	}
	for _, alias := range s.router.Aliases() {
		out = append(out, map[string]string{"id": alias, "kind": "alias"})
	}

	return out
}

type flowRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Steps       []struct {
		Agent    string `json:"agent"`
		Label    string `json:"label"`
		Input    string `json:"input"`
		Optional bool   `json:"optional"`
	} `json:"steps"`
}

// saveFlow creates (POST) or updates (PATCH) a console-managed flow and
// registers it live.
func (s *Server) saveFlow(w http.ResponseWriter, r *http.Request) {
	var req flowRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if name := r.PathValue("name"); name != "" {
		req.Name = name // PATCH takes the name from the path
	}

	rec := adminstore.FlowRecord{Name: strings.TrimSpace(req.Name), Description: req.Description}
	for _, st := range req.Steps {
		rec.Steps = append(rec.Steps, adminstore.FlowStepRecord{
			Agent: st.Agent, Label: st.Label, Input: st.Input, Optional: st.Optional,
		})
	}

	// Refused before it is saved rather than when somebody calls it: a step
	// naming nothing is a model name that answers 502 to its first caller, and
	// by then whoever wrote it has moved on.
	if err := s.validateFlowSteps(rec); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.admin.SaveFlow(rec); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.loadManagedFlows()
	writeJSON(w, http.StatusOK, map[string]any{"flow": rec})
}

// validateFlowSteps checks each step names something that exists and that the
// flow does not name itself.
//
// Only direct self-reference is caught here; a longer cycle is caught at run
// time by name. Checking the whole graph on save would refuse the legitimate
// order of writing two flows that call each other's predecessors.
func (s *Server) validateFlowSteps(rec adminstore.FlowRecord) error {
	for i, st := range rec.Steps {
		name := strings.TrimSpace(st.Agent)
		if name == "" {
			return fmt.Errorf("step %d names no agent", i+1)
		}
		if name == rec.Name {
			return fmt.Errorf("step %d names this flow itself", i+1)
		}
		if _, ok := s.agents.Lookup(name); ok {
			continue
		}
		if _, ok := s.flows.Lookup(name); ok {
			continue
		}
		if s.router.KnowsAlias(name) {
			continue
		}

		return fmt.Errorf("step %d names %q, which is not an agent, a flow or a model alias", i+1, name)
	}

	return nil
}

// deleteFlow removes a console-managed flow. Baked flows cannot be deleted
// here — edit their YAML in the repo.
func (s *Server) deleteFlow(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	managed := false
	for _, rec := range s.admin.Flows() {
		if rec.Name == name {
			managed = true
			break
		}
	}
	if !managed {
		writeError(w, http.StatusBadRequest, "this flow is defined in config; edit its YAML in the repo")
		return
	}

	if err := s.admin.DeleteFlow(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.flows != nil {
		s.flows.Remove(name)
	}

	w.WriteHeader(http.StatusNoContent)
}

// testFlow runs one message through a whole flow and returns every step's
// output, so an admin can see where a chain goes wrong rather than only that it
// did. Without the middle, a bad answer and a bad third step look identical.
func (s *Server) testFlow(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	fl, ok := s.flows.Lookup(name)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown flow")
		return
	}

	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<18)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	req := provider.ChatRequest{Messages: []provider.Message{{Role: "user", Content: body.Message}}}

	result, err := s.runFlow(r.Context(), fl, req, 1, nil)
	if err != nil {
		// The steps that did run are returned alongside the error: a chain that
		// died at step three is diagnosed by reading steps one and two.
		writeJSON(w, http.StatusOK, map[string]any{
			"flow":  fl.Name,
			"error": err.Error(),
			"steps": result.Steps,
		})

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"flow":   fl.Name,
		"output": result.Output,
		"steps":  result.Steps,
		"usage":  result.Usage,
	})
}

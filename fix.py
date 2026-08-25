import sys

with open("internal/adminstore/store.go", "r") as f:
    content = f.read()

content = content.replace("""func (s *Store) RecordUsage(project string, prompt, completion int64, cost float64) {
	if project == "" {
		project = "(admin)"
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	c := s.st.Usage[project]
	c.Requests++
	c.PromptTokens += prompt
	c.CompletionTokens += completion
	c.CostUSD += cost
	s.st.Usage[project] = c

	for i := range s.st.Tokens {""", """func (s *Store) RecordUsage(project, prov, model string, prompt, completion int64, cost float64) {
	if project == "" {
		project = "(admin)"
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	c := s.st.Usage[project]
	c.Requests++
	c.PromptTokens += prompt
	c.CompletionTokens += completion
	c.CostUSD += cost
	s.st.Usage[project] = c

	cm := s.st.UsageByModel[model]
	cm.Requests++
	cm.PromptTokens += prompt
	cm.CompletionTokens += completion
	cm.CostUSD += cost
	s.st.UsageByModel[model] = cm

	cp := s.st.UsageByProv[prov]
	cp.Requests++
	cp.PromptTokens += prompt
	cp.CompletionTokens += completion
	cp.CostUSD += cost
	s.st.UsageByProv[prov] = cp

	for i := range s.st.Tokens {""")

content = content.replace("""func (s *Store) Usage() map[string]Counters {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Counters, len(s.st.Usage))
	for k, v := range s.st.Usage {
		out[k] = v
	}
	return out
}""", """func (s *Store) Usage() (map[string]Counters, map[string]Counters, map[string]Counters) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Counters, len(s.st.Usage))
	for k, v := range s.st.Usage {
		out[k] = v
	}
	outM := make(map[string]Counters, len(s.st.UsageByModel))
	for k, v := range s.st.UsageByModel {
		outM[k] = v
	}
	outP := make(map[string]Counters, len(s.st.UsageByProv))
	for k, v := range s.st.UsageByProv {
		outP[k] = v
	}
	return out, outM, outP
}""")

content = content.replace("""if s.st.Usage == nil {
		s.st.Usage = make(map[string]Counters)
	}""", """if s.st.Usage == nil {
		s.st.Usage = make(map[string]Counters)
	}
	if s.st.UsageByModel == nil {
		s.st.UsageByModel = make(map[string]Counters)
	}
	if s.st.UsageByProv == nil {
		s.st.UsageByProv = make(map[string]Counters)
	}""")

content = content.replace("""if name == "" {
		s.st.Usage = map[string]Counters{}
	}""", """if name == "" {
		s.st.Usage = map[string]Counters{}
		s.st.UsageByModel = map[string]Counters{}
		s.st.UsageByProv = map[string]Counters{}
	}""")

with open("internal/adminstore/store.go", "w") as f:
    f.write(content)


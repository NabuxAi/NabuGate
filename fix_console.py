import sys

with open("internal/server/console.go", "r") as f:
    content = f.read()

content = content.replace(
    """func (s *Server) consoleUsage(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"by_project": s.admin.Usage()})
}""",
    """func (s *Server) consoleUsage(w http.ResponseWriter, _ *http.Request) {
	byProj, byModel, byProv := s.admin.Usage()
	writeJSON(w, http.StatusOK, map[string]any{
		"by_project": byProj,
		"by_model": byModel,
		"by_provider": byProv,
	})
}"""
)

with open("internal/server/console.go", "w") as f:
    f.write(content)

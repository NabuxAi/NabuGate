import sys

with open("internal/server/console.go", "r") as f:
    content = f.read()

content = content.replace(
    '''	writeJSON(w, http.StatusOK, map[string]any{
		"providers":   providers,
		"aliases":     aliases,
		"agents":      s.agents.Names(),
		"config_keys": s.policy.Projects(),
		"usage":       s.admin.Usage(),
	})''',
    '''	byProj, _, _ := s.admin.Usage()
	writeJSON(w, http.StatusOK, map[string]any{
		"providers":   providers,
		"aliases":     aliases,
		"agents":      s.agents.Names(),
		"config_keys": s.policy.Projects(),
		"usage":       byProj,
	})'''
)

with open("internal/server/console.go", "w") as f:
    f.write(content)

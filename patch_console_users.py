import sys

with open("internal/server/console.go", "r") as f:
    content = f.read()

content = content.replace(
    'mux.Handle("GET /api/admins", s.consoleAuth(requireAdmin(s.listAdmins)))',
    'mux.Handle("GET /api/admins", s.consoleAuth(requireAdmin(s.listAdmins)))\n\tmux.Handle("GET /api/users", s.consoleAuth(requireAdmin(s.listUsers)))\n\tmux.Handle("POST /api/users/recharge", s.consoleAuth(requireAdmin(s.adminRechargeUser)))'
)

new_methods = """
func (s *Server) listUsers(w http.ResponseWriter, _ *http.Request) {
	if s.admin == nil {
		writeJSON(w, http.StatusOK, map[string]any{"users": []any{}})
		return
	}
	users := s.admin.ListUsers()
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (s *Server) adminRechargeUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email  string  `json:"email"`
		Amount float64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if s.admin == nil {
		writeError(w, http.StatusInternalServerError, "no admin store")
		return
	}
	err := s.admin.AddPayment(body.Email, body.Amount, "admin-recharge", "manual-"+fmt.Sprint(time.Now().UnixNano()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
"""

content += new_methods

with open("internal/server/console.go", "w") as f:
    f.write(content)

import sys

with open("internal/adminstore/store.go", "r") as f:
    content = f.read()

new_method = """
// ListUsers returns a list of all registered users
func (s *Store) ListUsers() []User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []User
	for _, u := range s.st.Users {
		out = append(out, *u)
	}
	return out
}
"""

content += new_method

with open("internal/adminstore/store.go", "w") as f:
    f.write(content)


import sys

with open("internal/adminstore/store.go", "r") as f:
    content = f.read()
    
content = content.replace('''	if s.st.Usage == nil {
		s.st.Usage = map[string]Counters{}
	}
	if s.st.Sessions == nil {
		s.st.Sessions = map[string]time.Time{}
	}''', '''	if s.st.Usage == nil {
		s.st.Usage = map[string]Counters{}
	}
	if s.st.UsageByModel == nil {
		s.st.UsageByModel = map[string]Counters{}
	}
	if s.st.UsageByProv == nil {
		s.st.UsageByProv = map[string]Counters{}
	}
	if s.st.Sessions == nil {
		s.st.Sessions = map[string]time.Time{}
	}
	if s.st.UserSessions == nil {
		s.st.UserSessions = map[string]SessionInfo{}
	}
	if s.st.Users == nil {
		s.st.Users = map[string]*User{}
	}''')

with open("internal/adminstore/store.go", "w") as f:
    f.write(content)

with open("internal/adminstore/store_test.go", "r") as f:
    content = f.read()

content = content.replace(
    'token, secret, err := s.NewToken("test-project", []string{"gpt-4"}, 0, nil)',
    'token, secret, err := s.NewToken("test-project", []string{"gpt-4"}, 0, nil, "admin@test.com", nil)'
).replace(
    't1, _, _ := s.NewToken("limited", nil, 5, nil)',
    't1, _, _ := s.NewToken("limited", nil, 5, nil, "admin@test.com", nil)'
).replace(
    't2, _, _ := s.NewToken("unlimited", []string{"gpt-4"}, 0, nil)',
    't2, _, _ := s.NewToken("unlimited", []string{"gpt-4"}, 0, nil, "admin@test.com", nil)'
).replace(
    't1, _, _ := s.NewToken("to-delete", []string{"*"}, 0, nil)',
    't1, _, _ := s.NewToken("to-delete", []string{"*"}, 0, nil, "admin@test.com", nil)'
).replace(
    't2, _, _ := s.NewToken("to-keep", []string{"*"}, 0, nil)',
    't2, _, _ := s.NewToken("to-keep", []string{"*"}, 0, nil, "admin@test.com", nil)'
).replace(
    'usage := reopened.Usage()',
    'usage, _, _ := reopened.Usage()'
)

with open("internal/adminstore/store_test.go", "w") as f:
    f.write(content)


import sys

with open("internal/adminstore/store_test.go", "r") as f:
    content = f.read()

content = content.replace(
    '''if s.ValidSession(token) {''',
    '''if _, ok := s.ValidSession(token); ok {'''
)
content = content.replace(
    '''if !s.ValidSession(token) {''',
    '''if _, ok := s.ValidSession(token); !ok {'''
)
content = content.replace(
    '''token, secret, err := s.NewToken("test-project", []string{"gpt-4"}, 0, nil)''',
    '''token, secret, err := s.NewToken("test-project", []string{"gpt-4"}, 0, nil, "admin@test.com", nil)'''
)
content = content.replace(
    '''t1, _, _ := s.NewToken("limited", nil, 5, nil)''',
    '''t1, _, _ := s.NewToken("limited", nil, 5, nil, "admin@test.com", nil)'''
)
content = content.replace(
    '''t2, _, _ := s.NewToken("unlimited", []string{"gpt-4"}, 0, nil)''',
    '''t2, _, _ := s.NewToken("unlimited", []string{"gpt-4"}, 0, nil, "admin@test.com", nil)'''
)
content = content.replace(
    '''t1, _, _ := s.NewToken("to-delete", []string{"*"}, 0, nil)''',
    '''t1, _, _ := s.NewToken("to-delete", []string{"*"}, 0, nil, "admin@test.com", nil)'''
)
content = content.replace(
    '''t2, _, _ := s.NewToken("to-keep", []string{"*"}, 0, nil)''',
    '''t2, _, _ := s.NewToken("to-keep", []string{"*"}, 0, nil, "admin@test.com", nil)'''
)
content = content.replace(
    '''usage := reopened.Usage()''',
    '''usage, _, _ := reopened.Usage()'''
)

with open("internal/adminstore/store_test.go", "w") as f:
    f.write(content)

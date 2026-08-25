import sys
import re

with open("internal/adminstore/store.go", "r") as f:
    content = f.read()

content = content.replace('''	s := &Store{path: path, st: state{
		Usage:    map[string]Counters{},
		Sessions: map[string]time.Time{},
	}}''', '''	s := &Store{path: path, st: state{
		Usage:        map[string]Counters{},
		UsageByModel: map[string]Counters{},
		UsageByProv:  map[string]Counters{},
		Sessions:     map[string]time.Time{},
		UserSessions: map[string]SessionInfo{},
		Users:        map[string]*User{},
	}}''')

with open("internal/adminstore/store.go", "w") as f:
    f.write(content)

with open("internal/adminstore/store_test.go", "r") as f:
    content = f.read()

# Replace all occurrences of s.NewToken(a, b, c, d) that might have been missed
# Wait, I can just use a regex for NewToken(..., nil) to NewToken(..., nil, "admin@test.com", nil)
# but wait, the signature of NewToken is:
# func (s *Store) NewToken(name string, allow []string, rateLimit int, allowedOrigins []string, owner string, providers []string) (string, string, error) {
content = re.sub(r's\.NewToken\("([^"]+)", ([^,]+), ([^,]+), nil\)', r's.NewToken("\1", \2, \3, nil, "admin@test.com", nil)', content)

# There is also one line with `usage := reopened.Usage()` which I tried to replace earlier but might have failed.
content = re.sub(r'(?<!_)usage := reopened\.Usage\(\)', r'usage, _, _ := reopened.Usage()', content)

with open("internal/adminstore/store_test.go", "w") as f:
    f.write(content)

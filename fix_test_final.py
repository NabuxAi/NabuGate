import sys
import re

with open("internal/adminstore/store_test.go", "r") as f:
    content = f.read()

# Replace all forms of `usage := reopened.Usage()`
content = re.sub(r'usage := reopened\.Usage\(\)', 'usage, _, _ := reopened.Usage()', content)
# Just in case it was already replaced as `usage, _, _ := reopened.Usage()`, wait no, the error is exactly `usage := reopened.Usage()` on line 149

with open("internal/adminstore/store_test.go", "w") as f:
    f.write(content)

import sys

with open("internal/adminstore/store_test.go", "r") as f:
    content = f.read()

content = content.replace(
    'got := reopened.Usage()["nabuwrite"]',
    'usage, _, _ := reopened.Usage()\n\tgot := usage["nabuwrite"]'
)

with open("internal/adminstore/store_test.go", "w") as f:
    f.write(content)

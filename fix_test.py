import sys

with open("internal/adminstore/store_test.go", "r") as f:
    content = f.read()

content = content.replace(
    's.RecordUsage("nabuwrite", 100, 40, 0.002)',
    's.RecordUsage("nabuwrite", "openai", "gpt-4", 100, 40, 0.002)'
)
content = content.replace(
    's.RecordUsage("nabuwrite", 50, 10, 0.001)',
    's.RecordUsage("nabuwrite", "openai", "gpt-4", 50, 10, 0.001)'
)

with open("internal/adminstore/store_test.go", "w") as f:
    f.write(content)

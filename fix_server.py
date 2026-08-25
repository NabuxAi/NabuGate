import sys

with open("internal/server/server.go", "r") as f:
    content = f.read()

content = content.replace(
    "s.admin.RecordUsage(project, int64(u.PromptTokens), int64(u.CompletionTokens), cost)",
    "s.admin.RecordUsage(project, prov, model, int64(u.PromptTokens), int64(u.CompletionTokens), cost)"
)

with open("internal/server/server.go", "w") as f:
    f.write(content)

# Pangreksa AI Gateway Client — run examples.
# Requires the Gateway Daemon to be running (.\scripts\run-daemon.ps1).
#
# Proxy → Port mapping:
#   openai   → :8080   (default)
#   claude   → :8081
#   moonshot → :8082
#   ollama   → :8083

$env:DAEMON_TOKEN = "00000000-0000-0000-0000-000000000002"

Write-Host "Pangreksa AI Gateway Client" -ForegroundColor Cyan
Write-Host "Usage examples:" -ForegroundColor Gray
Write-Host "  go run ./cmd/client                                                     # OpenAI  gpt-4o (default)"
Write-Host "  go run ./cmd/client -proxy claude   -model claude-sonnet-4-5           # Anthropic Claude"
Write-Host "  go run ./cmd/client -proxy moonshot -model moonshot-v1-8k              # Moonshot Kimi"
Write-Host "  go run ./cmd/client -proxy ollama   -model llama3                      # Ollama local"
Write-Host "  go run ./cmd/client -proxy claude   -model claude-sonnet-4-5 -stream   # Claude streaming"
Write-Host "  go run ./cmd/client -proxy openai   -message 'Say hello'               # single-shot"
Write-Host ""

# Default interactive session — change -proxy / -model as needed:
go run ./cmd/client -proxy claude -model claude-sonnet-4-5

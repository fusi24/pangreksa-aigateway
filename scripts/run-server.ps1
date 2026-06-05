# Central Server Engine — set env vars and run
$env:DATABASE_URL                  = "postgres://pangreksa:devpassword@192.168.32.161:5432/pangreksa?sslmode=disable"
$env:REDIS_URL                     = "redis://192.168.32.161:6379"
$env:KAFKA_BROKERS                 = "192.168.32.161:9094"
$env:SERVER_PORT                   = "9000"
$env:ADMIN_API_KEY                 = "dev-admin-key-changeme"
$env:DAEMON_API_KEY                = "dev-daemon-key-changeme"
$env:ENCRYPT_KEY                   = "0000000000000000000000000000000000000000000000000000000000000000"
$env:LOG_LEVEL                     = "debug"
$env:OTEL_EXPORTER_OTLP_ENDPOINT   = "192.168.32.161:4317"

Write-Host "Starting Central Server on port $env:SERVER_PORT ..." -ForegroundColor Cyan
go run ./cmd/server

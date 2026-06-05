# Gateway Engine — set env vars and run
$env:GATEWAY_ID                    = "gw-dev-001"
$env:CONFIG_SERVER_URL             = "http://localhost:9000"
$env:DAEMON_API_KEY                = "dev-daemon-key-changeme"
$env:REDIS_URL                     = "redis://192.168.32.161:6379"
$env:KAFKA_BROKERS                 = "192.168.32.161:9094"
$env:POLL_INTERVAL_SEC             = "30"
$env:MAX_LIVENESS_RETRIES          = "0"
$env:ENCRYPT_KEY                   = "0000000000000000000000000000000000000000000000000000000000000000"
$env:LOG_LEVEL                     = "debug"
$env:OTEL_EXPORTER_OTLP_ENDPOINT   = "192.168.32.161:4317"

Write-Host "Starting Gateway Engine (connecting to $env:CONFIG_SERVER_URL) ..." -ForegroundColor Cyan
go run ./cmd/daemon

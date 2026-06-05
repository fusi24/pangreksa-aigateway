# Seed development data into the Pangreksa AI Gateway database.
# Run this ONCE before starting the Central Server and Gateway Daemon.
# It is safe to run multiple times (all inserts are idempotent).

$env:DATABASE_URL = "postgres://pangreksa:devpassword@192.168.32.161:5432/pangreksa?sslmode=disable"

Write-Host "Seeding development data..." -ForegroundColor Cyan
go run ./cmd/seed

// Command update-config encrypts real LLM provider API keys and persists them into
// the Pangreksa AI Gateway database. It updates the gateway_configs table so the
// Gateway Daemon will pick up the new provider credentials on its next config poll.
//
// It also upserts one row per provider into the provider_accounts table, giving the
// Central Server a normalised view of available credentials.
//
// Environment variables required:
//
//	DATABASE_URL        — PostgreSQL DSN (e.g. postgres://user:pass@host/db?sslmode=disable)
//	ENCRYPT_KEY         — 64-hex-char AES-256 key used to encrypt API keys at rest
//	OPENAI_API_KEY      — OpenAI API key (plaintext)
//	ANTHROPIC_API_KEY   — Anthropic API key (plaintext)
//	KIMI_API_KEY        — Moonshot (Kimi) API key (plaintext)
//
// The command exits with code 1 and prints a descriptive message if any env var is
// missing, the encryption fails, or the database write fails.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/pangreksa/ai-gateway-engine/pkg/crypto"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// devOrgID is the fixed UUID of the Pangreksa Dev organisation inserted by seed.
const devOrgID = "00000000-0000-0000-0000-000000000001"

// providerEntry pairs a human-readable provider name with the plaintext API key
// read from the environment and the ProxyConfig metadata used to build
// the GatewayConfig.
type providerEntry struct {
	// envKey is the name of the environment variable holding the plaintext API key.
	envKey string
	// provider is the provider name used in gateway_configs and provider_accounts.
	provider string
	// port is the TCP port the daemon proxy listener will bind to for this provider.
	port int
	// baseURL is the upstream LLM provider API base URL.
	baseURL string
	// apiKey holds the plaintext value read from the environment at runtime.
	apiKey string
}

func main() {
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║   PANGREKSA AI Gateway — Update Provider Config  ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "update-config: fatal: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Provider config updated successfully.")
}

// run is the top-level orchestrator. It reads env vars, encrypts API keys,
// builds the GatewayConfig, and writes both gateway_configs and provider_accounts
// to the database. Errors are returned (never os.Exit from here) so tests can
// inspect them.
func run() error {
	// ── 1. Read and validate required env vars ────────────────────────────────
	fmt.Println("Step 1: Reading environment variables...")

	dbURL, err := requireEnv("DATABASE_URL")
	if err != nil {
		return err
	}
	encKeyHex, err := requireEnv("ENCRYPT_KEY")
	if err != nil {
		return err
	}

	providers := []providerEntry{
		{
			envKey:   "OPENAI_API_KEY",
			provider: "openai",
			port:     8080,
			baseURL:  "https://api.openai.com",
		},
		{
			envKey:   "ANTHROPIC_API_KEY",
			provider: "claude",
			port:     8081,
			baseURL:  "https://api.anthropic.com",
		},
		{
			envKey:   "KIMI_API_KEY",
			provider: "moonshot",
			port:     8082,
			baseURL:  "https://api.moonshot.cn",
		},
	}

	for i := range providers {
		val, envErr := requireEnv(providers[i].envKey)
		if envErr != nil {
			return envErr
		}
		providers[i].apiKey = val
	}

	fmt.Println("  OK: all environment variables present")

	// ── 2. Parse the encryption key ───────────────────────────────────────────
	fmt.Println()
	fmt.Println("Step 2: Parsing ENCRYPT_KEY...")

	cryptoKey, err := crypto.KeyFromHex(encKeyHex)
	if err != nil {
		return fmt.Errorf("parse ENCRYPT_KEY: %w", err)
	}

	fmt.Println("  OK: 32-byte AES-256 key loaded")

	// ── 3. Encrypt each API key ───────────────────────────────────────────────
	fmt.Println()
	fmt.Println("Step 3: Encrypting API keys...")

	for i := range providers {
		enc, encErr := encryptAPIKey(cryptoKey, providers[i].apiKey)
		if encErr != nil {
			return fmt.Errorf("encrypt %s key: %w", providers[i].provider, encErr)
		}
		providers[i].apiKey = enc // replace plaintext with encrypted+base64 value
		fmt.Printf("  ✓ encrypted %s key\n", providers[i].provider)
	}

	// ── 4. Connect to the database ────────────────────────────────────────────
	fmt.Println()
	fmt.Println("Step 4: Connecting to database...")

	ctx := context.Background()

	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer conn.Close(ctx) //nolint:errcheck

	if err = conn.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	fmt.Println("  OK: connected")

	// ── 5. Ensure gateway_configs table ──────────────────────────────────────
	fmt.Println()
	fmt.Println("Step 5: Ensuring gateway_configs table exists...")

	if err = ensureGatewayConfigsTable(ctx, conn); err != nil {
		return fmt.Errorf("ensure gateway_configs table: %w", err)
	}

	fmt.Println("  OK: gateway_configs table ready")

	// ── 6. Build GatewayConfig with encrypted keys ────────────────────────────
	fmt.Println()
	fmt.Println("Step 6: Building GatewayConfig...")

	proxies := make([]model.ProxyConfig, 0, len(providers))
	for _, p := range providers {
		proxies = append(proxies, model.ProxyConfig{
			Provider:  p.provider,
			Port:      p.port,
			BaseURL:   p.baseURL,
			APIKeyEnc: p.apiKey, // already encrypted+base64 at this point
		})
	}

	cfg := model.GatewayConfig{
		Version:   "", // zeroed for hash computation; set after
		GatewayID: "",
		Proxies:   proxies,
		Registry: model.RegistryConfig{
			Prompts: []model.PromptConfig{},
			Skills:  []model.SkillConfig{},
			MCPs:    []model.MCPConfig{},
		},
		Policies: model.PolicyConfig{
			Guardrails:  []model.GuardrailConfig{},
			BudgetRules: []model.BudgetRule{},
			RateRules:   []model.RateLimitRule{},
		},
	}

	// ── 7. Compute SHA-256 version ────────────────────────────────────────────
	version, err := computeConfigVersion(cfg)
	if err != nil {
		return fmt.Errorf("compute config version: %w", err)
	}

	// ── 8. Embed version and marshal ──────────────────────────────────────────
	cfg.Version = version

	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal gateway config: %w", err)
	}

	fmt.Printf("  OK: version=%s\n", version)

	// ── 9. INSERT into gateway_configs ────────────────────────────────────────
	fmt.Println()
	fmt.Println("Step 7: Inserting GatewayConfig into gateway_configs...")

	if err = insertGatewayConfig(ctx, conn, version, cfgBytes); err != nil {
		return fmt.Errorf("insert gateway config: %w", err)
	}

	fmt.Printf("  OK: gateway_config version=%s inserted\n", version)

	// ── 10. Upsert provider_accounts ──────────────────────────────────────────
	fmt.Println()
	fmt.Println("Step 8: Upserting provider_accounts...")

	for _, p := range providers {
		if err = upsertProviderAccount(ctx, conn, p); err != nil {
			return fmt.Errorf("upsert provider_account for %s: %w", p.provider, err)
		}
		fmt.Printf("  OK: provider_account upserted for %s\n", p.provider)
	}

	return nil
}

// requireEnv returns the value of the named environment variable, or an error
// describing which variable is missing. This ensures every missing var produces
// a clear, actionable error message.
func requireEnv(name string) (string, error) {
	val := os.Getenv(name)
	if val == "" {
		return "", fmt.Errorf("required environment variable %s is not set or empty", name)
	}
	return val, nil
}

// encryptAPIKey encrypts plaintext using AES-256-GCM via pkg/crypto and returns
// the result as a standard base64-encoded string.
//
// The dispatcher (internal/llm/dispatcher.go) decrypts stored keys as:
//
//	cipherBytes, _ := base64.StdEncoding.DecodeString(encryptedKey)
//	plainBytes,  _ := crypto.Decrypt(d.cryptoKey, cipherBytes)
//
// So the stored format must be: base64.StdEncoding.EncodeToString(crypto.Encrypt(key, plaintext))
func encryptAPIKey(cryptoKey []byte, plaintext string) (string, error) {
	cipherBytes, err := crypto.Encrypt(cryptoKey, []byte(plaintext))
	if err != nil {
		return "", fmt.Errorf("encryptAPIKey: %w", err)
	}
	return base64.StdEncoding.EncodeToString(cipherBytes), nil
}

// ensureGatewayConfigsTable creates the gateway_configs table if it does not
// already exist. This replicates the DDL from ConfigStoreRepository.EnsureTable
// so update-config is self-contained and does not depend on the server package.
//
// ctx controls deadline and cancellation for the DDL statement.
func ensureGatewayConfigsTable(ctx context.Context, conn *pgx.Conn) error {
	const ddl = `
		CREATE TABLE IF NOT EXISTS gateway_configs (
			id         BIGSERIAL    PRIMARY KEY,
			version    VARCHAR(64)  NOT NULL,
			config     JSONB        NOT NULL,
			created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
		)
	`
	if _, err := conn.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("ensureGatewayConfigsTable: exec DDL: %w", err)
	}
	return nil
}

// computeConfigVersion returns the lowercase hex SHA-256 digest of the
// JSON-serialised GatewayConfig. The Version field is zeroed before hashing
// so it does not contribute to the digest (mirrors config_store.go behaviour).
//
// cfg is passed by value so the caller's struct is not mutated.
func computeConfigVersion(cfg model.GatewayConfig) (string, error) {
	cfg.Version = "" // exclude version from its own hash

	b, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("computeConfigVersion: marshal: %w", err)
	}

	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h), nil
}

// insertGatewayConfig inserts a new row into gateway_configs.
// It does not deduplicate by version — each run inserts a fresh timestamped row,
// and the daemon always reads the row with the highest id (most recent).
//
// ctx controls deadline and cancellation.
// version is the precomputed SHA-256 hex digest string.
// cfgBytes is the JSON-marshalled GatewayConfig (with version embedded).
func insertGatewayConfig(ctx context.Context, conn *pgx.Conn, version string, cfgBytes []byte) error {
	const q = `INSERT INTO gateway_configs (version, config) VALUES ($1, $2)`

	if _, err := conn.Exec(ctx, q, version, cfgBytes); err != nil {
		return fmt.Errorf("insertGatewayConfig: exec: %w", err)
	}
	return nil
}

// upsertProviderAccount inserts or updates a row in provider_accounts for the
// given provider. The upsert key is (org_id, provider_name).
//
// The provider_accounts table schema (from migrations/000001_init_schema.up.sql)
// does not include an updated_at column, so only api_key_enc and base_url are
// updated on conflict.
//
// ctx controls deadline and cancellation.
// p holds the provider name, encrypted API key, and base URL to persist.
func upsertProviderAccount(ctx context.Context, conn *pgx.Conn, p providerEntry) error {
	const q = `
		INSERT INTO provider_accounts (org_id, provider_name, api_key_enc, base_url)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (org_id, provider_name)
		DO UPDATE SET
			api_key_enc = EXCLUDED.api_key_enc,
			base_url    = EXCLUDED.base_url
	`

	if _, err := conn.Exec(ctx, q, devOrgID, p.provider, p.apiKey, p.baseURL); err != nil {
		return fmt.Errorf("upsertProviderAccount: exec for provider=%s: %w", p.provider, err)
	}
	return nil
}

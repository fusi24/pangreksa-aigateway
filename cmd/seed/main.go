// Command seed inserts development seed data into the Pangreksa AI Gateway database.
// It applies all pending migrations first, then inserts an organization, a user,
// user entitlements, and an initial GatewayConfig so the Gateway Daemon can start.
//
// Usage:
//
//	DATABASE_URL=postgres://... go run ./cmd/seed
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/pangreksa/ai-gateway-engine/internal/server/auth"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// migrationsDir is the path to SQL migration files relative to the working directory.
// When running via "go run ./cmd/seed" from the repo root this resolves correctly.
const migrationsDir = "migrations"

// devPassword is the console login password for the seeded dev user.
// Pair it with email dev@pangreksa.ai to log into the console.
const devPassword = "devpassword"

func main() {
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║   PANGREKSA AI Gateway — Dev Seed                ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "seed: fatal: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Seed completed successfully.")
}

func run() error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL environment variable is not set")
	}

	// ── 1. Run pending migrations ─────────────────────────────────────────────
	fmt.Println()
	fmt.Println("Step 1: Applying pending migrations...")
	if err := runMigrations(dbURL); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	fmt.Println("  OK: migrations up to date")

	// ── 2. Connect to the database ────────────────────────────────────────────
	fmt.Println()
	fmt.Println("Step 2: Connecting to database...")
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(ctx)

	if err = conn.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	fmt.Println("  OK: connected")

	// ── 3. Ensure gateway_configs table ──────────────────────────────────────
	fmt.Println()
	fmt.Println("Step 3: Ensuring gateway_configs table exists...")
	if err = ensureGatewayConfigsTable(ctx, conn); err != nil {
		return fmt.Errorf("ensure gateway_configs table: %w", err)
	}
	fmt.Println("  OK: gateway_configs table ready")

	// ── 4. Insert organization ────────────────────────────────────────────────
	fmt.Println()
	fmt.Println("Step 4: Inserting organization...")
	if err = insertOrganization(ctx, conn); err != nil {
		return fmt.Errorf("insert organization: %w", err)
	}
	fmt.Println("  OK: organization 'Pangreksa Dev' (00000000-0000-0000-0000-000000000001)")

	// ── 5. Insert user ────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Println("Step 5: Inserting user...")
	if err = insertUser(ctx, conn); err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	fmt.Println("  OK: user 'dev@pangreksa.ai' (password: 'devpassword')")

	// ── 6. Insert user entitlement ────────────────────────────────────────────
	fmt.Println()
	fmt.Println("Step 6: Inserting user entitlement...")
	if err = insertUserEntitlement(ctx, conn); err != nil {
		return fmt.Errorf("insert user entitlement: %w", err)
	}
	fmt.Println("  OK: entitlement (open access) (00000000-0000-0000-0000-000000000003)")

	// ── 7. Insert API key ─────────────────────────────────────────────────────
	fmt.Println()
	fmt.Println("Step 7: Inserting dev API key...")
	if err = insertAPIKey(ctx, conn); err != nil {
		return fmt.Errorf("insert api key: %w", err)
	}
	fmt.Println("  OK: api_key 'Dev PAT' (00000000-0000-0000-0000-000000000004)")

	// ── 8. Insert gateway config ──────────────────────────────────────────────
	fmt.Println()
	fmt.Println("Step 8: Inserting initial GatewayConfig...")
	version, err := insertGatewayConfig(ctx, conn)
	if err != nil {
		return fmt.Errorf("insert gateway config: %w", err)
	}
	fmt.Printf("  OK: gateway_config version=%s\n", version)

	return nil
}

// runMigrations applies all pending up-migrations using golang-migrate.
func runMigrations(databaseURL string) error {
	sourceURL := fmt.Sprintf("file://%s", migrationsDir)

	m, err := migrate.New(sourceURL, databaseURL)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}
	defer m.Close()

	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}

// ensureGatewayConfigsTable creates the gateway_configs table if it does not exist.
// This replicates the DDL from ConfigStoreRepository.EnsureTable so the seed is
// self-contained and does not need to import the repository package.
func ensureGatewayConfigsTable(ctx context.Context, conn *pgx.Conn) error {
	const ddl = `
		CREATE TABLE IF NOT EXISTS gateway_configs (
			id         BIGSERIAL    PRIMARY KEY,
			version    VARCHAR(64)  NOT NULL,
			config     JSONB        NOT NULL,
			created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
		)
	`
	_, err := conn.Exec(ctx, ddl)
	return err
}

// insertOrganization inserts the dev organization (idempotent via ON CONFLICT).
func insertOrganization(ctx context.Context, conn *pgx.Conn) error {
	const q = `
		INSERT INTO organizations (id, name, slug)
		VALUES ('00000000-0000-0000-0000-000000000001', 'Pangreksa Dev', 'pangreksa-dev')
		ON CONFLICT (slug) DO NOTHING
	`
	_, err := conn.Exec(ctx, q)
	return err
}

// insertUser inserts (or updates) the dev user with a bcrypt-hashed password so
// the account can log into the console via POST /auth/login.
//
// Re-running the seed refreshes the password hash and re-activates the account
// (ON CONFLICT DO UPDATE), keeping dev credentials predictable across runs.
func insertUser(ctx context.Context, conn *pgx.Conn) error {
	hash, err := auth.HashPassword(devPassword)
	if err != nil {
		return fmt.Errorf("hash dev password: %w", err)
	}

	const q = `
		INSERT INTO users (id, org_id, email, password_hash, provider, status)
		VALUES (
			'00000000-0000-0000-0000-000000000002',
			'00000000-0000-0000-0000-000000000001',
			'dev@pangreksa.ai',
			$1,
			'local',
			'active'
		)
		ON CONFLICT (org_id, email) DO UPDATE
			SET password_hash = EXCLUDED.password_hash,
			    status        = 'active'
	`
	_, err = conn.Exec(ctx, q, hash)
	return err
}

// insertUserEntitlement inserts open-access entitlements for the dev user.
// All console RBAC permissions are granted so the frontend module gates pass.
// Uses ON CONFLICT DO UPDATE so re-running the seed refreshes the permissions.
func insertUserEntitlement(ctx context.Context, conn *pgx.Conn) error {
	const allConsolePerms = `[
		"console.observability.read",
		"console.monitor.read",
		"console.telemetry.read",
		"gateway.prompt_registry.read",
		"gateway.prompt_registry.write",
		"console.reports.read",
		"console.reports.generate",
		"console.admin.read",
		"console.admin.write"
	]`

	const q = `
		INSERT INTO user_entitlements (
			id, user_id, org_id,
			allowed_prompts, allowed_skills, allowed_mcps, permissions,
			budget_limit_usd, rate_limit_rpm, rate_limit_tpm, data_scope
		) VALUES (
			'00000000-0000-0000-0000-000000000003',
			'00000000-0000-0000-0000-000000000002',
			'00000000-0000-0000-0000-000000000001',
			'["*"]', '["*"]', '["*"]', $1,
			9999.00, 1000, 10000000, 'all_data'
		) ON CONFLICT (user_id) DO UPDATE
			SET permissions = EXCLUDED.permissions
	`
	_, err := conn.Exec(ctx, q, allConsolePerms)
	return err
}

// insertAPIKey inserts the dev PAT (idempotent via ON CONFLICT on token_hash).
func insertAPIKey(ctx context.Context, conn *pgx.Conn) error {
	const q = `
		INSERT INTO api_keys (id, user_id, org_id, token_hash, name)
		VALUES (
			'00000000-0000-0000-0000-000000000004',
			'00000000-0000-0000-0000-000000000002',
			'00000000-0000-0000-0000-000000000001',
			'dev-test-pat-token-hash',
			'Dev PAT'
		) ON CONFLICT (token_hash) DO NOTHING
	`
	_, err := conn.Exec(ctx, q)
	return err
}

// insertGatewayConfig builds the initial GatewayConfig, computes its SHA-256 version
// (using the same algorithm as ConfigStoreRepository.Upsert), and inserts it into
// gateway_configs. If a row with the same version already exists, it is skipped.
//
// Returns the version hash string on success.
func insertGatewayConfig(ctx context.Context, conn *pgx.Conn) (string, error) {
	// Build the initial config with empty version (version is excluded from the hash).
	cfg := model.GatewayConfig{
		Version:   "",
		GatewayID: "",
		Proxies: []model.ProxyConfig{
			{
				Provider:  "openai",
				Port:      8080,
				BaseURL:   "https://api.openai.com",
				APIKeyEnc: "",
			},
			{
				Provider:  "claude",
				Port:      8081,
				BaseURL:   "https://api.anthropic.com",
				APIKeyEnc: "",
			},
		},
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

	// Compute version: SHA-256 of JSON with Version field zeroed.
	// This mirrors computeConfigVersion in internal/server/repository/config_store.go.
	version, err := computeConfigVersion(cfg)
	if err != nil {
		return "", fmt.Errorf("compute version: %w", err)
	}

	// Embed computed version into the config before persisting (self-describing doc).
	cfg.Version = version

	b, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}

	// Insert idempotently: skip if the exact same version already exists.
	// Cast parameters explicitly to avoid pgx type-inference ambiguity in the
	// SELECT … WHERE NOT EXISTS pattern.
	const q = `
		INSERT INTO gateway_configs (version, config)
		SELECT $1::varchar, $2::jsonb
		WHERE NOT EXISTS (
			SELECT 1 FROM gateway_configs WHERE version = $1::varchar
		)
	`
	_, err = conn.Exec(ctx, q, version, b)
	if err != nil {
		return "", fmt.Errorf("exec insert: %w", err)
	}

	return version, nil
}

// computeConfigVersion returns the lowercase hex SHA-256 digest of the
// JSON-serialised GatewayConfig. The Version field is zeroed before hashing
// so it does not contribute to the digest (mirrors config_store.go behaviour).
func computeConfigVersion(cfg model.GatewayConfig) (string, error) {
	cfg.Version = ""

	b, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h), nil
}

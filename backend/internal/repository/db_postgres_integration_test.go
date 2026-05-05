package repository

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/timmy/emomo/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestInitDBPostgresAutoMigrateLeavesRLSDisabledOnCoreTables(t *testing.T) {
	dsn := os.Getenv("EMOMO_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("set EMOMO_POSTGRES_TEST_DSN to run Postgres AutoMigrate integration test")
	}

	schema := "emomo_automigrate_verify_" + randomHex(t, 4)
	adminDB, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  postgresDSNWithSSLMode(dsn),
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	defer closeGormDB(t, adminDB)

	if err := adminDB.Exec(fmt.Sprintf(`CREATE SCHEMA "%s"`, schema)).Error; err != nil {
		t.Fatalf("failed to create temp schema: %v", err)
	}
	defer func() {
		if err := adminDB.Exec(fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schema)).Error; err != nil {
			t.Fatalf("failed to drop temp schema: %v", err)
		}
	}()

	schemaDSN, err := postgresDSNWithSearchPath(dsn, schema)
	if err != nil {
		t.Fatalf("failed to add search_path to dsn: %v", err)
	}
	db, err := InitDB(&config.DatabaseConfig{
		Driver:          "postgres",
		URL:             schemaDSN,
		AutoMigrate:     true,
		MaxIdleConns:    1,
		MaxOpenConns:    1,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	closeGormDB(t, db)

	for _, table := range []string{"memes", "meme_annotations", "meme_vectors"} {
		var exists bool
		if err := adminDB.Raw(`
			SELECT EXISTS (
				SELECT 1
				FROM information_schema.tables
				WHERE table_schema = ?
				  AND table_name = ?
			)
		`, schema, table).Scan(&exists).Error; err != nil {
			t.Fatalf("failed to inspect table %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("expected table %s in schema %s", table, schema)
		}
	}

	// Pre-enable RLS on the freshly-created tables before re-running InitDB to
	// confirm disableCoreTableRLS actively turns it off on existing schemas
	// (not just leaves a fresh-install no-op untouched).
	for _, table := range []string{"memes", "meme_annotations", "meme_vectors"} {
		if err := adminDB.Exec(fmt.Sprintf(`ALTER TABLE "%s"."%s" ENABLE ROW LEVEL SECURITY`, schema, table)).Error; err != nil {
			t.Fatalf("failed to pre-enable RLS on %s: %v", table, err)
		}
	}

	rerunDB, err := InitDB(&config.DatabaseConfig{
		Driver:          "postgres",
		URL:             schemaDSN,
		AutoMigrate:     true,
		MaxIdleConns:    1,
		MaxOpenConns:    1,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("InitDB() rerun error = %v", err)
	}
	closeGormDB(t, rerunDB)

	for _, table := range []string{"memes", "meme_annotations", "meme_vectors"} {
		var enabled bool
		if err := adminDB.Raw(`
			SELECT c.relrowsecurity
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = ?
			  AND c.relname = ?
		`, schema, table).Scan(&enabled).Error; err != nil {
			t.Fatalf("failed to inspect RLS for %s: %v", table, err)
		}
		if enabled {
			t.Fatalf("expected RLS disabled for %s after InitDB, but it was still enabled", table)
		}
	}
}

func postgresDSNWithSearchPath(rawDSN, schema string) (string, error) {
	if strings.Contains(rawDSN, "://") {
		u, err := url.Parse(rawDSN)
		if err != nil {
			return "", err
		}
		q := u.Query()
		q.Set("search_path", schema)
		if q.Get("sslmode") == "" {
			q.Set("sslmode", "require")
		}
		u.RawQuery = q.Encode()
		return u.String(), nil
	}

	dsn := postgresDSNWithSSLMode(rawDSN)
	if !strings.Contains(dsn, "search_path=") {
		dsn += " search_path=" + schema
	}
	return dsn, nil
}

func postgresDSNWithSSLMode(rawDSN string) string {
	if strings.Contains(rawDSN, "://") {
		u, err := url.Parse(rawDSN)
		if err != nil {
			return rawDSN
		}
		q := u.Query()
		if q.Get("sslmode") == "" {
			q.Set("sslmode", "require")
		}
		u.RawQuery = q.Encode()
		return u.String()
	}
	if strings.Contains(rawDSN, "sslmode=") {
		return rawDSN
	}
	return rawDSN + " sslmode=require"
}

func randomHex(t *testing.T, size int) string {
	t.Helper()

	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("failed to read random bytes: %v", err)
	}
	return hex.EncodeToString(b)
}

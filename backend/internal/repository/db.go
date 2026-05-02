package repository

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/timmy/emomo/internal/config"
	"github.com/timmy/emomo/internal/domain"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// InitDB initializes the database connection based on configuration and runs migrations.
// Parameters:
//   - cfg: database configuration including driver and connection settings.
//
// Returns:
//   - *gorm.DB: initialized database handle.
//   - error: non-nil if connection or migration fails.
func InitDB(cfg *config.DatabaseConfig) (*gorm.DB, error) {
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	}

	var db *gorm.DB
	var err error

	log.Printf("[DB] Initializing database with driver: %q", cfg.Driver)

	switch cfg.Driver {
	case "postgres":
		log.Printf("[DB] Using PostgreSQL driver")
		db, err = initPostgres(cfg, gormConfig)
	case "sqlite":
		log.Printf("[DB] Using SQLite driver")
		db, err = initSQLite(cfg, gormConfig)
	default:
		log.Printf("[DB] Unknown driver %q, defaulting to SQLite", cfg.Driver)
		db, err = initSQLite(cfg, gormConfig)
	}

	if err != nil {
		return nil, err
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB instance: %w", err)
	}

	// Set standard connection pool settings
	// These are critical for production stability regardless of the underlying driver
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if cfg.AutoMigrate {
		log.Printf("[DB] AutoMigrate enabled")
		if err := prepareLegacyMemesForAutoMigrate(db); err != nil {
			return nil, fmt.Errorf("failed to prepare legacy memes: %w", err)
		}
		preparedMemeVectors, err := prepareLegacyMemeVectorsForAutoMigrate(db)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare legacy meme vectors: %w", err)
		}
		models := []interface{}{
			&domain.Meme{},
			&domain.MemeAnnotation{},
		}
		if !preparedMemeVectors {
			models = append(models, &domain.MemeVector{})
		}
		if err := db.AutoMigrate(models...); err != nil {
			return nil, fmt.Errorf("failed to migrate database: %w", err)
		}
		if err := migrateMemes(db); err != nil {
			return nil, fmt.Errorf("failed to migrate memes: %w", err)
		}
		if err := migrateMemeAnnotations(db); err != nil {
			return nil, fmt.Errorf("failed to migrate meme annotations: %w", err)
		}
		if err := migrateMemeVectorIndexes(db); err != nil {
			return nil, fmt.Errorf("failed to migrate meme vector indexes: %w", err)
		}
		if err := dropLegacyArtifacts(db); err != nil {
			return nil, fmt.Errorf("failed to drop legacy artifacts: %w", err)
		}
		if err := disableCoreTableRLS(db); err != nil {
			return nil, fmt.Errorf("failed to disable core table RLS: %w", err)
		}
	} else {
		log.Printf("[DB] AutoMigrate disabled")
	}

	return db, nil
}

func prepareLegacyMemesForAutoMigrate(db *gorm.DB) error {
	if !db.Migrator().HasTable("memes") {
		return nil
	}
	// SQLite cannot do piecewise ALTER TABLE DROP COLUMN reliably (GORM's
	// driver rebuilds the table for each DropColumn / AlterColumn and that
	// path interacts badly with our pre-2026-05-01 layout where many legacy
	// columns coexist with the new ones). On SQLite we instead do a single
	// schema rebuild here, mirroring the cleanup that
	// prepareLegacyMemeVectorsForAutoMigrate performs for `meme_vectors`.
	if db.Dialector.Name() == "sqlite" {
		rebuilt, err := rebuildSQLiteLegacyMemes(db)
		if err != nil {
			return err
		}
		if rebuilt {
			return nil
		}
	}

	// Postgres path: add the new columns alongside the legacy ones; the
	// legacy columns are dropped later in dropLegacyArtifacts.
	if !db.Migrator().HasColumn("memes", "content_hash") {
		if err := db.Exec(`ALTER TABLE memes ADD COLUMN content_hash TEXT`).Error; err != nil {
			return err
		}
	}
	if !db.Migrator().HasColumn("memes", "image_info") {
		if err := db.Exec(`ALTER TABLE memes ADD COLUMN image_info TEXT`).Error; err != nil {
			return err
		}
	}
	return migrateMemes(db)
}

// rebuildSQLiteLegacyMemes rewrites the `memes` table so it matches the
// current domain.Meme schema and drops every legacy column in one shot.
// It returns true when a rebuild actually happened so the caller can skip
// the per-column ALTER TABLE path.
func rebuildSQLiteLegacyMemes(db *gorm.DB) (bool, error) {
	needsRebuild := false
	for _, col := range legacyMemesColumns {
		if db.Migrator().HasColumn("memes", col) {
			needsRebuild = true
			break
		}
	}
	if !needsRebuild {
		return false, nil
	}

	storageKeyExpr := "''"
	if db.Migrator().HasColumn("memes", "storage_key") {
		storageKeyExpr = "COALESCE(storage_key, '')"
	}

	contentHashExpr := "''"
	switch {
	case db.Migrator().HasColumn("memes", "content_hash") &&
		db.Migrator().HasColumn("memes", "md5_hash"):
		contentHashExpr = "COALESCE(NULLIF(content_hash, ''), md5_hash, '')"
	case db.Migrator().HasColumn("memes", "content_hash"):
		contentHashExpr = "COALESCE(content_hash, '')"
	case db.Migrator().HasColumn("memes", "md5_hash"):
		contentHashExpr = "COALESCE(md5_hash, '')"
	}

	imageInfoExpr := "'{}'"
	switch {
	case db.Migrator().HasColumn("memes", "image_info") &&
		db.Migrator().HasColumn("memes", "width") &&
		db.Migrator().HasColumn("memes", "height") &&
		db.Migrator().HasColumn("memes", "format"):
		imageInfoExpr = `
			CASE
				WHEN image_info IS NOT NULL AND image_info != '' AND image_info != '{}'
				THEN image_info
				ELSE
					'{"width":' || COALESCE(CAST(width AS TEXT), '0') ||
					',"height":' || COALESCE(CAST(height AS TEXT), '0') ||
					',"format":' ||
					CASE lower(COALESCE(format, ''))
						WHEN 'jpg' THEN '1'
						WHEN 'jpeg' THEN '1'
						WHEN 'png' THEN '2'
						WHEN 'webp' THEN '3'
						ELSE '0'
					END ||
					'}'
			END`
	case db.Migrator().HasColumn("memes", "image_info"):
		imageInfoExpr = "COALESCE(NULLIF(image_info, ''), '{}')"
	case db.Migrator().HasColumn("memes", "width") &&
		db.Migrator().HasColumn("memes", "height") &&
		db.Migrator().HasColumn("memes", "format"):
		imageInfoExpr = `
			'{"width":' || COALESCE(CAST(width AS TEXT), '0') ||
			',"height":' || COALESCE(CAST(height AS TEXT), '0') ||
			',"format":' ||
			CASE lower(COALESCE(format, ''))
				WHEN 'jpg' THEN '1'
				WHEN 'jpeg' THEN '1'
				WHEN 'png' THEN '2'
				WHEN 'webp' THEN '3'
				ELSE '0'
			END ||
			'}'`
	}

	tagsExpr := "'[]'"
	if db.Migrator().HasColumn("memes", "tags") {
		tagsExpr = "COALESCE(tags, '[]')"
	}
	categoryExpr := "''"
	if db.Migrator().HasColumn("memes", "category") {
		categoryExpr = "COALESCE(category, '')"
	}
	createdAtExpr := "CURRENT_TIMESTAMP"
	if db.Migrator().HasColumn("memes", "created_at") {
		createdAtExpr = "COALESCE(created_at, CURRENT_TIMESTAMP)"
	}
	updatedAtExpr := "CURRENT_TIMESTAMP"
	if db.Migrator().HasColumn("memes", "updated_at") {
		updatedAtExpr = "COALESCE(updated_at, CURRENT_TIMESTAMP)"
	}

	if err := db.Exec(`DROP TABLE IF EXISTS memes__clean`).Error; err != nil {
		return false, err
	}
	if err := db.Exec(`
		CREATE TABLE memes__clean (
			id TEXT PRIMARY KEY,
			storage_key TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			image_info TEXT NOT NULL DEFAULT '{}',
			tags TEXT,
			category TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error; err != nil {
		return false, err
	}
	if err := db.Exec(fmt.Sprintf(`
		INSERT INTO memes__clean (
			id,
			storage_key,
			content_hash,
			image_info,
			tags,
			category,
			created_at,
			updated_at
		)
		SELECT
			id,
			%s,
			%s,
			%s,
			%s,
			%s,
			%s,
			%s
		FROM memes
	`, storageKeyExpr, contentHashExpr, imageInfoExpr, tagsExpr, categoryExpr, createdAtExpr, updatedAtExpr)).Error; err != nil {
		return false, err
	}
	if err := db.Exec(`DROP TABLE memes`).Error; err != nil {
		return false, err
	}
	if err := db.Exec(`ALTER TABLE memes__clean RENAME TO memes`).Error; err != nil {
		return false, err
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_memes_content_hash ON memes(content_hash)`).Error; err != nil {
		return false, err
	}
	return true, nil
}

func prepareLegacyMemeVectorsForAutoMigrate(db *gorm.DB) (bool, error) {
	if !db.Migrator().HasTable("meme_vectors") {
		return false, nil
	}
	if db.Dialector.Name() == "postgres" {
		return false, preparePostgresLegacyMemeVectorsForAutoMigrate(db)
	}
	if db.Dialector.Name() != "sqlite" {
		return false, nil
	}
	if !needsSQLiteMemeVectorRebuild(db) {
		return false, nil
	}

	memeIDExpr := "''"
	if db.Migrator().HasColumn("meme_vectors", "meme_id") {
		memeIDExpr = "COALESCE(meme_id, '')"
	} else if db.Migrator().HasColumn("meme_vectors", "md5_hash") &&
		db.Migrator().HasTable("memes") &&
		db.Migrator().HasColumn("memes", "md5_hash") {
		memeIDExpr = "COALESCE((SELECT m.id FROM memes m WHERE m.md5_hash = meme_vectors.md5_hash LIMIT 1), '')"
	}

	vectorTypeExpr := "1"
	if db.Migrator().HasColumn("meme_vectors", "vector_type") {
		vectorTypeExpr = `
			CASE
				WHEN CAST(vector_type AS TEXT) = 'caption' THEN 2
				WHEN CAST(vector_type AS TEXT) = '2' THEN 2
				ELSE 1
			END`
	}

	inputHashExpr := "''"
	if db.Migrator().HasColumn("meme_vectors", "input_hash") {
		inputHashExpr = "COALESCE(input_hash, '')"
	} else if db.Migrator().HasColumn("meme_vectors", "md5_hash") {
		inputHashExpr = "COALESCE(md5_hash, '')"
	}

	annotationIDExpr := "''"
	hasAnnotationID := db.Migrator().HasColumn("meme_vectors", "annotation_id")
	hasDescriptionID := db.Migrator().HasColumn("meme_vectors", "description_id")
	if hasAnnotationID && hasDescriptionID {
		annotationIDExpr = "COALESCE(NULLIF(annotation_id, ''), description_id, '')"
	} else if hasAnnotationID {
		annotationIDExpr = "COALESCE(annotation_id, '')"
	} else if hasDescriptionID {
		annotationIDExpr = "COALESCE(description_id, '')"
	}

	createdAtExpr := "CURRENT_TIMESTAMP"
	if db.Migrator().HasColumn("meme_vectors", "created_at") {
		createdAtExpr = "COALESCE(created_at, CURRENT_TIMESTAMP)"
	}
	updatedAtExpr := "CURRENT_TIMESTAMP"
	if db.Migrator().HasColumn("meme_vectors", "updated_at") {
		updatedAtExpr = "COALESCE(updated_at, CURRENT_TIMESTAMP)"
	}

	if err := db.Exec("DROP TABLE IF EXISTS meme_vectors__clean").Error; err != nil {
		return false, err
	}
	if err := db.Exec(`
		CREATE TABLE meme_vectors__clean (
			id TEXT PRIMARY KEY,
			meme_id TEXT NOT NULL,
			collection TEXT NOT NULL,
			vector_type INTEGER NOT NULL DEFAULT 1,
			embedding_model TEXT NOT NULL,
			input_hash TEXT,
			annotation_id TEXT,
			qdrant_point_id TEXT NOT NULL,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error; err != nil {
		return false, err
	}
	if err := db.Exec(fmt.Sprintf(`
		INSERT INTO meme_vectors__clean (
			id,
			meme_id,
			collection,
			vector_type,
			embedding_model,
			input_hash,
			annotation_id,
			qdrant_point_id,
			created_at,
			updated_at
		)
		SELECT
			id,
			%s,
			COALESCE(collection, ''),
			%s,
			COALESCE(embedding_model, ''),
			%s,
			%s,
			COALESCE(qdrant_point_id, ''),
			%s,
			%s
		FROM meme_vectors
	`, memeIDExpr, vectorTypeExpr, inputHashExpr, annotationIDExpr, createdAtExpr, updatedAtExpr)).Error; err != nil {
		return false, err
	}
	if err := db.Exec("DROP TABLE meme_vectors").Error; err != nil {
		return false, err
	}
	return true, db.Exec("ALTER TABLE meme_vectors__clean RENAME TO meme_vectors").Error
}

func preparePostgresLegacyMemeVectorsForAutoMigrate(db *gorm.DB) error {
	if !db.Migrator().HasColumn("meme_vectors", "vector_type") {
		return nil
	}
	for _, columnType := range columnTypes(db, "meme_vectors") {
		if !strings.EqualFold(columnType.Name(), "vector_type") {
			continue
		}
		dbType := strings.ToLower(columnType.DatabaseTypeName())
		if strings.Contains(dbType, "int") {
			return nil
		}
		if err := db.Exec(`ALTER TABLE meme_vectors ALTER COLUMN vector_type DROP DEFAULT`).Error; err != nil {
			return err
		}
		if err := db.Exec(`
			ALTER TABLE meme_vectors
			ALTER COLUMN vector_type TYPE INTEGER
			USING CASE
				WHEN vector_type::text = 'caption' THEN 2
				WHEN vector_type::text = '2' THEN 2
				ELSE 1
			END
		`).Error; err != nil {
			return err
		}
		return db.Exec(`ALTER TABLE meme_vectors ALTER COLUMN vector_type SET DEFAULT 1`).Error
	}
	return nil
}

func needsSQLiteMemeVectorRebuild(db *gorm.DB) bool {
	requiredColumns := []string{
		"id",
		"meme_id",
		"collection",
		"vector_type",
		"embedding_model",
		"input_hash",
		"annotation_id",
		"qdrant_point_id",
		"created_at",
		"updated_at",
	}
	for _, column := range requiredColumns {
		if !db.Migrator().HasColumn("meme_vectors", column) {
			return true
		}
	}

	legacyColumns := []string{
		"md5_hash",
		"embedding_provider",
		"embedding_mode",
		"dimension",
		"status",
		"description_id",
	}
	for _, column := range legacyColumns {
		if db.Migrator().HasColumn("meme_vectors", column) {
			return true
		}
	}

	for _, columnType := range columnTypes(db, "meme_vectors") {
		name := strings.EqualFold(columnType.Name(), "vector_type")
		dbType := strings.EqualFold(columnType.DatabaseTypeName(), "integer")
		if name && !dbType {
			return true
		}
	}
	return false
}

func columnTypes(db *gorm.DB, table string) []gorm.ColumnType {
	columnTypes, err := db.Migrator().ColumnTypes(table)
	if err != nil {
		return nil
	}
	return columnTypes
}

func migrateMemes(db *gorm.DB) error {
	if db.Migrator().HasColumn("memes", "md5_hash") &&
		db.Migrator().HasColumn(&domain.Meme{}, "content_hash") {
		if err := db.Exec(`
			UPDATE memes
			SET content_hash = md5_hash
			WHERE (content_hash IS NULL OR content_hash = '')
				AND md5_hash IS NOT NULL
				AND md5_hash != ''
		`).Error; err != nil {
			return err
		}
	}
	if db.Migrator().HasColumn("memes", "width") &&
		db.Migrator().HasColumn("memes", "height") &&
		db.Migrator().HasColumn("memes", "format") &&
		db.Migrator().HasColumn(&domain.Meme{}, "image_info") {
		return db.Exec(`
			UPDATE memes
			SET image_info =
				'{"width":' || COALESCE(CAST(width AS TEXT), '0') ||
				',"height":' || COALESCE(CAST(height AS TEXT), '0') ||
				',"format":' ||
				CASE lower(COALESCE(format, ''))
					WHEN 'jpg' THEN '1'
					WHEN 'jpeg' THEN '1'
					WHEN 'png' THEN '2'
					WHEN 'webp' THEN '3'
					ELSE '0'
				END ||
				'}'
			WHERE image_info IS NULL OR image_info = '' OR image_info = '{}'
		`).Error
	}
	return nil
}

func migrateMemeVectorIndexes(db *gorm.DB) error {
	if db.Migrator().HasColumn(&domain.MemeVector{}, "vector_type") {
		if err := db.Exec(`
			UPDATE meme_vectors
			SET vector_type = CASE
				WHEN CAST(vector_type AS TEXT) = 'caption' THEN 2
				WHEN CAST(vector_type AS TEXT) = '2' THEN 2
				ELSE 1
			END
			WHERE CAST(vector_type AS TEXT) IN ('', '0', 'image', 'caption', 'fused', '1', '2')
		`).Error; err != nil {
			return err
		}
	}
	if err := db.Exec("DROP INDEX IF EXISTS idx_meme_vectors_md5_collection").Error; err != nil {
		return err
	}
	if err := db.Exec("DROP INDEX IF EXISTS idx_meme_vectors_md5_collection_type").Error; err != nil {
		return err
	}
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_meme_vectors_meme_collection_type
		ON meme_vectors(meme_id, collection, vector_type)
	`).Error; err != nil {
		return err
	}
	return db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_meme_vectors_annotation
		ON meme_vectors(annotation_id)
	`).Error
}

func migrateMemeAnnotations(db *gorm.DB) error {
	if db.Migrator().HasTable("meme_descriptions") {
		if err := copyLegacyDescriptionsToAnnotations(db); err != nil {
			return err
		}
	}
	if db.Migrator().HasColumn("meme_vectors", "description_id") &&
		db.Migrator().HasColumn(&domain.MemeVector{}, "annotation_id") {
		return db.Exec(`
			UPDATE meme_vectors
			SET annotation_id = description_id
			WHERE (annotation_id IS NULL OR annotation_id = '')
				AND description_id IS NOT NULL
				AND description_id != ''
		`).Error
	}
	return nil
}

// disableCoreTableRLS makes sure Row Level Security is OFF on the core
// tables on Postgres. This is intentionally a "force-off" (rather than a
// no-op) because earlier iterations of this codebase enabled RLS on
// memes / meme_annotations / meme_vectors as a "deny everything except
// service role" guard for Supabase deployments — but never paired that with
// explicit REVOKE / CREATE POLICY statements. The result was implicit and
// easy to misconfigure: production silently relied on a BYPASSRLS service
// role to read its own data.
//
// Access control is now handled at the connection layer (the backend always
// connects with a service-role DSN and Supabase's anon/authenticated REST
// API does not expose these tables for this project), so RLS here would
// only add confusion. Running this migration on a database that previously
// had RLS enabled will turn it off; on a fresh database it is a no-op.
func disableCoreTableRLS(db *gorm.DB) error {
	if db.Dialector.Name() != "postgres" {
		return nil
	}
	for _, table := range []string{"memes", "meme_annotations", "meme_vectors"} {
		if !db.Migrator().HasTable(table) {
			continue
		}
		if err := db.Exec(fmt.Sprintf(`ALTER TABLE "%s" DISABLE ROW LEVEL SECURITY`, table)).Error; err != nil {
			return err
		}
	}
	return nil
}

// dropLegacyArtifacts removes columns, indexes, and tables that were part of the
// pre-2026-05-01 schema. We do this in code (rather than via a separate SQL
// migration runner) because the project uses GORM AutoMigrate as its single
// source of truth — and AutoMigrate intentionally never drops anything.
//
// The cleanup runs after every other migration step so that data has already
// been backfilled into the new columns by the time legacy ones are dropped.
func dropLegacyArtifacts(db *gorm.DB) error {
	if err := dropLegacyMemesColumns(db); err != nil {
		return fmt.Errorf("drop legacy memes columns: %w", err)
	}
	if err := dropLegacyMemeVectorsColumns(db); err != nil {
		return fmt.Errorf("drop legacy meme_vectors columns: %w", err)
	}
	if err := finalizeMemesConstraints(db); err != nil {
		return fmt.Errorf("finalize memes constraints: %w", err)
	}
	if err := dropLegacyIndexes(db); err != nil {
		return fmt.Errorf("drop legacy indexes: %w", err)
	}
	if err := dropLegacyTables(db); err != nil {
		return fmt.Errorf("drop legacy tables: %w", err)
	}
	return nil
}

// legacyMemesColumns lists every column that ever appeared on `memes` and was
// later removed in favor of `content_hash` / `image_info` / `meme_annotations`
// / `meme_vectors`. Listed here so a Postgres install carrying old columns
// gets cleaned up automatically the next time the API/ingest binary boots.
var legacyMemesColumns = []string{
	"source_type",
	"source_id",
	"local_path",
	"width",
	"height",
	"format",
	"is_animated",
	"file_size",
	"md5_hash",
	"perceptual_hash",
	"status",
	"embedding_model",
	"qdrant_point_id",
	"vlm_description",
	"vlm_model",
}

func dropLegacyMemesColumns(db *gorm.DB) error {
	if !db.Migrator().HasTable("memes") {
		return nil
	}
	// Pass the model struct rather than a table name so the SQLite driver can
	// build a schema for the table-rebuild path; on Postgres the driver issues
	// individual ALTER TABLE DROP COLUMN statements and the model is only used
	// as a context handle.
	for _, column := range legacyMemesColumns {
		if !db.Migrator().HasColumn("memes", column) {
			continue
		}
		if err := db.Migrator().DropColumn(&domain.Meme{}, column); err != nil {
			return fmt.Errorf("memes.%s: %w", column, err)
		}
	}
	return nil
}

// legacyMemeVectorsColumns are columns that the new `meme_vectors` model no
// longer carries. SQLite installs already get these dropped indirectly via
// the table-rebuild path in prepareLegacyMemeVectorsForAutoMigrate; this
// function is therefore mostly Postgres-relevant, but is dialect-agnostic
// for safety.
var legacyMemeVectorsColumns = []string{
	"md5_hash",
	"embedding_provider",
	"embedding_mode",
	"dimension",
	"status",
	"description_id",
}

func dropLegacyMemeVectorsColumns(db *gorm.DB) error {
	if !db.Migrator().HasTable("meme_vectors") {
		return nil
	}
	for _, column := range legacyMemeVectorsColumns {
		if !db.Migrator().HasColumn("meme_vectors", column) {
			continue
		}
		if err := db.Migrator().DropColumn(&domain.MemeVector{}, column); err != nil {
			return fmt.Errorf("meme_vectors.%s: %w", column, err)
		}
	}
	return nil
}

// finalizeMemesConstraints applies the NOT NULL / DEFAULT constraints that
// GORM AutoMigrate cannot retroactively add to pre-existing columns. SQLite
// installs already get these from the GORM tags at table-creation time, so
// this is a no-op outside Postgres.
func finalizeMemesConstraints(db *gorm.DB) error {
	if db.Dialector.Name() != "postgres" {
		return nil
	}
	if !db.Migrator().HasTable("memes") {
		return nil
	}
	statements := []string{
		`ALTER TABLE memes ALTER COLUMN storage_key SET NOT NULL`,
		`ALTER TABLE memes ALTER COLUMN content_hash SET NOT NULL`,
		`ALTER TABLE memes ALTER COLUMN image_info SET DEFAULT '{}'`,
		`ALTER TABLE memes ALTER COLUMN image_info SET NOT NULL`,
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("apply %q: %w", stmt, err)
		}
	}
	return nil
}

// legacyIndexNames are indexes that were created by historical SQL migrations
// or earlier GORM models but no longer correspond to any current model tag.
var legacyIndexNames = []string{
	"idx_memes_md5",
	"idx_meme_vectors_md5_collection",
	"idx_meme_vectors_md5_collection_type",
	"idx_meme_vectors_vector_type",
	"idx_meme_vectors_description",
	"idx_meme_descriptions_md5_model",
	"idx_meme_descriptions_meme",
}

func dropLegacyIndexes(db *gorm.DB) error {
	for _, name := range legacyIndexNames {
		if err := db.Exec(fmt.Sprintf("DROP INDEX IF EXISTS %s", name)).Error; err != nil {
			return fmt.Errorf("drop index %s: %w", name, err)
		}
	}
	return nil
}

// legacyTables are entire tables that were superseded by the
// memes / meme_annotations / meme_vectors trio.
var legacyTables = []string{
	"meme_descriptions",
	"data_sources",
	"ingest_jobs",
}

func dropLegacyTables(db *gorm.DB) error {
	for _, table := range legacyTables {
		if !db.Migrator().HasTable(table) {
			continue
		}
		if err := db.Migrator().DropTable(table); err != nil {
			return fmt.Errorf("drop legacy table %s: %w", table, err)
		}
	}
	return nil
}

type legacyMemeDescription struct {
	ID          string
	MemeID      string
	VLMModel    string
	Description string
	OCRText     string
	CreatedAt   time.Time
}

func copyLegacyDescriptionsToAnnotations(db *gorm.DB) error {
	var rows []legacyMemeDescription
	if err := db.Table("meme_descriptions").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if row.ID == "" {
			continue
		}
		createdAt := row.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		labels := domain.AnnotationLabels{}
		if row.OCRText != "" {
			presence, _ := domain.TextPresenceFromOCRText(row.OCRText)
			labels.Text = &domain.TextLabel{Present: presence == domain.TextPresenceWithText}
		}
		annotation := domain.MemeAnnotation{
			ID:            row.ID,
			MemeID:        row.MemeID,
			AnalyzerModel: row.VLMModel,
			Description:   row.Description,
			OCRText:       row.OCRText,
			Labels:        labels,
			CreatedAt:     createdAt,
			UpdatedAt:     time.Now(),
		}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&annotation).Error; err != nil {
			return err
		}
	}
	return nil
}

// initPostgres initializes a PostgreSQL database connection using the unified DSN
func initPostgres(cfg *config.DatabaseConfig, gormConfig *gorm.Config) (*gorm.DB, error) {
	dsn := cfg.DSN()
	// Use postgres.New with PreferSimpleProtocol: true to support Transaction Poolers (like Supabase port 6543)
	// This disables implicit prepared statements which are incompatible with transaction pooling
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true,
	}), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	return db, nil
}

// initSQLite initializes a SQLite database connection
func initSQLite(cfg *config.DatabaseConfig, gormConfig *gorm.Config) (*gorm.DB, error) {
	// Ensure the directory exists
	if cfg.Path != "" {
		dir := filepath.Dir(cfg.Path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	dsn := cfg.DSN()
	db, err := gorm.Open(sqlite.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SQLite: %w", err)
	}

	// Enable WAL mode for better concurrency (SQLite specific)
	// These are PRAGMA statements, separate from the connection pool settings
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA foreign_keys=ON")

	return db, nil
}

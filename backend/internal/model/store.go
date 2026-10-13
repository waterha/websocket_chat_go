package model

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ DB *pgxpool.Pool }

func NewID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes[:])
}

func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (store *Store) Migrate(ctx context.Context, directory string) error {
	entries, err := filepath.Glob(filepath.Join(directory, "*.up.sql"))
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("no migrations in %s", directory)
	}
	sort.Strings(entries)
	connection, err := store.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	defer connection.Release()
	if _, err = connection.Exec(ctx, "SELECT pg_advisory_lock(73182401)"); err != nil {
		return err
	}
	defer connection.Exec(context.Background(), "SELECT pg_advisory_unlock(73182401)")
	if _, err = connection.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	for _, filename := range entries {
		version := strings.TrimSuffix(filepath.Base(filename), ".up.sql")
		var applied bool
		if err = connection.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)", version).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		if version == "000001_initial" {
			var legacy bool
			if err = connection.QueryRow(ctx, "SELECT to_regclass('users') IS NOT NULL AND to_regclass('messages') IS NOT NULL AND to_regclass('conversation_members') IS NOT NULL AND to_regclass('sessions') IS NOT NULL AND to_regclass('conversations') IS NOT NULL").Scan(&legacy); err != nil {
				return err
			}
			if legacy {
				if _, err = connection.Exec(ctx, "INSERT INTO schema_migrations(version) VALUES($1)", version); err != nil {
					return err
				}
				continue
			}
		}
		sql, readErr := os.ReadFile(filename)
		if readErr != nil {
			return readErr
		}
		tx, beginErr := connection.Begin(ctx)
		if beginErr != nil {
			return beginErr
		}
		if _, err = tx.Exec(ctx, string(sql)); err == nil {
			_, err = tx.Exec(ctx, "INSERT INTO schema_migrations(version) VALUES($1)", version)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", version, err)
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

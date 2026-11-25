//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vputt/rbpo_pvkg/internal/domain"
	"github.com/vputt/rbpo_pvkg/internal/service"
	"github.com/vputt/rbpo_pvkg/internal/storage/postgres"
)

const (
	trainingPassword   = "training-password-0001"
	expectedHashPrefix = "$2a$12$"
	unknownUserID      = int64(1000000)
)

// TestPasswordStorage закрывает первую часть критерия приёмки SR-09: в таблице
// пользователей пароля в читаемом или обратимом виде нет.
func TestPasswordStorage(t *testing.T) {
	pool := prepareDatabase(t)
	ctx := context.Background()
	store := postgres.NewUserStore(pool)
	users := service.NewUsers(store)

	if _, err := users.Create(ctx, "demo-user", trainingPassword, domain.RoleUser); err != nil {
		t.Fatalf("create training user: %v", err)
	}
	if _, err := users.Create(ctx, "demo-staff", trainingPassword, domain.RoleStaff); err != nil {
		t.Fatalf("create training staff: %v", err)
	}

	var userHash, staffHash string
	err := pool.QueryRow(ctx, `
		SELECT
			(SELECT password_hash FROM users WHERE login = 'demo-user'),
			(SELECT password_hash FROM users WHERE login = 'demo-staff')
	`).Scan(&userHash, &staffHash)
	if err != nil {
		t.Fatalf("read stored hashes: %v", err)
	}

	if userHash == staffHash {
		t.Fatal("two accounts with the same password must not share a stored value")
	}
	for login, hash := range map[string]string{"demo-user": userHash, "demo-staff": staffHash} {
		if strings.Contains(hash, trainingPassword) {
			t.Fatalf("%s: stored value contains the password itself", login)
		}
		if !strings.HasPrefix(hash, expectedHashPrefix) || len(hash) != 60 {
			t.Fatalf("%s: stored value = %q, want 60 bytes with prefix %q", login, hash, expectedHashPrefix)
		}
		if err := service.VerifyPassword(hash, trainingPassword); err != nil {
			t.Fatalf("%s: correct password must pass verification", login)
		}
		if err := service.VerifyPassword(hash, trainingPassword+"!"); err == nil {
			t.Fatalf("%s: wrong password must not pass verification", login)
		}
	}

	stored, err := store.ByLogin(ctx, "demo-staff")
	if err != nil {
		t.Fatalf("read user by login: %v", err)
	}
	if stored.Role != domain.RoleStaff || stored.PasswordHash != staffHash {
		t.Fatalf("stored user = %+v, want role %s and the same hash", stored, domain.RoleStaff)
	}
	if stored.FailedLoginCount != 0 || stored.LockedUntil != nil {
		t.Fatalf("new account must start without failures and without a lock, got %d and %v", stored.FailedLoginCount, stored.LockedUntil)
	}

	if _, err := users.Create(ctx, "demo-user", trainingPassword, domain.RoleUser); !errors.Is(err, domain.ErrLoginTaken) {
		t.Fatalf("duplicate login error = %v, want domain.ErrLoginTaken", err)
	}
	if _, err := store.ByLogin(ctx, "nobody"); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("unknown login error = %v, want domain.ErrUserNotFound", err)
	}
}

// TestLoginFailureCounters проверяет операции, на которых кусок 7 строит ограничение подбора.
func TestLoginFailureCounters(t *testing.T) {
	pool := prepareDatabase(t)
	ctx := context.Background()
	store := postgres.NewUserStore(pool)

	user, err := service.NewUsers(store).Create(ctx, "demo-user", trainingPassword, domain.RoleUser)
	if err != nil {
		t.Fatalf("create training user: %v", err)
	}

	for want := 1; want <= 2; want++ {
		count, err := store.IncrementLoginFailures(ctx, user.ID)
		if err != nil {
			t.Fatalf("increment login failures: %v", err)
		}
		if count != want {
			t.Fatalf("failed login count = %d, want %d", count, want)
		}
	}

	lockedUntil, err := store.LockLogin(ctx, user.ID, 15*time.Minute)
	if err != nil {
		t.Fatalf("lock login: %v", err)
	}
	var lockIsAhead bool
	err = pool.QueryRow(ctx, `
		SELECT locked_until = $2
		   AND locked_until - CURRENT_TIMESTAMP BETWEEN interval '14 minutes' AND interval '15 minutes'
		FROM users
		WHERE id = $1
	`, user.ID, lockedUntil).Scan(&lockIsAhead)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if !lockIsAhead {
		t.Fatalf("stored lock does not match the returned %v or is not 15 minutes ahead of database time", lockedUntil)
	}

	locked, err := store.ByLogin(ctx, "demo-user")
	if err != nil {
		t.Fatalf("read locked user: %v", err)
	}
	if locked.LockedUntil == nil || !locked.LockedUntil.Equal(lockedUntil) {
		t.Fatalf("locked until = %v, want %v", locked.LockedUntil, lockedUntil)
	}

	if err := store.ResetLoginFailures(ctx, user.ID); err != nil {
		t.Fatalf("reset login failures: %v", err)
	}
	released, err := store.ByLogin(ctx, "demo-user")
	if err != nil {
		t.Fatalf("read released user: %v", err)
	}
	if released.FailedLoginCount != 0 || released.LockedUntil != nil {
		t.Fatalf("after reset: count = %d, locked until = %v, want 0 and no lock", released.FailedLoginCount, released.LockedUntil)
	}

	if _, err := store.IncrementLoginFailures(ctx, unknownUserID); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("increment for unknown user = %v, want domain.ErrUserNotFound", err)
	}
	if _, err := store.LockLogin(ctx, unknownUserID, time.Minute); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("lock for unknown user = %v, want domain.ErrUserNotFound", err)
	}
	if err := store.ResetLoginFailures(ctx, unknownUserID); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("reset for unknown user = %v, want domain.ErrUserNotFound", err)
	}
}

func prepareDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv(testDSNEnvironment)
	if dsn == "" {
		t.Fatalf("%s is required and must point to a separate test database", testDSNEnvironment)
	}

	source, err := iofs.New(os.DirFS(migrationsPath(t)), ".")
	if err != nil {
		t.Fatalf("open migrations: %v", err)
	}
	migration, err := migrate.NewWithSourceInstance("iofs", source, migrationDatabaseURL(t, dsn))
	if err != nil {
		t.Fatalf("create migration runner: %v", err)
	}
	if err := migration.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("reset test database: %v", err)
	}
	if err := migration.Up(); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("create test database pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		if err := migration.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			t.Errorf("roll back migrations: %v", err)
		}
		closeMigration(t, migration)
	})
	return pool
}

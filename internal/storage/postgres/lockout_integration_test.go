//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vputt/rbpo_pvkg/internal/domain"
	"github.com/vputt/rbpo_pvkg/internal/service"
	"github.com/vputt/rbpo_pvkg/internal/storage/postgres"
)

// Значения из критерия приёмки SR-09 и из значений по умолчанию конфигурации.
const (
	loginMaxFails = 5
	loginLockFor  = 15 * time.Minute
)

// TestLockoutAfterRepeatedFailures закрывает вторую часть критерия приёмки SR-09.
func TestLockoutAfterRepeatedFailures(t *testing.T) {
	pool := prepareDatabase(t)
	ctx := context.Background()
	user := createTrainingUser(t, pool)
	auth := newAuthService(t, pool)

	for attempt := 1; attempt <= loginMaxFails; attempt++ {
		if _, err := auth.Login(ctx, "demo-user", trainingPassword+"!"); !errors.Is(err, service.ErrInvalidCredentials) {
			t.Fatalf("attempt %d: error = %v, want service.ErrInvalidCredentials", attempt, err)
		}
	}

	result, err := auth.Login(ctx, "demo-user", trainingPassword)
	if !errors.Is(err, service.ErrLoginLocked) {
		t.Fatalf("login with the correct password while locked: error = %v, want service.ErrLoginLocked", err)
	}
	if result.Token != "" {
		t.Fatal("a locked account must not receive a token")
	}

	failures, lockedUntil := loginState(t, pool, user.ID)
	if failures != loginMaxFails {
		t.Fatalf("failed login count = %d, want %d", failures, loginMaxFails)
	}
	if !lockAheadByQuarterHour(t, pool, user.ID) {
		t.Fatalf("locked until %v, want 15 minutes ahead of database time", lockedUntil)
	}
	if sessions := sessionCount(t, pool); sessions != 0 {
		t.Fatalf("session count = %d, want 0 while the account is locked", sessions)
	}

	// Попытки во время блокировки её не продлевают (D-04).
	for attempt := 1; attempt <= 3; attempt++ {
		if _, err := auth.Login(ctx, "demo-user", trainingPassword+"?"); !errors.Is(err, service.ErrLoginLocked) {
			t.Fatalf("attempt %d during the lock: error = %v, want service.ErrLoginLocked", attempt, err)
		}
	}
	failuresAfter, lockedUntilAfter := loginState(t, pool, user.ID)
	if failuresAfter != failures || !lockedUntilAfter.Equal(*lockedUntil) {
		t.Fatalf("after attempts during the lock: count %d and lock %v, want %d and %v",
			failuresAfter, lockedUntilAfter, failures, lockedUntil)
	}

	// Срок блокировки истёк: счётчик обнуляется, верный пароль проходит.
	if _, err := pool.Exec(ctx, `UPDATE users SET locked_until = CURRENT_TIMESTAMP - interval '1 minute' WHERE id = $1`, user.ID); err != nil {
		t.Fatalf("move the lock into the past: %v", err)
	}
	result, err = auth.Login(ctx, "demo-user", trainingPassword)
	if err != nil {
		t.Fatalf("login after the lock expired: %v", err)
	}
	if result.Token == "" {
		t.Fatal("login after the lock expired returned no token")
	}
	released, lockAfterRelease := loginState(t, pool, user.ID)
	if released != 0 || lockAfterRelease != nil {
		t.Fatalf("after a successful login: count = %d, lock = %v, want 0 and no lock", released, lockAfterRelease)
	}
	if sessions := sessionCount(t, pool); sessions != 1 {
		t.Fatalf("session count = %d, want 1", sessions)
	}
}

func TestSuccessfulLoginClearsFailures(t *testing.T) {
	pool := prepareDatabase(t)
	ctx := context.Background()
	user := createTrainingUser(t, pool)
	auth := newAuthService(t, pool)

	for attempt := 1; attempt < loginMaxFails; attempt++ {
		if _, err := auth.Login(ctx, "demo-user", trainingPassword+"!"); !errors.Is(err, service.ErrInvalidCredentials) {
			t.Fatalf("attempt %d: error = %v, want service.ErrInvalidCredentials", attempt, err)
		}
	}
	if failures, _ := loginState(t, pool, user.ID); failures != loginMaxFails-1 {
		t.Fatalf("failed login count = %d, want %d", failures, loginMaxFails-1)
	}

	if _, err := auth.Login(ctx, "demo-user", trainingPassword); err != nil {
		t.Fatalf("login with the correct password: %v", err)
	}
	failures, lockedUntil := loginState(t, pool, user.ID)
	if failures != 0 || lockedUntil != nil {
		t.Fatalf("after a successful login: count = %d, lock = %v, want 0 and no lock", failures, lockedUntil)
	}

	// Счётчик действительно обнулён: следующая неудача начинает счёт заново.
	if _, err := auth.Login(ctx, "demo-user", trainingPassword+"!"); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("error = %v, want service.ErrInvalidCredentials", err)
	}
	if failures, lockedUntil = loginState(t, pool, user.ID); failures != 1 || lockedUntil != nil {
		t.Fatalf("after one more failure: count = %d, lock = %v, want 1 and no lock", failures, lockedUntil)
	}
}

// TestConcurrentFailuresKeepTheCount: пять одновременных неудачных попыток дают ровно
// пять в счётчике и блокировку, то есть порог срабатывает и при параллельных запросах.
func TestConcurrentFailuresKeepTheCount(t *testing.T) {
	pool := prepareDatabase(t)
	ctx := context.Background()
	user := createTrainingUser(t, pool)
	auth := newAuthService(t, pool)

	var attempts sync.WaitGroup
	for attempt := 0; attempt < loginMaxFails; attempt++ {
		attempts.Add(1)
		go func() {
			defer attempts.Done()
			if _, err := auth.Login(ctx, "demo-user", trainingPassword+"!"); !errors.Is(err, service.ErrInvalidCredentials) {
				t.Errorf("concurrent attempt: error = %v, want service.ErrInvalidCredentials", err)
			}
		}()
	}
	attempts.Wait()

	failures, lockedUntil := loginState(t, pool, user.ID)
	if failures != loginMaxFails {
		t.Fatalf("failed login count = %d, want %d: parallel attempts lost the count", failures, loginMaxFails)
	}
	if lockedUntil == nil {
		t.Fatal("the account must be locked after the threshold is reached by parallel attempts")
	}
	if _, err := auth.Login(ctx, "demo-user", trainingPassword); !errors.Is(err, service.ErrLoginLocked) {
		t.Fatalf("error = %v, want service.ErrLoginLocked", err)
	}
}

// TestConcurrentFailuresAfterExpiredLockKeepTheCount - случай, ради которого по D-04 чтение,
// сравнение и обновление счётчика идут в одной транзакции. Истёкшая блокировка обнуляет
// счётчик, а это чтение и запись двумя запросами: без блокировки строки параллельные попытки
// увидят истёкшую блокировку все вместе, каждая обнулит счёт, и порог не сработает.
func TestConcurrentFailuresAfterExpiredLockKeepTheCount(t *testing.T) {
	pool := prepareDatabase(t)
	ctx := context.Background()
	user := createTrainingUser(t, pool)
	auth := newAuthService(t, pool)

	_, err := pool.Exec(ctx, `
		UPDATE users
		SET failed_login_count = $2, locked_until = CURRENT_TIMESTAMP - interval '1 minute'
		WHERE id = $1
	`, user.ID, loginMaxFails)
	if err != nil {
		t.Fatalf("set an expired lock: %v", err)
	}

	var attempts sync.WaitGroup
	for attempt := 0; attempt < loginMaxFails; attempt++ {
		attempts.Add(1)
		go func() {
			defer attempts.Done()
			if _, err := auth.Login(ctx, "demo-user", trainingPassword+"!"); !errors.Is(err, service.ErrInvalidCredentials) {
				t.Errorf("concurrent attempt after the lock expired: error = %v, want service.ErrInvalidCredentials", err)
			}
		}()
	}
	attempts.Wait()

	failures, lockedUntil := loginState(t, pool, user.ID)
	if failures != loginMaxFails {
		t.Fatalf("failed login count = %d, want %d: parallel attempts reset the count more than once", failures, loginMaxFails)
	}
	if lockedUntil == nil || !lockAheadByQuarterHour(t, pool, user.ID) {
		t.Fatalf("locked until %v, want a fresh lock 15 minutes ahead of database time", lockedUntil)
	}
}

func newAuthService(t *testing.T, pool *pgxpool.Pool) *service.Auth {
	t.Helper()
	auth, err := service.NewAuth(postgres.NewTransactionRunner(pool), service.AuthSettings{
		SessionTTL:    sessionTTL,
		LoginMaxFails: loginMaxFails,
		LoginLockFor:  loginLockFor,
	})
	if err != nil {
		t.Fatalf("create auth service: %v", err)
	}
	return auth
}

func createTrainingUser(t *testing.T, pool *pgxpool.Pool) domain.User {
	t.Helper()
	user, err := service.NewUsers(postgres.NewUserStore(pool)).Create(context.Background(), "demo-user", trainingPassword, domain.RoleUser)
	if err != nil {
		t.Fatalf("create training user: %v", err)
	}
	return user
}

func loginState(t *testing.T, pool *pgxpool.Pool, userID int64) (int, *time.Time) {
	t.Helper()
	var failures int
	var lockedUntil *time.Time
	err := pool.QueryRow(context.Background(), `
		SELECT failed_login_count, locked_until FROM users WHERE id = $1
	`, userID).Scan(&failures, &lockedUntil)
	if err != nil {
		t.Fatalf("read login state: %v", err)
	}
	return failures, lockedUntil
}

func lockAheadByQuarterHour(t *testing.T, pool *pgxpool.Pool, userID int64) bool {
	t.Helper()
	var ahead bool
	err := pool.QueryRow(context.Background(), `
		SELECT locked_until - CURRENT_TIMESTAMP BETWEEN interval '14 minutes' AND interval '15 minutes'
		FROM users
		WHERE id = $1
	`, userID).Scan(&ahead)
	if err != nil {
		t.Fatalf("read lock interval: %v", err)
	}
	return ahead
}

func sessionCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var sessions int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	return sessions
}

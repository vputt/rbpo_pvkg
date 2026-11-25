//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vputt/rbpo_pvkg/internal/service"
)

const sessionTTL = 8 * time.Hour

// TestLoginStoresOnlyTokenHash закрывает условие куска: в таблице sessions нет самого
// токена ни в одном поле, лежит только его SHA-256 (D-01).
func TestLoginStoresOnlyTokenHash(t *testing.T) {
	pool := prepareDatabase(t)
	ctx := context.Background()
	user := createTrainingUser(t, pool)

	result, err := newAuthService(t, pool).Login(ctx, "demo-user", trainingPassword)
	if err != nil {
		t.Fatalf("login with the correct password: %v", err)
	}
	if len(result.Token) != 43 {
		t.Fatalf("token length = %d, want 43 characters of base64url for 32 bytes", len(result.Token))
	}

	var storedHash []byte
	var storedUserID int64
	var expiresAsExpected bool
	var tokenFoundInRow int
	err = pool.QueryRow(ctx, `
		SELECT token_hash,
		       user_id,
		       expires_at = $1
		         AND expires_at - CURRENT_TIMESTAMP BETWEEN interval '7 hours 59 minutes' AND interval '8 hours',
		       strpos(sessions::text, $2)
		FROM sessions
	`, result.ExpiresAt, result.Token).Scan(&storedHash, &storedUserID, &expiresAsExpected, &tokenFoundInRow)
	if err != nil {
		t.Fatalf("read stored session: %v", err)
	}

	expectedHash, err := service.SessionTokenHash(result.Token)
	if err != nil {
		t.Fatalf("hash the returned token: %v", err)
	}
	if !bytes.Equal(storedHash, expectedHash) {
		t.Fatal("stored value is not the SHA-256 of the returned token")
	}
	if len(storedHash) != 32 {
		t.Fatalf("stored hash length = %d, want 32", len(storedHash))
	}
	if tokenFoundInRow != 0 {
		t.Fatal("the token itself must not appear in any column of sessions")
	}
	if storedUserID != user.ID {
		t.Fatalf("session user_id = %d, want %d", storedUserID, user.ID)
	}
	if !expiresAsExpected {
		t.Fatal("expires_at must match the returned value and be the configured lifetime ahead of database time")
	}
}

func TestLoginRefusesInvalidCredentials(t *testing.T) {
	pool := prepareDatabase(t)
	ctx := context.Background()
	createTrainingUser(t, pool)
	auth := newAuthService(t, pool)

	start := time.Now()
	if _, err := auth.Login(ctx, "demo-user", trainingPassword+"!"); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v, want service.ErrInvalidCredentials", err)
	}
	wrongPassword := time.Since(start)

	start = time.Now()
	if _, err := auth.Login(ctx, "nobody", trainingPassword); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("unknown login error = %v, want service.ErrInvalidCredentials", err)
	}
	unknownLogin := time.Since(start)

	if sessions := sessionCount(t, pool); sessions != 0 {
		t.Fatalf("session count = %d, want 0 after refused logins", sessions)
	}

	// По D-04 неизвестный логин тоже проходит сравнение с хешем-заглушкой, поэтому
	// быстрого ответа тут быть не должно: без заглушки эта ветка укладывается в единицы миллисекунд.
	t.Logf("wrong password: %v, unknown login: %v", wrongPassword.Round(time.Millisecond), unknownLogin.Round(time.Millisecond))
	if unknownLogin < 50*time.Millisecond {
		t.Fatalf("unknown login answered in %v: the dummy hash comparison is missing", unknownLogin)
	}
}

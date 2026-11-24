// Пакет postgres содержит репозитории на pgx.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/vputt/rbpo_pvkg/internal/domain"
)

// DB - часть pgx, одинаково реализованная пулом и транзакцией: репозиторий можно
// построить и на том, и на другом, когда нужна одна транзакция на несколько запросов.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type UserStore struct {
	db DB
}

var _ domain.UserRepository = (*UserStore)(nil)

func NewUserStore(db DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) Create(ctx context.Context, user domain.User) (domain.User, error) {
	err := s.db.QueryRow(ctx, `
		INSERT INTO users (login, password_hash, role)
		VALUES ($1, $2, $3)
		RETURNING id, failed_login_count, locked_until
	`, user.Login, user.PasswordHash, user.Role).
		Scan(&user.ID, &user.FailedLoginCount, &user.LockedUntil)
	if err != nil {
		var pgError *pgconn.PgError
		if errors.As(err, &pgError) && pgError.Code == pgerrcode.UniqueViolation {
			return domain.User{}, domain.ErrLoginTaken
		}
		return domain.User{}, fmt.Errorf("создание пользователя: %w", err)
	}
	return user, nil
}

func (s *UserStore) ByLogin(ctx context.Context, login string) (domain.User, error) {
	var user domain.User
	err := s.db.QueryRow(ctx, `
		SELECT id, login, password_hash, role, failed_login_count, locked_until
		FROM users
		WHERE login = $1
	`, login).
		Scan(&user.ID, &user.Login, &user.PasswordHash, &user.Role, &user.FailedLoginCount, &user.LockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("чтение пользователя: %w", err)
	}
	return user, nil
}

func (s *UserStore) IncrementLoginFailures(ctx context.Context, userID int64) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `
		UPDATE users
		SET failed_login_count = failed_login_count + 1
		WHERE id = $1
		RETURNING failed_login_count
	`, userID).Scan(&count)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrUserNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("увеличение счётчика неудачных попыток входа: %w", err)
	}
	return count, nil
}

// LockLogin считает срок блокировки от времени сервера БД: по D-04 запись и проверка
// блокировки идут от одного источника времени.
func (s *UserStore) LockLogin(ctx context.Context, userID int64, lockFor time.Duration) (time.Time, error) {
	var lockedUntil time.Time
	err := s.db.QueryRow(ctx, `
		UPDATE users
		SET locked_until = CURRENT_TIMESTAMP + make_interval(secs => $2)
		WHERE id = $1
		RETURNING locked_until
	`, userID, lockFor.Seconds()).Scan(&lockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, domain.ErrUserNotFound
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("блокировка входа: %w", err)
	}
	return lockedUntil, nil
}

func (s *UserStore) ResetLoginFailures(ctx context.Context, userID int64) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE users
		SET failed_login_count = 0, locked_until = NULL
		WHERE id = $1
	`, userID)
	if err != nil {
		return fmt.Errorf("сброс счётчика неудачных попыток входа: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

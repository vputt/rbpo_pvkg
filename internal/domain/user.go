package domain

import (
	"context"
	"errors"
	"time"
)

const (
	RoleUser  = "user"
	RoleStaff = "staff"
)

var (
	ErrUserNotFound = errors.New("пользователь не найден")
	ErrLoginTaken   = errors.New("логин уже занят")
)

type User struct {
	ID               int64
	Login            string
	PasswordHash     string
	Role             string
	FailedLoginCount int
	LockedUntil      *time.Time
}

func KnownRole(role string) bool {
	return role == RoleUser || role == RoleStaff
}

// UserRepository хранит пользователей. Счётчик попыток и срок блокировки меняются
// отдельными операциями: каждая выполняется одним запросом, а их согласование
// между собой - дело вызывающего кода (D-04).
type UserRepository interface {
	Create(ctx context.Context, user User) (User, error)
	ByLogin(ctx context.Context, login string) (User, error)
	IncrementLoginFailures(ctx context.Context, userID int64) (int, error)
	LockLogin(ctx context.Context, userID int64, lockFor time.Duration) (time.Time, error)
	ResetLoginFailures(ctx context.Context, userID int64) error
}

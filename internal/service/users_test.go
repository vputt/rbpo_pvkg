package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/vputt/rbpo_pvkg/internal/domain"
	"github.com/vputt/rbpo_pvkg/internal/service"
)

const trainingPassword = "training-password-0001"

// Стоимость 12 зафиксирована в D-04, поэтому ожидаемый префикс записан в тесте явно:
// изменение стоимости должно проходить через правку решения.
const expectedHashPrefix = "$2a$12$"

type repositoryStub struct {
	created []domain.User
}

var _ domain.UserRepository = (*repositoryStub)(nil)

func (r *repositoryStub) Create(ctx context.Context, user domain.User) (domain.User, error) {
	user.ID = int64(len(r.created) + 1)
	r.created = append(r.created, user)
	return user, nil
}

func (r *repositoryStub) ByLogin(ctx context.Context, login string) (domain.User, error) {
	for _, user := range r.created {
		if user.Login == login {
			return user, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}

func (r *repositoryStub) ByLoginForUpdate(ctx context.Context, login string) (domain.User, time.Time, error) {
	user, err := r.ByLogin(ctx, login)
	return user, time.Now(), err
}

func (r *repositoryStub) IncrementLoginFailures(ctx context.Context, userID int64) (int, error) {
	return 0, nil
}

func (r *repositoryStub) LockLogin(ctx context.Context, userID int64, lockFor time.Duration) (time.Time, error) {
	return time.Time{}, nil
}

func (r *repositoryStub) ResetLoginFailures(ctx context.Context, userID int64) error {
	return nil
}

func TestCreateStoresHashInsteadOfPassword(t *testing.T) {
	repository := &repositoryStub{}
	users := service.NewUsers(repository)

	created, err := users.Create(context.Background(), "  demo-user  ", trainingPassword, domain.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	if created.Login != "demo-user" {
		t.Fatalf("login = %q, want it stored without surrounding spaces", created.Login)
	}
	if strings.Contains(created.PasswordHash, trainingPassword) {
		t.Fatal("password must not be stored as is")
	}
	if !strings.HasPrefix(created.PasswordHash, expectedHashPrefix) || len(created.PasswordHash) != 60 {
		t.Fatalf("hash = %q, want 60 bytes with prefix %q", created.PasswordHash, expectedHashPrefix)
	}
	if err := service.VerifyPassword(created.PasswordHash, trainingPassword); err != nil {
		t.Fatal("correct password must pass verification")
	}
	if err := service.VerifyPassword(created.PasswordHash, trainingPassword+"!"); !errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		t.Fatalf("wrong password verification error = %v, want a mismatch", err)
	}
}

func TestSamePasswordGetsDifferentHashes(t *testing.T) {
	first, err := service.HashPassword(trainingPassword)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.HashPassword(trainingPassword)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two hashes of the same password must differ")
	}
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name     string
		login    string
		password string
		role     string
		want     error
	}{
		{"blank login", "   ", trainingPassword, domain.RoleUser, service.ErrEmptyLogin},
		{"empty password", "demo-user", "", domain.RoleUser, service.ErrEmptyPassword},
		{"unknown role", "demo-user", trainingPassword, "admin", service.ErrUnknownRole},
	}

	repository := &repositoryStub{}
	users := service.NewUsers(repository)
	for _, testCase := range cases {
		_, err := users.Create(context.Background(), testCase.login, testCase.password, testCase.role)
		if !errors.Is(err, testCase.want) {
			t.Errorf("%s: error = %v, want %v", testCase.name, err, testCase.want)
		}
	}
	if len(repository.created) != 0 {
		t.Fatal("rejected input must not reach the repository")
	}
}

// bcrypt учитывает только первые 72 байта пароля (ограничение D-04): остаток
// не должен молча отбрасываться.
func TestHashPasswordRejectsLongPassword(t *testing.T) {
	if _, err := service.HashPassword(strings.Repeat("a", 73)); !errors.Is(err, bcrypt.ErrPasswordTooLong) {
		t.Fatalf("error = %v, want bcrypt.ErrPasswordTooLong", err)
	}
}

func BenchmarkVerifyPassword(b *testing.B) {
	hash, err := service.HashPassword(trainingPassword)
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if err := service.VerifyPassword(hash, trainingPassword); err != nil {
			b.Fatal(err)
		}
	}
}

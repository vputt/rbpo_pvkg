// Пакет service реализует сценарии продукта.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/vputt/rbpo_pvkg/internal/domain"
)

// Стоимость bcrypt взята из D-04.
const PasswordCost = 12

var (
	ErrEmptyLogin    = errors.New("логин не задан")
	ErrEmptyPassword = errors.New("пароль не задан")
	ErrUnknownRole   = errors.New("роль должна быть user или staff")
)

type Users struct {
	repository domain.UserRepository
}

func NewUsers(repository domain.UserRepository) *Users {
	return &Users{repository: repository}
}

// Create - единственный путь появления пользователя: пароль уходит в хранилище только хешем.
func (s *Users) Create(ctx context.Context, login, password, role string) (domain.User, error) {
	login = strings.TrimSpace(login)
	switch {
	case login == "":
		return domain.User{}, ErrEmptyLogin
	case password == "":
		return domain.User{}, ErrEmptyPassword
	case !domain.KnownRole(role):
		return domain.User{}, ErrUnknownRole
	}

	hash, err := HashPassword(password)
	if err != nil {
		return domain.User{}, err
	}
	return s.repository.Create(ctx, domain.User{Login: login, PasswordHash: hash, Role: role})
}

// HashPassword возвращает хеш bcrypt: соль и стоимость библиотека помещает в саму строку хеша.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), PasswordCost)
	if err != nil {
		return "", fmt.Errorf("хеширование пароля: %w", err)
	}
	return string(hash), nil
}

func VerifyPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

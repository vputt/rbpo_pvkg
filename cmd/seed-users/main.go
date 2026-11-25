package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vputt/rbpo_pvkg/internal/config"
	"github.com/vputt/rbpo_pvkg/internal/domain"
	"github.com/vputt/rbpo_pvkg/internal/service"
	"github.com/vputt/rbpo_pvkg/internal/storage/postgres"
)

const (
	envUserLogin     = "PVKG_SEED_USER_LOGIN"
	envUserPassword  = "PVKG_SEED_USER_PASSWORD"
	envStaffLogin    = "PVKG_SEED_STAFF_LOGIN"
	envStaffPassword = "PVKG_SEED_STAFF_PASSWORD"
)

const (
	defaultUserLogin  = "demo-user"
	defaultStaffLogin = "demo-staff"
	// Длинный пароль - требование D-04 к учебным записям: блокировка замедляет подбор, но не останавливает его.
	minPasswordLength = 12
	seedTimeout       = 30 * time.Second
)

type account struct {
	login    string
	password string
	role     string
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("настройки приложения: %w", err)
	}

	accounts, err := accountsFromEnvironment()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), seedTimeout)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DBDSN)
	if err != nil {
		return errors.New("не удалось создать пул подключений к PostgreSQL")
	}
	defer pool.Close()

	users := service.NewUsers(postgres.NewUserStore(pool))
	for _, item := range accounts {
		created, err := users.Create(ctx, item.login, item.password, item.role)
		switch {
		case errors.Is(err, domain.ErrLoginTaken):
			log.Printf("запись %s уже существует, пароль не изменён", item.login)
		case err != nil:
			return fmt.Errorf("учебная запись %s: %w", item.login, err)
		default:
			log.Printf("создана запись %s, роль %s, id %d", created.Login, created.Role, created.ID)
		}
	}
	return nil
}

func accountsFromEnvironment() ([]account, error) {
	userPassword, err := password(envUserPassword)
	if err != nil {
		return nil, err
	}
	staffPassword, err := password(envStaffPassword)
	if err != nil {
		return nil, err
	}
	if userPassword == staffPassword {
		return nil, fmt.Errorf("пароли %s и %s совпадают: учебным записям нужны разные пароли", envUserPassword, envStaffPassword)
	}

	return []account{
		{login: login(envUserLogin, defaultUserLogin), password: userPassword, role: domain.RoleUser},
		{login: login(envStaffLogin, defaultStaffLogin), password: staffPassword, role: domain.RoleStaff},
	}, nil
}

func login(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

// password не возвращается в сообщениях об ошибках и не печатается.
func password(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("не задана переменная окружения %s - пароль учебной записи; значения по умолчанию у него нет", name)
	}
	if len(value) < minPasswordLength {
		return "", fmt.Errorf("переменная окружения %s: пароль учебной записи короче %d байт", name, minPasswordLength)
	}
	return value, nil
}

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vputt/rbpo_pvkg/internal/config"
	"github.com/vputt/rbpo_pvkg/internal/httpapi"
	"github.com/vputt/rbpo_pvkg/internal/service"
	"github.com/vputt/rbpo_pvkg/internal/storage/postgres"
)

const shutdownTimeout = 5 * time.Second

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

	poolConfig, err := pgxpool.ParseConfig(cfg.DBDSN)
	if err != nil {
		return errors.New("не удалось настроить подключение к PostgreSQL")
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return errors.New("не удалось создать пул подключений к PostgreSQL")
	}
	defer pool.Close()

	auth, err := service.NewAuth(postgres.NewTransactionRunner(pool), service.AuthSettings{
		SessionTTL:    cfg.SessionTTL,
		LoginMaxFails: cfg.LoginMaxFails,
		LoginLockFor:  cfg.LoginLockFor,
	})
	if err != nil {
		return err
	}

	router := httpapi.NewRouter(nil, httpapi.NewAuthHandlers(auth), pool)
	server := &http.Server{
		Addr:    cfg.Addr,
		Handler: router,
	}

	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverError := make(chan error, 1)
	log.Printf("HTTP API: %s", cfg.Addr)
	go func() {
		serverError <- server.ListenAndServe()
	}()

	select {
	case err := <-serverError:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP API: %w", err)
	case <-signalContext.Done():
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		return fmt.Errorf("остановка HTTP API: %w", err)
	}

	if err := <-serverError; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("HTTP API: %w", err)
	}
	return nil
}

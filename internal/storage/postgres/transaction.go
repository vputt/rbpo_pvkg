package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vputt/rbpo_pvkg/internal/domain"
)

type TransactionRunner struct {
	pool *pgxpool.Pool
}

func NewTransactionRunner(pool *pgxpool.Pool) *TransactionRunner {
	return &TransactionRunner{pool: pool}
}

// InTransaction выполняет fn в одной транзакции и передаёт ей репозитории, привязанные
// к этой же транзакции. Ошибка от fn означает откат, поэтому прикладной отказ - например,
// неверный пароль - возвращается наружу не ошибкой fn: иначе откат отменил бы и запись,
// которую отказ должен оставить, вроде увеличенного счётчика попыток.
func (r *TransactionRunner) InTransaction(ctx context.Context, fn func(users domain.UserRepository, sessions domain.SessionRepository) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("начало транзакции: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := fn(NewUserStore(tx), NewSessionStore(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("фиксация транзакции: %w", err)
	}
	return nil
}

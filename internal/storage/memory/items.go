package memory

import (
	"context"
	"sync"

	"github.com/vputt/rbpo_pvkg/internal/domain"
)

type Store struct {
	mu     sync.RWMutex
	lastID int64
	items  []domain.Item
}

var _ domain.ItemRepository = (*Store)(nil)

func New() *Store {
	return &Store{}
}

func (s *Store) Create(ctx context.Context, item domain.Item) (domain.Item, error) {
	if err := ctx.Err(); err != nil {
		return domain.Item{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastID++
	item.ID = s.lastID
	item.Status = domain.StatusAvailable
	s.items = append(s.items, item)
	return item, nil
}

func (s *Store) List(ctx context.Context) ([]domain.Item, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]domain.Item, len(s.items))
	copy(items, s.items)
	return items, nil
}

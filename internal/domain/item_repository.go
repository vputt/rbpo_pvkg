package domain

import "context"

type ItemRepository interface {
	Create(ctx context.Context, item Item) (Item, error)
	List(ctx context.Context) ([]Item, error)
}

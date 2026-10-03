package memory_test

import (
	"context"
	"sync"
	"testing"

	"github.com/vputt/rbpo_pvkg/internal/domain"
	"github.com/vputt/rbpo_pvkg/internal/storage/memory"
)

func TestCreateAndList(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	items, err := store.List(ctx)
	if err != nil || items == nil || len(items) != 0 {
		t.Fatal("new store must return an empty list")
	}
	created, err := store.Create(ctx, domain.Item{
		ID: 99, Category: "keys", PublicDescription: "two keys", Status: "issued",
	})
	if err != nil || created.ID != 1 || created.Status != domain.StatusAvailable {
		t.Fatal("store must assign the ID and initial status")
	}
	items, err = store.List(ctx)
	if err != nil || len(items) != 1 || items[0] != created {
		t.Fatal("created item is missing from the list")
	}
	items[0].PublicDescription = "changed"
	items, _ = store.List(ctx)
	if items[0].PublicDescription != created.PublicDescription {
		t.Fatal("changing a returned list must not change stored data")
	}
	empty, _ := memory.New().List(ctx)
	if len(empty) != 0 {
		t.Fatal("independent stores must not share data")
	}
}

func TestConcurrentCreation(t *testing.T) {
	const count = 20
	store := memory.New()
	var workers sync.WaitGroup
	for i := 0; i < count; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if _, err := store.Create(context.Background(), domain.Item{Category: "keys"}); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	items, err := store.List(context.Background())
	if err != nil || len(items) != count {
		t.Fatal("concurrent creation lost records")
	}
	ids := make(map[int64]bool)
	for _, item := range items {
		if item.ID <= 0 || ids[item.ID] {
			t.Fatal("item IDs must be unique")
		}
		ids[item.ID] = true
	}
}

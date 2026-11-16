package store_test

import (
	"context"
	"sync"
	"testing"

	"github.com/locodarwin/mila/server/internal/db"
	"github.com/locodarwin/mila/server/internal/dbtest"
	"github.com/locodarwin/mila/server/internal/store"
)

func TestInstanceSettings(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		s := store.New(d)

		if _, ok, err := s.InstanceSetting(ctx, "greeting"); err != nil || ok {
			t.Fatalf("unset key: ok=%v err=%v", ok, err)
		}
		if err := s.SetInstanceSetting(ctx, "greeting", "hello"); err != nil {
			t.Fatal(err)
		}
		if err := s.SetInstanceSetting(ctx, "greeting", "hi"); err != nil {
			t.Fatal(err)
		}
		if v, ok, err := s.InstanceSetting(ctx, "greeting"); err != nil || !ok || v != "hi" {
			t.Fatalf("got %q ok=%v err=%v, want hi", v, ok, err)
		}
	})
}

func TestInitInstanceSettingKeepsFirstValue(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d *db.DB) {
		ctx := context.Background()
		s := store.New(d)

		// Racing initializers must all see the same winner.
		var wg sync.WaitGroup
		got := make([]string, 8)
		for i := range got {
			wg.Add(1)
			go func() {
				defer wg.Done()
				v, err := s.InitInstanceSetting(ctx, "instance_id", string(rune('a'+i)))
				if err != nil {
					t.Error(err)
				}
				got[i] = v
			}()
		}
		wg.Wait()
		for _, v := range got {
			if v != got[0] {
				t.Fatalf("initializers disagree: %v", got)
			}
		}
		if v, err := s.InitInstanceSetting(ctx, "instance_id", "late"); err != nil || v != got[0] {
			t.Errorf("late initializer got %q, want %q", v, got[0])
		}
	})
}

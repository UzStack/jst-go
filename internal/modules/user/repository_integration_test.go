//go:build integration

// Integration tests for the postgres repository. They require Docker and run
// only with `-tags integration` (see `make test-integration`), keeping the
// default `go test ./...` fast and Docker-free.
package user_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/UzStack/jst-go/internal/modules/auth"
	"github.com/UzStack/jst-go/internal/modules/user"
	"github.com/UzStack/jst-go/internal/shared/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func newStore(t *testing.T) *database.Store {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("jstgo_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	if err := database.MigrateUp(dsn, "file://../../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return database.NewStore(pool)
}

func TestRepository_CRUD(t *testing.T) {
	repo := user.NewPostgresRepository(newStore(t))
	ctx := context.Background()

	u := &user.User{ID: uuid.New(), Email: "a@b.com", Name: "A", PasswordHash: "x"}
	if err := repo.Create(ctx, u); err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.Role != user.RoleUser {
		t.Errorf("default role = %q, want %q", u.Role, user.RoleUser)
	}

	// duplicate email -> ErrEmailTaken
	dup := &user.User{ID: uuid.New(), Email: "a@b.com", Name: "dup", PasswordHash: "x"}
	if err := repo.Create(ctx, dup); !errors.Is(err, user.ErrEmailTaken) {
		t.Errorf("duplicate create err = %v, want ErrEmailTaken", err)
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil || got.Email != "a@b.com" {
		t.Fatalf("getByID: %v, %+v", err, got)
	}

	if _, err := repo.UpdateRole(ctx, u.ID, user.RoleAdmin); err != nil {
		t.Fatalf("updateRole: %v", err)
	}
	got, _ = repo.GetByID(ctx, u.ID)
	if got.Role != user.RoleAdmin {
		t.Errorf("role = %q, want admin", got.Role)
	}

	// list + count + search
	list, err := repo.List(ctx, user.ListFilter{Limit: 10})
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v, len=%d", err, len(list))
	}
	if n, _ := repo.Count(ctx, "a@b"); n != 1 {
		t.Errorf("count search = %d, want 1", n)
	}
	if n, _ := repo.Count(ctx, "nomatch"); n != 0 {
		t.Errorf("count no-match = %d, want 0", n)
	}

	if err := repo.Delete(ctx, u.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, u.ID); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("after delete err = %v, want ErrNotFound", err)
	}
}

// The point of carrying the transaction in the context: two modules that know
// nothing about each other write through one transaction, and one failure undoes
// both. Neither repository is passed a tx — they find it on the ctx.
func TestStore_DoRollsBackAcrossModules(t *testing.T) {
	store := newStore(t)
	users := user.NewPostgresRepository(store)
	tokens := auth.NewRefreshStore(store)
	ctx := context.Background()

	u := &user.User{ID: uuid.New(), Email: "tx@b.com", Name: "TX", PasswordHash: "x"}
	jti := uuid.New()

	wantErr := errors.New("checkout failed")
	err := store.Do(ctx, func(ctx context.Context) error {
		if err := users.Create(ctx, u); err != nil {
			return err
		}
		if err := tokens.Save(ctx, jti, u.ID, time.Now().Add(time.Hour)); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Do err = %v, want %v", err, wantErr)
	}

	if _, err := users.GetByID(ctx, u.ID); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("user survived rollback: err = %v, want ErrNotFound", err)
	}
	if active, err := tokens.IsActive(ctx, jti); err != nil || active {
		t.Errorf("refresh token survived rollback: active = %v, err = %v", active, err)
	}
}

func TestStore_DoCommitsAcrossModules(t *testing.T) {
	store := newStore(t)
	users := user.NewPostgresRepository(store)
	tokens := auth.NewRefreshStore(store)
	ctx := context.Background()

	u := &user.User{ID: uuid.New(), Email: "ok@b.com", Name: "OK", PasswordHash: "x"}
	jti := uuid.New()

	if err := store.Do(ctx, func(ctx context.Context) error {
		if err := users.Create(ctx, u); err != nil {
			return err
		}
		return tokens.Save(ctx, jti, u.ID, time.Now().Add(time.Hour))
	}); err != nil {
		t.Fatalf("Do: %v", err)
	}

	if _, err := users.GetByID(ctx, u.ID); err != nil {
		t.Errorf("user not committed: %v", err)
	}
	if active, err := tokens.IsActive(ctx, jti); err != nil || !active {
		t.Errorf("token not committed: active = %v, err = %v", active, err)
	}
}

// A usecase that opens its own transaction must stay safe to call from a larger
// one: the inner failure undoes only the inner work.
func TestStore_NestedDoRollsBackToSavepoint(t *testing.T) {
	store := newStore(t)
	users := user.NewPostgresRepository(store)
	ctx := context.Background()

	outer := &user.User{ID: uuid.New(), Email: "outer@b.com", Name: "O", PasswordHash: "x"}
	inner := &user.User{ID: uuid.New(), Email: "inner@b.com", Name: "I", PasswordHash: "x"}

	if err := store.Do(ctx, func(ctx context.Context) error {
		if err := users.Create(ctx, outer); err != nil {
			return err
		}
		nestedErr := store.Do(ctx, func(ctx context.Context) error {
			if err := users.Create(ctx, inner); err != nil {
				return err
			}
			return errors.New("inner failed")
		})
		if nestedErr == nil {
			t.Error("nested Do err = nil, want error")
		}
		return nil // outer swallows the inner failure and commits its own work
	}); err != nil {
		t.Fatalf("outer Do: %v", err)
	}

	if _, err := users.GetByID(ctx, outer.ID); err != nil {
		t.Errorf("outer write lost: %v", err)
	}
	if _, err := users.GetByID(ctx, inner.ID); !errors.Is(err, database.ErrNotFound) {
		t.Errorf("inner write survived savepoint rollback: err = %v", err)
	}
}

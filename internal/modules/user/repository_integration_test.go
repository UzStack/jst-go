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

	"github.com/UzStack/jst-go/internal/modules/user"
	"github.com/UzStack/jst-go/internal/shared/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func newPostgres(t *testing.T) *pgxpool.Pool {
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
	return pool
}

func TestRepository_CRUD(t *testing.T) {
	repo := user.NewPostgresRepository(newPostgres(t))
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

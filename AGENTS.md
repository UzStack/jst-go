# AGENTS.md — AI agent guide for jst-go

Bu fayl AI agentlar (Claude Code, Cursor, Copilot, Codex va boshqalar) uchun. Loyihada **mos** va **maintainable** kod yozish qoidalari shu yerda. Inson dasturchilarga ham foydali.

## 1. Loyiha haqida qisqacha

**jst-go** — production-tayyor Go template, **feature-modular clean architecture** asosida. Stack: `gin + pgx + sqlc + golang-migrate + viper + zap`. Maqsadi: kichik loyihada overkill bo'lmaslik, lekin katta loyihada o'sa olish.

Module nomi: `github.com/example/goapp` (yangi loyihada o'zingiznikiga almashtiriladi).

## 2. Direktoriya strukturasi — qoida

```
cmd/api/                    # entry point — faqat main.go
internal/
  modules/<name>/           # har feature shu yerda, self-contained
    domain.go               # entity + Repository interface
    repository.go           # pgx + sqlc implementatsiya (adapter)
    usecase.go              # business logic — faqat domain interface'ga bog'liq
    dto.go                  # request/response struct'lari, validate teglari
    handler.go              # gin handlerlar
    routes.go               # RegisterRoutes funksiyasi
  shared/                   # cross-cutting infra — modulga tegishli emas
    config/                 # viper config loader
    database/               # pgxpool + migrate + sqlc generated kod
    httpx/                  # AppError + standart response shape
    logger/                 # zap wrapper
    middleware/             # logger, recovery, auth (TokenVerifier interface)
    validator/              # singleton validator
  server/                   # DI wiring + router
migrations/                 # *.up.sql / *.down.sql (golang-migrate)
queries/                    # sqlc input (.sql fayllar :one/:many/:exec)
configs/config.yaml         # default config
```

### Qaerga nima yoziladi

| Vazifa | Joy |
|---|---|
| Yangi entity (User, Order, Post) | `internal/modules/<name>/` — yangi papka |
| Yangi business logic operatsiya | mavjud `<name>/usecase.go` |
| Yangi HTTP endpoint | `<name>/handler.go` + `<name>/routes.go` |
| Yangi SQL query | `queries/<table>.sql` → `make sqlc` |
| Yangi jadval | `make migrate-new NAME=...` → `migrations/` |
| Cross-cutting helper (har modul ishlatadigan) | `internal/shared/<area>/` |
| pkg-da yangi public API | **kerak emas** — `pkg/` papkasi yo'q, hammasi `internal/` |

## 3. Arxitektura qoidalari — buzilmasin

### 3.1 Bog'lanish yo'nalishi (dependency direction)

```
handler ──> usecase ──> domain (Repository interface)
              │              ▲
              ▼              │
         dto + httpx     repository (pgx/sqlc) implementatsiya qiladi
```

- **Usecase faqat interface'ga bog'lanadi** (`Repository`, `TokenVerifier`). Concrete tipga (`*pgRepo`) bog'lanmaydi.
- **Domain (`domain.go`) hech narsaga bog'lanmaydi** — `database/sql`, `gin`, `zap`, `pgx` import qilmaydi.
- **Repository concrete tipi** (`pgRepo`) — package'dan tashqariga **eksport qilinmaydi**. Faqat `NewPostgresRepository(pool) Repository` eksport qilinadi.

### 3.2 Modullararo bog'lanish

- Modullar **bir-birining concrete tipiga** murojaat qilmaydi. Faqat **eksport qilingan interfeyslar** orqali.
- Misol: `auth` moduli `user.Usecase` interfeysiga bog'lanadi (concrete `*usecase` ga emas).
- Circular import paydo bo'lsa — yangi interface yarat (`shared/middleware/auth.go`'da `TokenVerifier` shu sababli).

### 3.3 Shared paketga nima qo'shiladi

`internal/shared/` ga **kamida 2 modul ishlatishi kerak**. Bitta modul uchun — shu modul ichida qoldir.

## 4. Database qoidalari

### 4.1 Query yozish workflow'i

**Har doim sqlc orqali** — manual pgx queryni `repository.go`'da yozma (dynamic WHERE bundan istisno).

```sql
-- queries/posts.sql

-- name: CreatePost :one
INSERT INTO posts (user_id, title, body) VALUES ($1, $2, $3) RETURNING *;

-- name: GetPostByID :one
SELECT * FROM posts WHERE id = $1;

-- name: ListPostsByUser :many
SELECT * FROM posts WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: DeletePost :execrows
DELETE FROM posts WHERE id = $1 AND user_id = $2;
```

Keyin:
```bash
make sqlc
```

Repository'da:
```go
func (r *pgRepo) GetByID(ctx context.Context, id uuid.UUID) (*Post, error) {
    row, err := r.queries.GetPostByID(ctx, id)
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, database.ErrNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("get post: %w", err)
    }
    p := fromSQLC(row)
    return &p, nil
}
```

### 4.2 sqlc tag qo'llanmasi

| Tag | Qachon |
|---|---|
| `:one` | Aniq 1 qator (yo'q bo'lsa `pgx.ErrNoRows`) |
| `:many` | Slice qaytaradi |
| `:exec` | UPDATE/DELETE — RowsAffected kerak emas |
| `:execrows` | UPDATE/DELETE — `(int64, error)` — "topilmadi" tekshiruvi uchun |

### 4.3 Generated kodga TEGMA

`internal/shared/database/sqlc/*.go` — **DO NOT EDIT** ushbu kommentariy bilan. SQL'ni o'zgartir, `make sqlc` qayta yurit.

### 4.4 Migration qoidalari

- Yaratish: `make migrate-new NAME=add_orders_status`
- Har migration **reversible** bo'lsin — `up.sql` va `down.sql` ikkalasi to'liq.
- Production'da **destructive migration** (DROP COLUMN, RENAME) — alohida deploy qiladi (multi-step), bitta bo'lib emas.
- Migration faylni qo'lda **o'zgartirma** (push qilingan bo'lsa) — yangi migration yoz.

### 4.5 Domain ↔ persistence model mapping

sqlc'ning model (`sqlcdb.User`) **domain.User**'dan alohida. Repository'da convertor funksiya yoz:

```go
func fromSQLC(r sqlcdb.User) User {
    return User{ID: r.ID, Email: r.Email, ...}
}
```

Identik bo'lsa ham — bu chegara muhim. Sxema o'zgarsa, domain shokga uchramaydi.

### 4.6 Transaction

Pool'dan `Begin(ctx)` qil, sqlc'ning `WithTx(tx)` orqali queries ol:

```go
tx, err := r.pool.Begin(ctx)
if err != nil { return err }
defer tx.Rollback(ctx)  // commit bo'lsa noop

q := r.queries.WithTx(tx)
if _, err := q.CreatePost(ctx, ...); err != nil { return err }
if err := q.IncrementCount(ctx, userID); err != nil { return err }
return tx.Commit(ctx)
```

Buning uchun `pgRepo` strukturasiga `pool *pgxpool.Pool` qaytaring.

## 5. Xatolar — AppError pattern

### 5.1 Usecase qatlamida

Domain xatosi → `httpx.AppError` qaytar:

```go
func (u *usecase) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
    user, err := u.repo.GetByID(ctx, id)
    if err != nil {
        if errors.Is(err, database.ErrNotFound) {
            return nil, httpx.NotFound("user.not_found", "user not found")
        }
        return nil, httpx.Internal(err)
    }
    return user, nil
}
```

Mavjud helperlar: `httpx.NotFound`, `BadRequest`, `Unauthorized`, `Forbidden`, `Conflict`, `Internal`.

### 5.2 Handler qatlamida

```go
if err != nil {
    httpx.Error(c, err)   // AppError bo'lsa to'g'ri status; aks holda 500
    return
}
```

### 5.3 Code naming konvensiyasi

`<resource>.<reason>` — snake_case:
- `user.not_found`
- `user.email_taken`
- `auth.invalid_token`
- `auth.invalid_credentials`
- `request.malformed`
- `request.invalid`

Frontend bu kodlarga binoan localized xabar ko'rsatadi.

### 5.4 Xato yozishda qoidalar

- **Internal xatoni clientga ko'rsatma**: `httpx.Internal(err)` faqat `"internal server error"` qaytaradi, lekin loglarda to'liq.
- **fmt.Errorf("xxx: %w", err)** bilan o'rab boring (wrap), `%v` emas.
- **panic ishlatma** — `httpx.Internal(err)` qaytar.

## 6. HTTP handler qoidalari

### 6.1 Standart shablon

```go
func (h *Handler) Create(c *gin.Context) {
    var req CreateRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        httpx.Error(c, httpx.BadRequest("request.malformed", "invalid request body"))
        return
    }
    if details, err := validator.Struct(req); err != nil {
        httpx.ErrorWithDetails(c, httpx.BadRequest("request.invalid", "validation failed"), details)
        return
    }

    result, err := h.uc.Create(c.Request.Context(), req)
    if err != nil {
        httpx.Error(c, err)
        return
    }
    httpx.OK(c, http.StatusCreated, toResponse(result))
}
```

- Handler **biznes logikasini bajarmaydi** — faqat parse + validate + delegate + respond.
- Har doim `c.Request.Context()`'ni usecase'ga uzating.
- Kontekstga maxsus qiymat (user_id) qo'shmaslik — middleware'dan `middleware.UserID(c)` orqali oling.

### 6.2 DTO va domain — alohida

`dto.go`'da request/response struct, `domain.go`'da entity. Ularni **aralashtirma**. Convertor funksiya: `toResponse(entity)`.

```go
type UserResponse struct {
    ID        string    `json:"id"`
    Email     string    `json:"email"`
    Name      string    `json:"name"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

func toResponse(u *User) UserResponse {
    return UserResponse{ID: u.ID.String(), Email: u.Email, ...}
}
```

`PasswordHash` kabi maydonlarni javobga **chiqarma**.

### 6.3 Validatsiya teglari

```go
type RegisterRequest struct {
    Email    string `json:"email" validate:"required,email"`
    Name     string `json:"name" validate:"required,min=1,max=100"`
    Password string `json:"password" validate:"required,min=8,max=128"`
}
```

JSON tag nomi xato javobida ko'rinadi (validator.RegisterTagNameFunc shunga sozlangan).

## 7. Yangi modul qo'shish — checklist

Misol uchun `post` moduli kerak bo'lsin:

1. **Migration**:
   ```bash
   make migrate-new NAME=create_posts
   # up.sql va down.sql ni yozing
   make migrate-up
   ```

2. **Query**:
   ```bash
   # queries/posts.sql yozing
   make sqlc
   ```

3. **Module fayllar** (`internal/modules/post/`):
   - `domain.go` — `Post` entity + `Repository` interface
   - `repository.go` — `pgRepo` + sqlc mapping
   - `usecase.go` — `Usecase` interface + concrete
   - `dto.go` — request/response struct'lar
   - `handler.go` — gin handlers
   - `routes.go` — `RegisterRoutes(rg, h, verifier)`

4. **Wiring** (`internal/server/server.go`):
   ```go
   postRepo := post.NewPostgresRepository(s.pool)
   postUC := post.NewUsecase(postRepo)
   post.RegisterRoutes(v1, post.NewHandler(postUC), tokens)
   ```

5. **Test** — kamida usecase uchun `fakeRepo` bilan unit test.

## 8. Logging qoidalari

`zap` ishlatiladi (`internal/shared/logger`):

```go
log.Info("user created",
    logger.String("user_id", u.ID.String()),
    logger.String("email", u.Email),
)

log.Error("payment failed",
    logger.Err(err),
    logger.String("order_id", orderID.String()),
)
```

- **Production'da** sezgir ma'lumotlarni (password, token, PII) logga yozma.
- **Strukturali field** ishlat, message ichida `%s` bilan format qilma.
- `log.Fatal` faqat startup'da (DB connect, config load) ishlatiladi.

## 9. Test qoidalari

### 9.1 Unit test (usecase)

`fakeRepo` (in-memory Repository implementation) bilan. Real DB chaqirma.

```go
type fakeRepo struct { /* maps */ }
func (r *fakeRepo) Create(ctx context.Context, u *user.User) error { ... }
// ... boshqa metodlar
```

### 9.2 Integration test (repository)

[testcontainers-go](https://github.com/testcontainers/testcontainers-go) bilan real postgres ko'tarib `*pgxpool.Pool` ga `NewPostgresRepository`'ni qo'llang. **Mock DB ishlatma** — sxema xatolari prod'da paydo bo'ladi.

### 9.3 Test naming

`TestRegister_AndAuthenticate`, `TestRegister_DuplicateEmail` — `Test<Method>_<Case>` formatda.

```bash
make test    # race detector bilan
make cover   # coverage.html
```

## 10. Konfiguratsiya

- Yangi config qiymat qo'shilsa: `Config` struct'ga maydon + `setDefaults(v)` ga default qiymat + `configs/config.yaml`'ga misol + `.env.example`'ga `APP_*` versiya.
- Hech qachon **secret'larni codega yozma**. `APP_JWT_SECRET=...` env orqali.
- Production validatsiyasi: `Config.validate()` ichida `if env == "production" { ... }` bloki.

## 11. Naming konvensiyalari

| Element | Konvensiya | Misol |
|---|---|---|
| Package | qisqa, single-word, lowercase | `user`, `auth`, `httpx` |
| Interface | rolga ko'ra | `Repository`, `Usecase`, `TokenVerifier` |
| Constructor | `New<Type>` | `NewUsecase`, `NewPostgresRepository` |
| Error variable | `Err<Reason>` | `ErrEmailTaken`, `ErrNotFound` |
| HTTP route | RESTful, kebab-case yo'q, plural | `/users/me`, `/auth/login` |
| SQL query | PascalCase camelCase ham bo'lishi mumkin | `GetUserByEmail`, `ListPostsByUser` |
| Migration fayl | `seq_snake_case` | `000003_add_user_roles.up.sql` |
| AppError code | `<resource>.<reason>` | `user.not_found` |

## 12. Style — Go idioms

- **Faqat zarur paytda commentariy yoz**. Identifikator nomi yetarli bo'lsa, komment qo'shma.
- **Komment WHY tushuntirishi kerak, WHAT emas**. "Increments counter" — yomon. "Increments counter atomically to avoid race during high-load checkout" — yaxshi.
- **Receiver name**: `func (r *pgRepo) ...` — 1-3 harf, tip nomidan kelib chiqsin.
- **Error tekshiruvi**: `if err != nil { return ... }` — har doim ketma-ket, nested emas.
- **Context har doim birinchi parametr**: `func Do(ctx context.Context, ...)`.
- **defer ishlatish** resource cleanup uchun (`defer pool.Close()`, `defer tx.Rollback(ctx)`).
- **time.Duration** ishlat — `int` seconds emas (`timeout time.Duration`).

## 13. Backwards compatibility shimlar — TAQIQLANGAN

- Removed maydon uchun `// Deprecated` qoldirib o'tirma — o'chiraver.
- `// old: ...` kabi komment yozma.
- Versionlangan API kerak bo'lsa — `v1`, `v2` URL'larda emas, **alohida modul** sifatida.

## 14. Build/run buyruqlari (Makefile)

```bash
make help              # barcha buyruqlar
make install-tools     # air, migrate, sqlc, golangci-lint o'rnatish
make db-up             # postgres docker'da ko'tarish
make dev               # air bilan hot reload
make run               # oddiy go run
make build             # static binary
make test              # race detector bilan
make cover             # coverage.html
make lint              # golangci-lint
make fmt               # gofmt + goimports
make sqlc              # SQL → Go generation
make migrate-new NAME=...
make migrate-up / migrate-down / migrate-force V=...
make up / down         # to'liq docker stack
```

## 15. Agent uchun maxsus eslatmalar

- **Module nomi**: yangi loyihada `github.com/example/goapp` o'rniga real nom turishi kerak. Refactor paytida tekshiring.
- **DO NOT** `vendor/` papkasini commit qiling — `.gitignore`'da bor.
- **DO NOT** `.env` ni commit qiling — faqat `.env.example`.
- **DO NOT** generated sqlc kodini qo'lda tahrirla.
- **DO NOT** yangi feature uchun `pkg/` ochma — hamma kod `internal/`'da.
- **DO NOT** GORM, sqlx, yoki boshqa ORM qo'shma — sqlc + pgx yetadi.
- **DO NOT** `log.Println` ishlat — `zap` orqali.
- **DO NOT** global state qo'shma — DI orqali server.go'da wire qil.
- **DO** har feature uchun `domain.go` da `Repository` interface yoz.
- **DO** har repository metodida `errors.Is(err, pgx.ErrNoRows)` tekshir.
- **DO** har handler'da DTO validatsiyani 2 qadamga ajrat: `ShouldBindJSON` → `validator.Struct`.
- **DO** har commit oldidan `go build ./... && go vet ./... && go test ./...` ishlatib ko'r.

## 16. Production checklist (deploy oldidan)

- [ ] `APP_JWT_SECRET` 32+ baytli random (`openssl rand -hex 32`).
- [ ] `APP_ENV=production`.
- [ ] Migration alohida job/init container'da, `main.go`'dan olib tashlangan.
- [ ] HTTPS reverse proxy (nginx/caddy/traefik) orqasida.
- [ ] DB credentialslar secret manager'da.
- [ ] Rate limiting middleware qo'shilgan.
- [ ] OpenTelemetry/Prometheus tracing — kerak bo'lsa `internal/shared/middleware/`ga.
- [ ] Backup strategiyasi (postgres pg_dump cron yoki managed service).
- [ ] Healthcheck endpoint (`/healthz`) load balancer'da ulangan.

---

**Savol bo'lsa**: oldin `README.md`'ni o'qing, keyin shu faylni. Ikkalasida ham yo'q bo'lsa — issue yarating.

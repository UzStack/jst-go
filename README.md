# goapp — Go Clean Architecture Template

Production-tayyor Go template. Kichik loyihalarda overkill emas, kattalarida esa to'g'ridan-to'g'ri kengaytirish mumkin. Stack: **gin + pgx + golang-migrate + viper + zap + air + docker**. sqlc tayyor turibdi — istasangiz yoqasiz.

## Asosiy g'oyalar

- **Feature-modular** struktura: har feature `internal/modules/<name>/` ichida — `domain.go`, `repository.go`, `usecase.go`, `handler.go`, `dto.go`, `routes.go`. Bitta featureda ishlash bitta papkani ochish bilan tugaydi.
- **Clean architecture qatlamlari** (har modulda):
  - `domain.go` — pure entity + `Repository` interfeysi (port).
  - `repository.go` — pgx implementatsiyasi (adapter). Hand-written queries, lekin sqlc'ga oson o'tasiz.
  - `usecase.go` — biznes logika. Faqat `Repository` interfeysiga bog'liq.
  - `handler.go` + `dto.go` + `routes.go` — HTTP delivery.
- **Shared infra** `internal/shared/`: `config`, `logger`, `database`, `httpx` (response/error), `middleware`, `validator`.
- **AppError** orqali xato pattern: domain xatosi → HTTP javob mapping `httpx.Error()`'da.
- **Graceful shutdown**, **migrationlar avtomatik startda** (yoki CLI orqali qo'lda).
- **Zero generation required** ishga tushishi uchun: `sqlc` opsional, manual pgx queries default.

## Tezkor start

```bash
# 1. Module nomini o'zgartiring (default github.com/example/goapp)
# go.mod va barcha importlarni replace qiling, masalan:
#   find . -type f -name '*.go' -exec sed -i '' 's|github.com/example/goapp|github.com/youruser/yourproject|g' {} +
#   go mod edit -module github.com/youruser/yourproject

# 2. Dev toollar
make install-tools     # air, migrate, sqlc, golangci-lint

# 3. Database
make db-up             # postgresni docker compose orqali ko'taring

# 4. Run
make dev               # air bilan hot reload
# yoki:
make run

# 5. Health check
curl http://localhost:8080/healthz
```

`make dev` ishga tushishi bilan migrationlar avtomatik qo'llaniladi (`internal/shared/database/migrate.go`). Production'da bu xatti-harakatni o'zgartirib, CLI orqali boshqarishingiz mumkin.

## API namunasi

```bash
# Register
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"a@b.com","name":"A","password":"supersecret"}'

# Login
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"a@b.com","password":"supersecret"}'

# Me (access token bilan)
curl http://localhost:8080/api/v1/users/me \
  -H 'Authorization: Bearer <ACCESS_TOKEN>'
```

## Yangi modul qo'shish

1. `internal/modules/<name>/` papka yarat.
2. `domain.go` da entity + `Repository` interfeysi.
3. `repository.go` da pgx implementatsiyasi (`NewPostgresRepository`).
4. `usecase.go` da `Usecase` interfeysi va konkret tip.
5. `handler.go`, `dto.go`, `routes.go`.
6. `internal/server/server.go` ichida wiring qo'sh:
   ```go
   widgetRepo := widget.NewPostgresRepository(s.pool)
   widgetUC := widget.NewUsecase(widgetRepo)
   widget.RegisterRoutes(v1, widget.NewHandler(widgetUC), tokens)
   ```
7. Migration qo'sh: `make migrate-new NAME=create_widgets`.

Bu hammasi. Module o'zining ichida self-contained — boshqa modullarga faqat ularning eksport qilingan interfeyslari orqali bog'lanadi (masalan, `auth` modul `user.Usecase` interfeysidan foydalanadi).

## sqlc'ga o'tish (opsional)

Manual pgx queries yetarli — lekin SQL ko'paygach `sqlc` foydali bo'ladi:

```bash
make sqlc      # queries/*.sql -> internal/shared/database/sqlc/*.go
```

Keyin `repository.go` ichidagi pgx kodini sqlc generated wrapperga almashtiring. Domain interfeysi (`Repository`) o'zgarmaydi — usecase tegmasdan qoladi.

## Migrationlar

- Yaratish: `make migrate-new NAME=add_orders`
- Yuqori: `make migrate-up`
- Pastga: `make migrate-down`
- Versiyani majburlash (xato holatda): `make migrate-force V=2`

Migrate library startda avtomatik chaqiriladi (`MigrateUp`), shu sababli local dev'da alohida buyruq kerak emas. Production deploylarda buni o'chirib, alohida migration job ishlatsangiz xavfsizroq — `cmd/api/main.go`'dagi `MigrateUp` chaqiruvini olib tashlang.

## Konfiguratsiya

Viper `configs/config.yaml` + `APP_*` env vars. Production'da odatda env-only:

```
APP_DB_HOST=db.prod.local
APP_JWT_SECRET=$(openssl rand -hex 32)
APP_ENV=production
APP_LOG_LEVEL=info
```

## Testlash

- **Unit**: `internal/modules/user/usecase_test.go` — fakeRepo bilan.
- **Integration**: production'da [testcontainers-go](https://github.com/testcontainers/testcontainers-go) bilan real postgres ko'tarib `*pgxpool.Pool`'ga `NewPostgresRepository`'ni qo'llang.

```bash
make test        # race detector bilan
make cover       # coverage.html
```

## Struktura

```
.
├── cmd/api/main.go               # entry point, graceful shutdown
├── configs/config.yaml           # default config
├── internal/
│   ├── modules/
│   │   ├── user/                 # CRUD + me endpoints
│   │   └── auth/                 # JWT register/login/refresh
│   ├── shared/
│   │   ├── config/               # viper loader
│   │   ├── database/             # pgxpool + migrate
│   │   ├── httpx/                # AppError + response helpers
│   │   ├── logger/               # zap wrapper
│   │   ├── middleware/           # logger, recovery, auth
│   │   └── validator/            # validator instance
│   └── server/server.go          # DI + routing
├── migrations/                   # *.up.sql / *.down.sql
├── queries/                      # sqlc input (optional)
├── Dockerfile
├── docker-compose.yml
├── Makefile
├── sqlc.yaml
└── .air.toml
```

## Production checklist

- [ ] `APP_JWT_SECRET` 32+ baytli random qiymat.
- [ ] `APP_ENV=production` (gin release mode, slog-friendly).
- [ ] Migration startup-da emas, alohida job/init container'da.
- [ ] HTTPS reverse proxy (nginx/caddy/traefik) orqasida.
- [ ] DB credentialslar secret manager'da.
- [ ] OpenTelemetry/Prometheus tracing — kerak bo'lganda `internal/shared/middleware/`ga qo'shing.
- [ ] Rate limiting middleware.
- [ ] `httpServer.TLSConfig` agar to'g'ridan-to'g'ri TLS terminate qilsa.

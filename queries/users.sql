-- name: CreateUser :one
INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
VALUES ($1, $2, $3, $4, NOW(), NOW())
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: UpdateUserName :one
UPDATE users
SET name = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateUserRole :one
UPDATE users
SET role = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteUser :execrows
DELETE FROM users WHERE id = $1;

-- name: ListUsers :many
SELECT * FROM users
WHERE (sqlc.narg('search')::text IS NULL
       OR email ILIKE '%' || sqlc.narg('search') || '%'
       OR name ILIKE '%' || sqlc.narg('search') || '%')
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountUsers :one
SELECT COUNT(*) FROM users
WHERE (sqlc.narg('search')::text IS NULL
       OR email ILIKE '%' || sqlc.narg('search') || '%'
       OR name ILIKE '%' || sqlc.narg('search') || '%');

-- name: CreateUser :one
INSERT INTO users (name)
VALUES ($1)
RETURNING *;
-- name: UpdateUser :one
UPDATE users
SET name = $2
WHERE id = $1
RETURNING *;
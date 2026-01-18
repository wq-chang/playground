-- name: CreateUser :one
INSERT INTO users (
  id, username, email, first_name, last_name
) VALUES (
  @id, @username, @email, @first_name, @last_name
)
RETURNING *;

-- name: UpdateUser :execresult
UPDATE users
SET
  username = COALESCE(sqlc.narg('username'), username),
  email = COALESCE(sqlc.narg('email'), email),
  first_name = COALESCE(sqlc.narg('first_name'), first_name),
  first_name = COALESCE(sqlc.narg('last_name'), last_name)
WHERE id = @id;

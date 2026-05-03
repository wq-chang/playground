-- name: CreateUser :exec
INSERT INTO users (
  id, username, email, first_name, last_name
) VALUES (
  @id, @username, @email, @first_name, @last_name
);

-- name: UpdateUser :execresult
UPDATE users
SET
  username = COALESCE(sqlc.narg('username'), username),
  email = COALESCE(sqlc.narg('email'), email),
  first_name = COALESCE(sqlc.narg('first_name'), first_name),
  first_name = COALESCE(sqlc.narg('last_name'), last_name)
WHERE id = @id;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = @id;

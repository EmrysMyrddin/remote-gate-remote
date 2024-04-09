-- name: ListUsers :many
select * from "users";

-- name: GetUser :one
select * from "users" where id = $1;

-- name: GetUserByEmail :one
select * from "users" where email = $1;

-- name: CreateUser :one
insert into "users" (email, full_name, apartment, pwd_salt, pwd_hash, pwd_iterations, pwd_parallelism, pwd_memory, pwd_version, "role") 
values (
  $1, $2, $3, $4, $5, $6, $7, $8, $9,
  (select (case when count(id) = 0 then 'admin' else 'user' end) role from "users")
) 
returning *;

-- name: EmailVerified :one
update "users" set email_verified = true where id = $1 returning *;

-- name: DeleteUser :one
delete from "users" where id = $1 returning *;

-- name: DropAllUsers :exec
delete from "users";

-- name: CreateLog :one
insert into "logs" (user_id) values ($1) returning *;

-- name: ListLogs :many
select * from "logs";

-- name: ListLogsByUser :many
select * from "logs" where user_id = $1;

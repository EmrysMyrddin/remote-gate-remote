-- name: ListUsers :many
select * from "users";

-- name: ListUsersByRole :many
select * from "users" where role = $1;

-- name: GetUser :one
select * from "users" where id = $1;

-- name: GetUserByEmail :one
select * from "users" where email = $1;

-- name: CreateUser :one
insert into "users" (email, full_name, apartment, pwd_salt, pwd_hash, pwd_iterations, pwd_parallelism, pwd_memory, pwd_version, "role", registration_state) 
values (
  $1, $2, $3, $4, $5, $6, $7, $8, $9,
  (select (case when count(id) = 0 then 'admin' else 'user' end) role from "users"),
  (select (case when count(id) = 0 then 'accepted' else 'new' end) registration_state from "users")
) 
returning *;

-- name: EmailVerified :exec
update "users" set email_verified = true where id = $1;

-- name: RegistrationPending :one
update "users" set registration_state = 'pending' where id = $1 returning *;

-- name: RegistrationAccepted :one
update "users" set registration_state = 'accepted' where id = $1 returning *;;

-- name: RegistrationRejected :one
update "users" set registration_state = 'rejected' where id = $1 returning *;

-- name: RegistrationSuspended :one
update "users" set registration_state = 'suspended' where id = $1 returning *;

-- name: RenewRegistration :one
update "users" set registration_state = 'accepted', last_registration = now() where id = $1 returning *;

-- name: UpdatePassword :exec
update "users" set pwd_salt = $2, pwd_hash = $3, pwd_iterations = $4, pwd_parallelism = $5, pwd_memory = $6, pwd_version = $7 where id = $1;

-- name: UpdateUserInfo :one
update "users" set "role" = $2, full_name = $3, apartment = $4, email = $5 where id = $1 returning *;

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

-- name: DeleteOldLogs :execrows
delete from "logs" where created_at < now() - interval '1 year';

-- name: GetRegistrationCode :one
select code from "registration_code";

-- name: SetRegistrationCode :exec
insert into "registration_code" (id, code) values (1, $1) on conflict (id) do update set code = $1;

-- name: CycleRegistrationCode :execrows
update "registration_code" set code = lpad(floor(random() * 899999 + 100000)::text, 6, "0") where updated_at < now() - interval '2 month';

-- name: ListUsersRegisteredSince :many
select * from "users" where last_registration + sqlc.arg(since)::text::interval >= current_date  and last_registration + sqlc.arg(since)::text::interval < current_date + interval '1 day' ;

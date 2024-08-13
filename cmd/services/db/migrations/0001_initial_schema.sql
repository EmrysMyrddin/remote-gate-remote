-- +goose Up

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = current_timestamp;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

create table if not exists "users" (
  id uuid primary key default gen_random_uuid(),
  email varchar(255) not null,
  full_name varchar(255) not null,
  apartment varchar(5) not null,
  pwd_salt varchar(255) not null,
  pwd_hash varchar(255) not null,
  pwd_iterations int not null,
  pwd_parallelism smallint not null,
  pwd_memory int not null,
  pwd_version int not null,
  role varchar(255) not null default 'user',
  email_verified boolean not null default false,
  created_at timestamp not null default current_timestamp,
  updated_at timestamp not null default current_timestamp,
  registration_state varchar(100) not null default 'new'
);
create unique index if not exists users_email_key on "users" (email);


create table if not exists "used_token" (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references "users" (id),
  token text not null,
  created_at timestamp not null default current_timestamp
);
create unique index if not exists used_tokens_token_key on "used_token" (token);

CREATE OR REPLACE TRIGGER trigger_updated_at_users
  BEFORE UPDATE ON "users"
  FOR EACH ROW
  EXECUTE PROCEDURE trigger_set_timestamp ();

create table if not exists "logs" (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references "users" (id),
  created_at timestamp not null default current_timestamp
);

create table if not exists "registration_code" (
  id smallint primary key default 1,
  code varchar(255) not null,
  updated_at timestamp not null default current_timestamp
);

CREATE OR REPLACE TRIGGER trigger_updated_at_registration_code
  BEFORE UPDATE ON "registration_code"
  FOR EACH ROW
  EXECUTE PROCEDURE trigger_set_timestamp ();

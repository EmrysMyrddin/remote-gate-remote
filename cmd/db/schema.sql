-- Description: This file contains the schema for the postgres database.

CREATE OR REPLACE FUNCTION trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = current_timestamp;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

create table "users" (
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
  created_at timestamp not null default current_timestamp,
  updated_at timestamp not null default current_timestamp
);

create unique index users_email_key on "users" (email);

CREATE TRIGGER trigger_updated_at_users
  BEFORE UPDATE ON "users"
  FOR EACH ROW
  EXECUTE PROCEDURE trigger_set_timestamp ();

create table "logs" (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null references "users" (id),
  created_at timestamp not null default current_timestamp
);
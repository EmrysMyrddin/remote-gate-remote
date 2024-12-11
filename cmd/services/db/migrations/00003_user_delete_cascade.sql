-- +goose Up
-- +goose StatementBegin
alter table "logs" drop constraint if exists logs_user_id_fkey;
alter table "logs" add constraint logs_user_id_fkey foreign key (user_id) references "users"(id) on delete cascade;

alter table "used_token" drop constraint if exists used_token_user_id_fkey;
alter table "used_token" add constraint used_token_user_id_fkey foreign key (user_id) references "users"(id) on delete cascade;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
alter table "logs" drop constraint if exists logs_user_id_fkey;
alter table "logs" add logs_user_id_fkey foreign key (user_id) references "users"(id);

alter table "used_token" drop constraint if exists used_token_user_id_fkey;
alter table "used_token" add constraint used_token_user_id_fkey foreign key (user_id) references "users"(id);
-- +goose StatementEnd

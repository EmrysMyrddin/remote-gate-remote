-- +goose Up
-- +goose StatementBegin
alter table "users" add column last_registration timestamp not null default current_timestamp;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
alter table "users" drop column last_registration;
-- +goose StatementEnd

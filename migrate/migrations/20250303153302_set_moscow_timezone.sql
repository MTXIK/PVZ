-- +goose Up
-- +goose StatementBegin
ALTER DATABASE postgres SET timezone = 'Europe/Moscow';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER DATABASE postgres SET timezone = 'UTC';
-- +goose StatementEnd
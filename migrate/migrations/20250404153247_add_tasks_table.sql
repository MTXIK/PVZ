-- +goose Up
-- +goose StatementBegin
CREATE TYPE task_status AS ENUM (
    'CREATED',
    'PROCESSING',
    'COMPLETED',
    'FAILED',
    'NO_ATTEMPTS_LEFT'
);

CREATE TABLE IF NOT EXISTS audit_tasks (
    id SERIAL PRIMARY KEY,
    log_id INT NOT NULL REFERENCES audit_logs(id),
    status task_status NOT NULL DEFAULT 'CREATED',
    attempts_left INT NOT NULL DEFAULT 3,
    next_attempt_after TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT
);

CREATE INDEX idx_audit_tasks_log_id ON audit_tasks(log_id);
CREATE INDEX idx_audit_tasks_status ON audit_tasks(status);
CREATE INDEX idx_audit_tasks_next_attempt ON audit_tasks(next_attempt_after);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS audit_tasks;
DROP TYPE IF EXISTS task_status;
DROP INDEX IF EXISTS idx_audit_tasks_log_id;
DROP INDEX IF EXISTS idx_audit_tasks_status;
DROP INDEX IF EXISTS idx_audit_tasks_next_attempt;
-- +goose StatementEnd

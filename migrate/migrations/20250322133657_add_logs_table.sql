-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS audit_logs (
    id SERIAL PRIMARY KEY,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    type VARCHAR(50) NOT NULL,
    path VARCHAR(255),
    method VARCHAR(10),
    request_id VARCHAR(100),
    ip VARCHAR(45),
    body TEXT,
    status_code INT,
    order_id BIGINT,
    old_status VARCHAR(50),
    new_status VARCHAR(50),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_order_id ON audit_logs(order_id);
CREATE INDEX idx_audit_logs_type ON audit_logs(type);
CREATE INDEX idx_audit_logs_timestamp ON audit_logs(timestamp);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS audit_logs;
DROP INDEX IF EXISTS idx_audit_logs_order_id;
DROP INDEX IF EXISTS idx_audit_logs_type;
DROP INDEX IF EXISTS idx_audit_logs_timestamp;
-- +goose StatementEnd
-- +goose Up
-- +goose StatementBegin
INSERT INTO audit_tasks (log_id)
SELECT id
FROM audit_logs
WHERE NOT EXISTS (SELECT 1 FROM audit_tasks WHERE audit_tasks.log_id = audit_logs.id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM audit_tasks 
WHERE log_id IN (
    SELECT id FROM audit_logs
);
-- +goose StatementEnd

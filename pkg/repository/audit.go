package repository

import (
	"context"
	"fmt"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"gitlab.ozon.dev/gojhw1/pkg/db"
	"gitlab.ozon.dev/gojhw1/pkg/model"
)

var (
	// ErrAuditLogNotFound определяет ошибку, которая возникает, когда аудит-лог не найден в репозитории
	ErrAuditLogNotFound = fmt.Errorf("аудит-лог не найден")
)

// PostgresAuditRepository реализует хранилище аудит-логов в PostgreSQL
type PostgresAuditRepository struct {
	pool *db.Pool
}

// NewPostgresAuditRepository создает новый экземпляр PostgresAuditRepository
func NewPostgresAuditRepository(pool *db.Pool) *PostgresAuditRepository {
	return &PostgresAuditRepository{
		pool: pool,
	}
}

// CreateLogsWithTasks создает аудит-логи и связанные с ними задачи в рамках одной транзакции
func (r *PostgresAuditRepository) CreateLogsWithTasks(ctx context.Context, logs []model.AuditLog) error {
	if len(logs) == 0 {
		return nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	logIDs, err := r.createLogs(ctx, tx, logs)
	if err != nil {
		return err
	}

	err = r.createTasks(ctx, tx, logIDs)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// FetchTasksIDs получает и блокирует задачи для обработки, возвращая массив ID связанных логов
func (r *PostgresAuditRepository) FetchTasksIDs(ctx context.Context, limit int) ([]model.AuditIDs, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	query := `
        UPDATE audit_tasks
        SET status = 'PROCESSING'::task_status, updated_at = NOW()
        WHERE id IN (
            SELECT id FROM audit_tasks
            WHERE (status = 'CREATED'::task_status OR 
                  (status = 'FAILED'::task_status AND attempts_left > 0 AND 
                   (next_attempt_after IS NULL OR next_attempt_after <= NOW())))
            ORDER BY created_at
            FOR UPDATE SKIP LOCKED
            LIMIT $1
        )
        RETURNING id, log_id
    `

	rows, err := tx.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("получение задач: %w", err)
	}
	defer rows.Close()

	auditIDs := make([]model.AuditIDs, 0)

	for rows.Next() {
		var task model.AuditIDs
		if err := rows.Scan(&task.TaskID, &task.LogID); err != nil {
			return nil, fmt.Errorf("сканирование auditID: %w", err)
		}
		auditIDs = append(auditIDs, task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка при чтении строк: %w", err)
	}

	return auditIDs, tx.Commit(ctx)
}

// GetAuditLog получает аудит-лог по его ID
func (r *PostgresAuditRepository) GetAuditLog(ctx context.Context, id uint64) (model.AuditLog, error) {
	var DBlog model.AuditLogDB

	err := pgxscan.Get(ctx, r.pool, &DBlog, `
		SELECT 
            id, timestamp, type, path, method, request_id, ip, body, 
            status_code, order_id, old_status, new_status
        FROM audit_logs
        WHERE id = $1
	`, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return model.AuditLog{}, fmt.Errorf("ошибка получения лога: %w", ErrAuditLogNotFound)
		}
		return model.AuditLog{}, fmt.Errorf("ошибка получения лога: %w", err)
	}

	log, err := toAuditLog(DBlog)
	if err != nil {
		return model.AuditLog{}, fmt.Errorf("ошибка преобразования лога: %w", err)
	}

	return log, nil
}

// MarkTaskFailed помечает задачу как неуспешную и уменьшает счетчик попыток
func (r *PostgresAuditRepository) MarkTaskFailed(ctx context.Context, taskID uint64, taskErr error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var exists bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_tasks WHERE id = $1) FOR UPDATE`, taskID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("ошибка блокировки задачи: %w", err)
	}
	if !exists {
		return fmt.Errorf("задача с ID %d не найдена", taskID)
	}

	sql := `
		UPDATE audit_tasks
		SET
			status = CASE WHEN attempts_left > 1 THEN 'FAILED'::task_status ELSE 'NO_ATTEMPTS_LEFT'::task_status END,
			attempts_left = attempts_left - 1,
			next_attempt_after = CASE WHEN attempts_left > 1 THEN NOW() + INTERVAL '2 seconds' ELSE NULL END,
			updated_at = NOW(),
			error_message = $2
		WHERE id = $1
	`

	_, err = tx.Exec(ctx, sql, taskID, taskErr.Error())
	if err != nil {
		return fmt.Errorf("ошибка обновления статуса задачи %d: %w", taskID, err)
	}

	return tx.Commit(ctx)
}

// MarkTaskCompleted помечает задачу как успешно выполненную
func (r *PostgresAuditRepository) MarkTaskCompleted(ctx context.Context, taskID uint64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var exists bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_tasks WHERE id = $1) FOR UPDATE`, taskID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("ошибка блокировки задачи: %w", err)
	}
	if !exists {
		return fmt.Errorf("задача с ID %d не найдена", taskID)
	}

	_, err = tx.Exec(ctx, `
		UPDATE audit_tasks
		SET
			status = 'COMPLETED'::task_status,
			updated_at = NOW(),
			completed_at = NOW()
		WHERE id = $1
	`, taskID)
	if err != nil {
		return fmt.Errorf("ошибка обновления статуса задачи %d: %w", taskID, err)
	}

	return tx.Commit(ctx)
}

// createLogs создает записи аудит-логов в базе данных в рамках транзакции
func (r *PostgresAuditRepository) createLogs(ctx context.Context, tx pgx.Tx, batch []model.AuditLog) ([]uint64, error) {
	if len(batch) == 0 {
		return nil, nil
	}

	dbLogs, err := fromAuditLogs(batch)
	if err != nil {
		return nil, err
	}

	sql := `
        INSERT INTO audit_logs
        (timestamp, type, path, method, request_id, ip, body, status_code, order_id, old_status, new_status)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id
    `

	pgxBatch := &pgx.Batch{}

	for _, dbLog := range dbLogs {
		pgxBatch.Queue(sql,
			dbLog.Timestamp,
			dbLog.Type,
			dbLog.Path,
			dbLog.Method,
			dbLog.RequestID,
			dbLog.IP,
			dbLog.Body,
			dbLog.StatusCode,
			dbLog.OrderID,
			dbLog.OldStatus,
			dbLog.NewStatus,
		)
	}

	br := tx.SendBatch(ctx, pgxBatch)
	defer br.Close()

	logIDs := make([]uint64, 0, len(dbLogs))

	for range pgxBatch.Len() {
		row := br.QueryRow()
		var id uint64
		if err := row.Scan(&id); err != nil {
			return nil, err
		}
		logIDs = append(logIDs, id)
	}

	return logIDs, nil
}

// createTasks создает задачи для обработки аудит-логов в базе данных
func (r *PostgresAuditRepository) createTasks(ctx context.Context, tx pgx.Tx, logIDs []uint64) error {
	if len(logIDs) == 0 {
		return nil
	}

	sql := `
		INSERT INTO audit_tasks
		(log_id)
		VALUES ($1)
	`

	pgxBatch := &pgx.Batch{}

	for _, logID := range logIDs {
		pgxBatch.Queue(sql, logID)
	}

	br := tx.SendBatch(ctx, pgxBatch)
	defer br.Close()

	for range pgxBatch.Len() {
		_, err := br.Exec()
		if err != nil {
			return err
		}
	}

	return nil
}

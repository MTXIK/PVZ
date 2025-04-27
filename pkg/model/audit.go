package model

import (
	"database/sql"
	"time"
)

// AuditLogType определяет тип аудит-лога
type AuditLogType string

const (
	// AuditLogTypeRequest представляет тип аудит-лога для запросов
	AuditLogTypeRequest AuditLogType = "REQUEST"
	// AuditLogTypeResponse представляет тип аудит-лога для ответов
	AuditLogTypeResponse AuditLogType = "RESPONSE"
	// AuditLogTypeOrderStatus представляет тип аудит-лога для изменений статуса заказа
	AuditLogTypeOrderStatus AuditLogType = "ORDER_STATUS"
)

// AuditLog представляет структуру аудит-лога для бизнес-логики
type AuditLog struct {
	RequestID  string       `json:"request_id,omitempty"`
	Type       AuditLogType `json:"type"`
	Timestamp  time.Time    `json:"timestamp"`
	StatusCode int          `json:"status_code,omitempty"`
	Path       string       `json:"path,omitempty"`
	Method     string       `json:"method,omitempty"`
	IP         string       `json:"ip,omitempty"`
	Body       any          `json:"body,omitempty"`

	// Поля для AuditLogTypeOrderStatus
	OrderID   int64  `json:"order_id,omitempty"`
	OldStatus string `json:"old_status,omitempty"`
	NewStatus string `json:"new_status,omitempty"`
}

// AuditLogDB представляет структуру аудит-лога для работы с базой данных
type AuditLogDB struct {
	ID         uint64         `db:"id"`
	Timestamp  time.Time      `db:"timestamp"`
	Type       string         `db:"type"`
	Path       sql.NullString `db:"path"`
	Method     sql.NullString `db:"method"`
	RequestID  sql.NullString `db:"request_id"`
	IP         sql.NullString `db:"ip"`
	Body       sql.NullString `db:"body"`
	StatusCode sql.NullInt32  `db:"status_code"`
	OrderID    sql.NullInt64  `db:"order_id"`
	OldStatus  sql.NullString `db:"old_status"`
	NewStatus  sql.NullString `db:"new_status"`
}

// AuditIDs представляет структуру для хранения идентификаторов задачи и лога
type AuditIDs struct {
	TaskID uint64
	LogID  uint64
}

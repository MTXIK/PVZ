package repository

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.ozon.dev/gojhw1/pkg/model"
)

// getPackageTypeStr преобразует указатель на PackageType в строку
func getPackageTypeStr(pt *model.PackageType) string {
	if pt == nil {
		return ""
	}

	return string(*pt)
}

// getWrapperTypeStr преобразует указатель на WrapperType в строку
func getWrapperTypeStr(wt *model.WrapperType) string {
	if wt == nil {
		return ""
	}

	return string(*wt)
}

// nullableString возвращает sql.NullString из строки
func nullableString(s string) sql.NullString {
	return sql.NullString{
		String: s,
		Valid:  s != "",
	}
}

// nullableInt32 возвращает sql.NullInt32 из int
func nullableInt32(i int) sql.NullInt32 {
	return sql.NullInt32{
		Int32: int32(i),
		Valid: i != 0,
	}
}

// nullableInt64 возвращает sql.NullInt64 из int64
func nullableInt64(i int64) sql.NullInt64 {
	return sql.NullInt64{
		Int64: i,
		Valid: i != 0,
	}
}

// FromAuditLog преобразует обычный AuditLog в формат для БД
func fromAuditLog(log model.AuditLog) (model.AuditLogDB, error) {
	dbLog := model.AuditLogDB{
		Timestamp:  log.Timestamp,
		Type:       string(log.Type),
		Path:       nullableString(log.Path),
		Method:     nullableString(log.Method),
		RequestID:  nullableString(log.RequestID),
		IP:         nullableString(log.IP),
		StatusCode: nullableInt32(log.StatusCode),
		OrderID:    nullableInt64(log.OrderID),
		OldStatus:  nullableString(log.OldStatus),
		NewStatus:  nullableString(log.NewStatus),
	}

	// Преобразуем body в JSON, если оно не nil
	if log.Body != nil {
		bodyJSON, err := json.Marshal(log.Body)
		if err != nil {
			return dbLog, err
		}

		if len(bodyJSON) > 0 {
			dbLog.Body = nullableString(string(bodyJSON))
		}
	}

	return dbLog, nil
}

// toAuditLog преобразует формат AuditLogDB в обычный AuditLog
func toAuditLog(dbLog model.AuditLogDB) (model.AuditLog, error) {
	result := model.AuditLog{
		Type:      model.AuditLogType(dbLog.Type),
		Timestamp: dbLog.Timestamp,
	}

	if dbLog.Path.Valid {
		result.Path = dbLog.Path.String
	}

	if dbLog.Method.Valid {
		result.Method = dbLog.Method.String
	}

	if dbLog.RequestID.Valid {
		result.RequestID = dbLog.RequestID.String
	}

	if dbLog.IP.Valid {
		result.IP = dbLog.IP.String
	}

	if dbLog.StatusCode.Valid {
		result.StatusCode = int(dbLog.StatusCode.Int32)
	}

	if dbLog.OrderID.Valid {
		result.OrderID = dbLog.OrderID.Int64
	}

	if dbLog.OldStatus.Valid {
		result.OldStatus = dbLog.OldStatus.String
	}

	if dbLog.NewStatus.Valid {
		result.NewStatus = dbLog.NewStatus.String
	}

	// Обработка поля Body, если оно существует
	if dbLog.Body.Valid && dbLog.Body.String != "" {
		// Пытаемся распарсить JSON
		var bodyData any
		if err := json.Unmarshal([]byte(dbLog.Body.String), &bodyData); err != nil {
			// Если не получилось распарсить как JSON, используем строку как есть
			result.Body = dbLog.Body.String
		} else {
			// Иначе используем распарсенные данные
			result.Body = bodyData
		}
	}

	return result, nil
}

// toAuditLogs преобразует слайс model.AuditLogDB в слайс AuditLog
func toAuditLogs(dbLogs []model.AuditLogDB) ([]model.AuditLog, error) {
	logs := make([]model.AuditLog, len(dbLogs))

	for i, dbLog := range dbLogs {
		log, err := toAuditLog(dbLog)
		if err != nil {
			return nil, err
		}
		logs[i] = log
	}

	return logs, nil
}

// FromAuditLogs преобразует слайс AuditLog в слайс model.AuditLogDB
func fromAuditLogs(logs []model.AuditLog) ([]model.AuditLogDB, error) {
	dbLogs := make([]model.AuditLogDB, len(logs))

	for i, log := range logs {
		dbLog, err := fromAuditLog(log)
		if err != nil {
			return nil, err
		}
		dbLogs[i] = dbLog
	}

	return dbLogs, nil
}

// generateSalt генерирует случайный соль заданного размера
func generateSalt(size int) ([]byte, error) {
	salt := make([]byte, size)
	_, err := rand.Read(salt)
	if err != nil {
		return nil, err
	}

	return salt, nil
}

// hashPasswordSHA256 хеширует пароль с использованием SHA-256 и соли
func hashPasswordSHA256(password string) (string, error) {
	salt, err := generateSalt(16)
	if err != nil {
		return "", err
	}

	hash := sha256.New()
	hash.Write(salt)
	hash.Write([]byte(password))
	hashedPassword := hash.Sum(nil)

	return fmt.Sprintf("%s:%s",
		base64.StdEncoding.EncodeToString(salt),
		base64.StdEncoding.EncodeToString(hashedPassword)), nil
}

// checkPassword проверяет, соответствует ли введенный пароль хешу
func checkPassword(storedHash, password string) bool {
	parts := strings.Split(storedHash, ":")
	if len(parts) != 2 {
		return false
	}

	salt, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}

	storedPasswordHash, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	hash := sha256.New()
	hash.Write(salt)
	hash.Write([]byte(password))
	hashedPassword := hash.Sum(nil)

	return subtle.ConstantTimeCompare(hashedPassword, storedPasswordHash) == 1
}

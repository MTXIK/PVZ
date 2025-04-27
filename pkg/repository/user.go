package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"gitlab.ozon.dev/gojhw1/pkg/db"
	"gitlab.ozon.dev/gojhw1/pkg/logger"
	"gitlab.ozon.dev/gojhw1/pkg/model"
)

var (
	// ErrUserNotFound определяет ошибку, которая возникает, когда пользователь не найден в репозитории
	ErrUserNotFound = errors.New("пользователь не найден")
	// ErrUserAlreadyExists определяет ошибку, которая возникает, когда пользователь с таким именем уже существует в репозитории
	ErrUserAlreadyExists = errors.New("пользователь с таким именем уже существует")
)

// PostgresUserRepository реализация репозитория для работы с пользователями в PostgreSQL
type PostgresUserRepository struct {
	pool *db.Pool
}

// NewPostgresUserRepository создает новый репозиторий пользователей
func NewPostgresUserRepository(pool *db.Pool) *PostgresUserRepository {
	return &PostgresUserRepository{
		pool: pool,
	}
}

// Create создает нового пользователя с хешированным паролем
func (r *PostgresUserRepository) Create(ctx context.Context, user model.User, plainPassword string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTransactionStartError, err)
	}
	defer tx.Rollback(ctx)

	// Проверим, существует ли пользователь с таким именем
	var exists bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE username = $1 FOR UPDATE)", user.Username).Scan(&exists)
	if err != nil {
		return fmt.Errorf("ошибка проверки существования пользователя: %w", err)
	}
	if exists {
		return ErrUserAlreadyExists
	}

	// Хешируем пароль
	passwordHash, err := hashPasswordSHA256(plainPassword)
	if err != nil {
		return fmt.Errorf("ошибка хеширования пароля: %w", err)
	}

	now := time.Now()

	_, err = tx.Exec(ctx, `
        INSERT INTO users (username, password_hash, role, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5)`,
		user.Username,
		passwordHash,
		user.Role,
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("ошибка создания пользователя: %w", err)
	}

	return tx.Commit(ctx)
}

// Update обновляет информацию о пользователе
func (r *PostgresUserRepository) Update(ctx context.Context, user model.User) error {
	now := time.Now()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTransactionStartError, err)
	}
	defer tx.Rollback(ctx)

	var existingUser model.User
	err = pgxscan.Get(ctx, tx, &existingUser,
		"SELECT id FROM users WHERE id = $1 FOR UPDATE", user.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf("ошибка получения пользователя для обновления: %w", err)
	}

	commandTag, err := tx.Exec(ctx, `
        UPDATE users
        SET username = $2, role = $3, updated_at = $4
        WHERE id = $1`,
		user.ID,
		user.Username,
		user.Role,
		now,
	)
	if err != nil {
		return fmt.Errorf("ошибка обновления пользователя: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return tx.Commit(ctx)
}

// UpdatePassword обновляет пароль пользователя
func (r *PostgresUserRepository) UpdatePassword(ctx context.Context, userID int64, newPassword string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTransactionStartError, err)
	}
	defer tx.Rollback(ctx)

	var existingUser model.User
	err = pgxscan.Get(ctx, tx, &existingUser,
		"SELECT id FROM users WHERE id = $1 FOR UPDATE", userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf("ошибка получения пользователя для обновления пароля: %w", err)
	}

	// Хешируем новый пароль
	passwordHash, err := hashPasswordSHA256(newPassword)
	if err != nil {
		return fmt.Errorf("ошибка хеширования пароля: %w", err)
	}

	now := time.Now()

	commandTag, err := tx.Exec(ctx, `
        UPDATE users
        SET password_hash = $2, updated_at = $3
        WHERE id = $1`,
		userID,
		passwordHash,
		now,
	)
	if err != nil {
		return fmt.Errorf("ошибка обновления пароля: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return tx.Commit(ctx)
}

// Delete удаляет пользователя по ID
func (r *PostgresUserRepository) Delete(ctx context.Context, id int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTransactionStartError, err)
	}
	defer tx.Rollback(ctx)

	var existingUser model.User
	err = pgxscan.Get(ctx, tx, &existingUser,
		"SELECT id FROM users WHERE id = $1 FOR UPDATE", id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf("ошибка получения пользователя для удаления: %w", err)
	}

	commandTag, err := tx.Exec(ctx, "DELETE FROM users WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("ошибка удаления пользователя: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return tx.Commit(ctx)
}

// GetByID получает пользователя по ID
func (r *PostgresUserRepository) GetByID(ctx context.Context, id int64) (model.User, error) {
	var user model.User
	err := pgxscan.Get(ctx, r.pool, &user,
		"SELECT id, username, password_hash, role, created_at, updated_at FROM users WHERE id = $1",
		id,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, ErrUserNotFound
		}
		return model.User{}, fmt.Errorf("ошибка получения пользователя: %w", err)
	}

	return user, nil
}

// GetByUsername получает пользователя по имени пользователя
func (r *PostgresUserRepository) GetByUsername(ctx context.Context, username string) (model.User, error) {
	var user model.User
	err := pgxscan.Get(ctx, r.pool, &user,
		"SELECT id, username, password_hash, role, created_at, updated_at FROM users WHERE username = $1",
		username,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, ErrUserNotFound
		}
		return model.User{}, fmt.Errorf("ошибка получения пользователя: %w", err)
	}

	return user, nil
}

// List возвращает список всех пользователей
func (r *PostgresUserRepository) List(ctx context.Context, searchTerm string) ([]model.User, error) {
	var users []model.User
	query := "SELECT id, username, role, created_at, updated_at FROM users WHERE 1=1"
	var args []any

	if searchTerm != "" {
		query += " AND (username ILIKE $1 OR CAST(id AS TEXT) LIKE $1)"
		args = append(args, "%"+searchTerm+"%")
	}

	query += " ORDER BY username"

	var err error
	if len(args) > 0 {
		err = pgxscan.Select(ctx, r.pool, &users, query, args...)
	} else {
		err = pgxscan.Select(ctx, r.pool, &users, query)
	}

	if err != nil {
		return nil, fmt.Errorf("ошибка получения списка пользователей: %w", err)
	}

	return users, nil
}

// CheckPassword проверяет правильность пароля для указанного пользователя
func (r *PostgresUserRepository) CheckPassword(ctx context.Context, username, password string) bool {

	var storedPasswordHash string
	err := r.pool.QueryRow(ctx, "SELECT password_hash FROM users WHERE username = $1", username).Scan(&storedPasswordHash)
	if err != nil {
		logger.Errorf("Ошибка при получении пользователя %s: %v", username, err)
		return false
	}

	return checkPassword(storedPasswordHash, password)
}

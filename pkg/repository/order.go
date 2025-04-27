package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/jackc/pgx/v5"
	"gitlab.ozon.dev/gojhw1/pkg/db"
	"gitlab.ozon.dev/gojhw1/pkg/model"
)

var (
	// ErrOrderAlreadyExists - заказ уже существует
	ErrOrderAlreadyExists = errors.New("заказ уже существует")
	// ErrOrderNotFound - заказ не существует
	ErrOrderNotFound = errors.New("заказ не существует")
	// ErrOrdersNotFound - заказы не найдены
	ErrOrdersNotFound = errors.New("заказы не найдены")
	// ErrInvalidOrderID - недопустимый ID заказа
	ErrInvalidOrderID = errors.New("недопустимый ID заказа")
	// ErrInvalidCustomerID - недопустимый ID клиента
	ErrInvalidCustomerID = errors.New("недопустимый ID клиента")
	// ErrTransactionStartError - ошибка начала транзакции
	ErrTransactionStartError = errors.New("ошибка начала транзакции")
)

type PostgresOrderRepository struct {
	pool *db.Pool
}

// NewPostgresRepository создает новый репозиторий PostgreSQL
func NewPostgresOrderRepository(pool *db.Pool) *PostgresOrderRepository {
	return &PostgresOrderRepository{
		pool: pool,
	}
}

// Create создает новый заказ в базе данных
func (r *PostgresOrderRepository) Create(ctx context.Context, order model.Order) error {
	if order.ID <= 0 {
		return fmt.Errorf("%w: %d", ErrInvalidOrderID, order.ID)
	}

	if order.CustomerID <= 0 {
		return fmt.Errorf("%w: %d", ErrInvalidCustomerID, order.CustomerID)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTransactionStartError, err)
	}
	defer tx.Rollback(ctx)

	var exists bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM orders WHERE id = $1 FOR UPDATE)", order.ID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("ошибка проверки существования заказа: %w", err)
	}
	if exists {
		return fmt.Errorf("%w: %d", ErrOrderAlreadyExists, order.ID)
	}

	_, err = tx.Exec(ctx, `
        INSERT INTO orders 
        (id, customer_id, state_id, weight, cost, package_type_id, wrapper_type_id, deadline_at, updated_at, delivered_at, returned_at) 
        VALUES (
        $1, 
        $2, 
        (SELECT id FROM order_states WHERE name = $3), 
        $4, 
        $5, 
        (SELECT id FROM package_types WHERE name = $6), 
        (SELECT id FROM wrapper_types WHERE name = $7), 
        $8, 
        $9, 
        $10, 
        $11)`,
		order.ID,
		order.CustomerID,
		string(order.State),
		order.Weight,
		order.Cost,
		getPackageTypeStr(order.PackageType),
		getWrapperTypeStr(order.Wrapper),
		order.DeadlineAt,
		order.UpdatedAt,
		order.DeliveredAt,
		order.ReturnedAt,
	)
	if err != nil {
		return fmt.Errorf("ошибка добавления заказа: %w", err)
	}

	return tx.Commit(ctx)
}

// Update обновляет существующий заказ в базе данных
func (r *PostgresOrderRepository) Update(ctx context.Context, order model.Order) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTransactionStartError, err)
	}
	defer tx.Rollback(ctx)

	var exists bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM orders WHERE id = $1 FOR UPDATE)", order.ID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("ошибка блокировки заказа: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: %d", ErrOrderNotFound, order.ID)
	}

	commandTag, err := tx.Exec(ctx, `
        UPDATE orders SET 
        customer_id = $2, 
        state_id = (SELECT id FROM order_states WHERE name = $3), 
        weight = $4, 
        cost = $5, 
        package_type_id = (SELECT id FROM package_types WHERE name = $6), 
        wrapper_type_id = (SELECT id FROM wrapper_types WHERE name = $7), 
        deadline_at = $8, 
        updated_at = $9, 
        delivered_at = $10, 
        returned_at = $11
        WHERE id = $1`,
		order.ID,
		order.CustomerID,
		string(order.State),
		order.Weight,
		order.Cost,
		getPackageTypeStr(order.PackageType),
		getWrapperTypeStr(order.Wrapper),
		order.DeadlineAt,
		order.UpdatedAt,
		order.DeliveredAt,
		order.ReturnedAt)

	if err != nil {
		return fmt.Errorf("ошибка обновления заказа: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %d", ErrOrderNotFound, order.ID)
	}

	return tx.Commit(ctx)
}

// Delete удаляет заказ по ID
func (r *PostgresOrderRepository) Delete(ctx context.Context, id int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrTransactionStartError, err)
	}
	defer tx.Rollback(ctx)

	var exists bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM orders WHERE id = $1 FOR UPDATE)", id).Scan(&exists)
	if err != nil {
		return fmt.Errorf("ошибка блокировки заказа: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: %d", ErrOrderNotFound, id)
	}

	commandTag, err := tx.Exec(ctx, "DELETE FROM orders WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("ошибка удаления заказа: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %d", ErrOrderNotFound, id)
	}

	return tx.Commit(ctx)
}

// GetByID возвращает заказ по ID
func (r *PostgresOrderRepository) GetByID(ctx context.Context, id int64) (model.Order, error) {
	var order model.Order
	err := pgxscan.Get(ctx, r.pool, &order, `
        SELECT 
            o.id, 
			o.customer_id, 
            os.name AS state, 
            o.weight, 
			o.cost, 
            pt.name AS package_type, 
            wt.name AS wrapper, 
            o.deadline_at, 
			o.updated_at, 
			o.delivered_at, 
			o.returned_at
        FROM orders o
        JOIN order_states os ON o.state_id = os.id
        LEFT JOIN package_types pt ON o.package_type_id = pt.id
        LEFT JOIN wrapper_types wt ON o.wrapper_type_id = wt.id
        WHERE o.id = $1`, id)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Order{}, fmt.Errorf("%w: %d", ErrOrderNotFound, id)
		}
		return model.Order{}, fmt.Errorf("ошибка поиска заказа: %w", err)
	}

	return order, nil
}

// List возвращает список заказов с возможностью поиска
func (r *PostgresOrderRepository) List(ctx context.Context, searchTerm string) ([]model.Order, error) {
	var orders []model.Order
	query := `
        SELECT 
            o.id, o.customer_id, 
            os.name as state, 
            o.weight, o.cost, 
            pt.name as package_type, 
            wt.name as wrapper, 
            o.deadline_at, 
			o.updated_at, 
			o.delivered_at, 
			o.returned_at
        FROM orders o
        JOIN order_states os ON o.state_id = os.id
        LEFT JOIN package_types pt ON o.package_type_id = pt.id
        LEFT JOIN wrapper_types wt ON o.wrapper_type_id = wt.id
        WHERE 1=1`

	var args []interface{}
	if searchTerm != "" {
		query += " AND (CAST(o.id AS TEXT) LIKE $1 OR CAST(o.customer_id AS TEXT) LIKE $1)"
		args = append(args, "%"+searchTerm+"%")
	}

	query += " ORDER BY o.updated_at DESC"

	var err error
	if len(args) > 0 {
		err = pgxscan.Select(ctx, r.pool, &orders, query, args...)
	} else {
		err = pgxscan.Select(ctx, r.pool, &orders, query)
	}

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []model.Order{}, fmt.Errorf("%w", ErrOrdersNotFound)
		}
		return []model.Order{}, fmt.Errorf("ошибка поиска заказа: %w", err)
	}

	return orders, nil
}

// ListWithCursor выполняет выборку заказов с использованием курсорной пагинации по ID
func (r *PostgresOrderRepository) ListWithCursor(ctx context.Context, cursorID int64, limit int, customerID int64, filterPVZ bool, searchTerm string) ([]model.Order, error) {
	var orders []model.Order
	var queryArgs []any

	query := `
        SELECT 
            o.id, 
			o.customer_id, 
            os.name as state, 
            o.weight, 
			o.cost, 
            pt.name as package_type, 
            wt.name as wrapper, 
            o.deadline_at, 
            o.updated_at, 
            o.delivered_at, 
            o.returned_at
        FROM orders o
        JOIN order_states os ON o.state_id = os.id
        LEFT JOIN package_types pt ON o.package_type_id = pt.id
        LEFT JOIN wrapper_types wt ON o.wrapper_type_id = wt.id
        WHERE 1=1`

	// Добавляем поиск по тексту
	if searchTerm != "" {
		query += " AND (CAST(o.id AS TEXT) LIKE $1 OR CAST(o.customer_id AS TEXT) LIKE $1)"
		queryArgs = append(queryArgs, "%"+searchTerm+"%")
	}

	// Условия фильтрации по заказчику
	if customerID > 0 {
		paramNum := len(queryArgs) + 1
		query += fmt.Sprintf(" AND o.customer_id = $%d", paramNum)
		queryArgs = append(queryArgs, customerID)
	}

	// Условие фильтрации для заказов, доступных в ПВЗ
	if filterPVZ {
		paramNum := len(queryArgs) + 1
		query += fmt.Sprintf(" AND os.name = 'accepted' AND o.deadline_at > $%d", paramNum)
		queryArgs = append(queryArgs, time.Now())
	}

	// Условие для курсорной пагинации по ID
	if cursorID > 0 {
		paramNum := len(queryArgs) + 1
		query += fmt.Sprintf(" AND o.id < $%d", paramNum)
		queryArgs = append(queryArgs, cursorID)
	}

	// Сортировка по ID в порядке убывания
	query += " ORDER BY o.id DESC"

	// Добавление лимита
	paramNum := len(queryArgs) + 1
	query += fmt.Sprintf(" LIMIT $%d", paramNum)
	queryArgs = append(queryArgs, limit)

	err := pgxscan.Select(ctx, r.pool, &orders, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("ошибка при получении списка заказов: %w", err)
	}

	return orders, nil
}

// ListReturnsWithCursor выполняет выборку возвращенных заказов с использованием курсорной пагинации по ID
func (r *PostgresOrderRepository) ListReturnsWithCursor(ctx context.Context, cursorID int64, limit int, searchTerm string) ([]model.Order, error) {
	var orders []model.Order
	var queryArgs []any

	query := `
        SELECT 
            o.id, o.customer_id, 
            os.name as state, 
            o.weight, o.cost, 
            pt.name as package_type, 
            wt.name as wrapper, 
            o.deadline_at, 
            o.updated_at, 
            o.delivered_at, 
            o.returned_at
        FROM orders o
        JOIN order_states os ON o.state_id = os.id
        LEFT JOIN package_types pt ON o.package_type_id = pt.id
        LEFT JOIN wrapper_types wt ON o.wrapper_type_id = wt.id
        WHERE os.name = 'returned'`

	// Добавляем поиск по тексту
	if searchTerm != "" {
		query += " AND (CAST(o.id AS TEXT) LIKE $1 OR CAST(o.customer_id AS TEXT) LIKE $1)"
		queryArgs = append(queryArgs, "%"+searchTerm+"%")
	}

	// Условие для курсорной пагинации по ID
	if cursorID > 0 {
		paramNum := len(queryArgs) + 1
		query += fmt.Sprintf(" AND o.id < $%d", paramNum)
		queryArgs = append(queryArgs, cursorID)
	}

	// Сортировка по ID в порядке убывания
	query += " ORDER BY o.id DESC"

	// Добавление лимита
	paramNum := len(queryArgs) + 1
	query += fmt.Sprintf(" LIMIT $%d", paramNum)
	queryArgs = append(queryArgs, limit)

	err := pgxscan.Select(ctx, r.pool, &orders, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("ошибка при получении списка возвращенных заказов: %w", err)
	}

	return orders, nil
}

// ListActual возвращает список актуальных заказов
func (r *PostgresOrderRepository) ListActual(ctx context.Context) ([]model.Order, error) {
	var orders []model.Order
	err := pgxscan.Select(ctx, r.pool, &orders, `
		SELECT 
			o.id, 
			o.customer_id, 
			os.name AS state, 
			o.weight, 
			o.cost, 
			pt.name AS package_type, 
			wt.name AS wrapper, 
			o.deadline_at, 
			o.updated_at, 
			o.delivered_at, 
			o.returned_at
		FROM orders o
		JOIN order_states os ON o.state_id = os.id
		LEFT JOIN package_types pt ON o.package_type_id = pt.id
		LEFT JOIN wrapper_types wt ON o.wrapper_type_id = wt.id
		WHERE os.name IN ('accepted', 'delivered')
		ORDER BY o.updated_at DESC
	`)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []model.Order{}, fmt.Errorf("%w", ErrOrdersNotFound)
		}
		return []model.Order{}, fmt.Errorf("ошибка поиска заказа: %w", err)
	}

	return orders, nil
}

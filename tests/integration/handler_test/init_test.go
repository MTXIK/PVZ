package handler_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
	"gitlab.ozon.dev/gojhw1/migrate"
	"gitlab.ozon.dev/gojhw1/pkg/cache"
	"gitlab.ozon.dev/gojhw1/pkg/config"
	"gitlab.ozon.dev/gojhw1/pkg/db"
	"gitlab.ozon.dev/gojhw1/pkg/handler"
	"gitlab.ozon.dev/gojhw1/pkg/repository"
	"gitlab.ozon.dev/gojhw1/pkg/service"
	"gitlab.ozon.dev/gojhw1/pkg/utils"
)

// путь к тестовому конфигу
const testConfigPath = "test_config.json"

// setupTestDB создаёт подключение к тестовой БД и очищает её данные
func setupTestDB(t *testing.T) (*db.Pool, func()) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	// Загружаем конфигурацию из тестового файла
	cfg, err := config.Load(testConfigPath)
	if err != nil {
		// Если тестовый конфиг не найден, используем стандартный
		cfg, err = config.Load("../../../config.json")
		require.NoError(t, err, "Не удалось загрузить конфигурацию")
	}

	// Формируем URL для подключения к тестовой БД
	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.Database.User, cfg.Database.Password, cfg.Database.Host,
		cfg.Database.Port, cfg.Database.Name)

	// URL для подключения к системной БД postgres
	pgURL := fmt.Sprintf("postgres://%s:%s@%s:%s/postgres?sslmode=disable",
		cfg.Database.User, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port)

	// Проверяем существование и создаем тестовую БД, если её нет
	pgPool, err := db.New(ctx, pgURL)
	require.NoError(t, err)

	var exists bool
	err = pgPool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)",
		cfg.Database.Name).Scan(&exists)
	require.NoError(t, err)

	if !exists {
		_, err = pgPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", cfg.Database.Name))
		require.NoError(t, err)
	}

	pgPool.Close()

	// Выполняем миграции на тестовой БД
	err = migrate.Run(dbURL, "up")
	require.NoError(t, err)

	// Подключаемся к тестовой БД
	testPool, err := db.New(ctx, dbURL)
	require.NoError(t, err)

	// Очищаем все таблицы перед тестом, сохраняя схему
	tables := []string{"orders", "users", "audit_logs"}
	for _, table := range tables {
		_, err := testPool.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		require.NoError(t, err)
	}

	// Функция очистки после тестов
	cleanup := func() {
		// Опять очищаем таблицы после тестов - можно не делать, но для порядка
		for _, table := range tables {
			_, _ = testPool.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		}
		cancel()
		testPool.Close()
	}

	return testPool, cleanup
}

func setupTestRedis(t *testing.T) (*cache.RedisCache, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	cfg, err := config.Load(testConfigPath)
	if err != nil {
		// Если тестовый конфиг не найден, используем стандартный
		cfg, err = config.Load("../../../config.json")
		require.NoError(t, err, "Не удалось загрузить конфигурацию")
	}

	// Создаем тестовый Redis-сервер в памяти
	mr, err := miniredis.Run()
	require.NoError(t, err, "Не удалось запустить тестовый Redis-сервер")

	// Создаем кэш, подключенный к тестовому серверу
	redisCache, err := cache.NewRedisCache(ctx, mr.Addr(), cfg)
	require.NoError(t, err, "Ошибка подключения к тестовому Redis")

	// Функция очистки
	cleanup := func() {
		err := redisCache.ClearOrderCache(ctx)
		require.NoError(t, err, "Ошибка очистки Redis-кэша")
		cancel()
		mr.Close()
	}

	return redisCache, cleanup
}

// setupOrderTest создает тестовое окружение с реальной БД для тестирования хэндлеров заказов
func setupOrderTest(t *testing.T) (*fiber.App, *service.OrderService, *db.Pool, func()) {
	t.Helper()

	ctx, ctxCancel := context.WithCancel(context.Background())

	// Настраиваем тестовую БД
	pool, dbCleanup := setupTestDB(t)

	// Настраиваем тестовый Redis
	redisCache, redisCleanup := setupTestRedis(t)

	// Создаём репозитории
	orderRepo := repository.NewPostgresOrderRepository(pool)
	auditRepo := repository.NewPostgresAuditRepository(pool)

	logger := utils.NewAuditLogger(ctx, auditRepo, 2, 5, 500*time.Millisecond)

	// Создаём сервис
	orderService := service.NewOrderService(orderRepo, logger, redisCache)

	// Создаём хэндлер
	orderHandler := handler.NewOrderHandler(orderService)

	// Создаём приложение Fiber
	app := fiber.New()

	// Регистрируем маршруты
	app.Post("/orders", orderHandler.CreateOrder)
	app.Get("/orders/:id", orderHandler.GetOrder)
	app.Post("/orders/:id/return", orderHandler.ReturnToCourier)
	app.Post("/orders/process", orderHandler.ProcessCustomer)
	app.Get("/orders", orderHandler.ListOrders)
	app.Get("/returns", orderHandler.ListReturns)
	app.Get("/history", orderHandler.OrderHistory)
	app.Delete("/clear", orderHandler.ClearDatabase)

	// Функция очистки
	cleanup := func() {
		dbCleanup()
		redisCleanup()
		ctxCancel()
	}

	return app, orderService, pool, cleanup
}

// setupUserTest создает тестовое окружение с реальной БД для тестирования хэндлеров пользователей
func setupUserTest(t *testing.T) (*fiber.App, *repository.PostgresUserRepository, *db.Pool, func()) {
	t.Helper()

	// Настраиваем тестовую БД
	pool, dbCleanup := setupTestDB(t)

	// Создаём репозиторий
	userRepo := repository.NewPostgresUserRepository(pool)

	// Создаём хэндлер
	userHandler := handler.NewUserHandler(userRepo)

	// Создаём приложение Fiber
	app := fiber.New()

	// Регистрируем маршруты
	app.Post("/users/register", userHandler.CreateUser)
	app.Get("/users", userHandler.ListUsers)
	app.Get("/users/:id", userHandler.GetUser)
	app.Put("/users/:id", userHandler.UpdateUser)
	app.Delete("/users/:id", userHandler.DeleteUser)
	app.Put("/users/:id/password", userHandler.UpdatePassword)

	// Функция очистки
	cleanup := func() {
		dbCleanup()
	}

	return app, userRepo, pool, cleanup
}

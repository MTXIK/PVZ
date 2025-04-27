package suite_handler_test

import (
	"context"
	"fmt"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gitlab.ozon.dev/gojhw1/migrate"
	"gitlab.ozon.dev/gojhw1/pkg/cache"
	"gitlab.ozon.dev/gojhw1/pkg/config"
	"gitlab.ozon.dev/gojhw1/pkg/db"
)

// путь к тестовому конфигу
const testConfigPath = "suite_test_config.json"
const timeLayout = "2006-01-02T15:04:05"

// BaseSuite содержит общую для всех suite функциональность
type BaseSuite struct {
	suite.Suite
	ctx        context.Context
	cancel     context.CancelFunc
	pool       *db.Pool
	app        *fiber.App
	redisCache *cache.RedisCache
	miniRedis  *miniredis.Miniredis
}

// setupTestDB создаёт подключение к тестовой БД и очищает её данные
func (s *BaseSuite) setupTestDB() {
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Загружаем конфигурацию из тестового файла
	cfg, err := config.Load(testConfigPath)
	if err != nil {
		// Если тестовый конфиг не найден, используем стандартный
		cfg, err = config.Load("../../../config.json")
		require.NoError(s.T(), err, "Не удалось загрузить конфигурацию")
	}

	// Формируем URL для подключения к тестовой БД
	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.Database.User, cfg.Database.Password, cfg.Database.Host,
		cfg.Database.Port, cfg.Database.Name)

	// URL для подключения к системной БД postgres
	pgURL := fmt.Sprintf("postgres://%s:%s@%s:%s/postgres?sslmode=disable",
		cfg.Database.User, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port)

	// Проверяем существование и создаем тестовую БД, если её нет
	pgPool, err := db.New(s.ctx, pgURL)
	s.Require().NoError(err)

	var exists bool
	err = pgPool.QueryRow(s.ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)",
		cfg.Database.Name).Scan(&exists)
	s.Require().NoError(err)

	if !exists {
		_, err = pgPool.Exec(s.ctx, fmt.Sprintf("CREATE DATABASE %s", cfg.Database.Name))
		s.Require().NoError(err)
	}

	pgPool.Close()

	// Выполняем миграции на тестовой БД
	err = migrate.Run(dbURL, "up")
	s.Require().NoError(err)

	// Подключаемся к тестовой БД
	s.pool, err = db.New(s.ctx, dbURL)
	s.Require().NoError(err)
}

func (s *BaseSuite) setupTestRedis() {
	var err error

	// Загружаем конфигурацию из тестового файла
	cfg, err := config.Load(testConfigPath)
	if err != nil {
		// Если тестовый конфиг не найден, используем стандартный
		cfg, err = config.Load("../../../config.json")
		require.NoError(s.T(), err, "Не удалось загрузить конфигурацию")
	}

	// Создаем тестовый Redis-сервер в памяти
	s.miniRedis, err = miniredis.Run()
	require.NoError(s.T(), err, "Не удалось запустить тестовый Redis-сервер")

	// Создаем кэш, подключенный к тестовому серверу
	s.redisCache, err = cache.NewRedisCache(s.ctx, s.miniRedis.Addr(), cfg)
	require.NoError(s.T(), err, "Ошибка подключения к тестовому Redis")
}

// SetupTest очищает таблицы перед каждым тестом
func (s *BaseSuite) SetupTest() {
	// Очищаем все таблицы перед каждым тестом, сохраняя схему
	if s.pool != nil {
		tables := []string{"orders", "users", "audit_logs"}
		for _, table := range tables {
			_, err := s.pool.Exec(s.ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
			s.Require().NoError(err)
		}
	}

	if s.redisCache != nil {
		err := s.redisCache.ClearOrderCache(s.ctx)
		s.Require().NoError(err, "Ошибка очистки кэша Redis")
	}
}

// TearDownSuite закрывает подключение к БД
func (s *BaseSuite) TearDownSuite() {
	if s.pool != nil {
		s.pool.Close()
	}

	if s.miniRedis != nil {
		s.miniRedis.Close()
	}

	s.cancel()
}

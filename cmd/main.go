package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"gitlab.ozon.dev/gojhw1/migrate"
	"gitlab.ozon.dev/gojhw1/pkg/config"
	"gitlab.ozon.dev/gojhw1/pkg/db"
	"gitlab.ozon.dev/gojhw1/pkg/grpc"
	"gitlab.ozon.dev/gojhw1/pkg/kafka"
	"gitlab.ozon.dev/gojhw1/pkg/logger"
	"gitlab.ozon.dev/gojhw1/pkg/repository"
	"gitlab.ozon.dev/gojhw1/pkg/router"
	"gitlab.ozon.dev/gojhw1/pkg/service"
	"gitlab.ozon.dev/gojhw1/pkg/tracer"
	"gitlab.ozon.dev/gojhw1/pkg/utils"
)

const (
	workersCount       = 2
	batchSize          = 5
	batchTimeout       = 500 * time.Millisecond
	outboxWorkersCount = 3                      // Количество воркеров для обработки outbox
	outboxBatchSize    = 5                      // Количество задач за одну итерацию
	outboxPollingRate  = 500 * time.Millisecond // Интервал опроса БД
	configPath         = "config.json"
)

func main() {
	ctx, mainCtxCancel := context.WithCancel(context.Background())
	defer mainCtxCancel()

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Fatalf("ошибка загрузки конфигурации: %v", err)
	}

	err = logger.InitGlobalLogger(cfg)
	if err != nil {
		logger.Fatalf("ошибка инициализации логгера: %v", err)
	}

	tracerShutdown, err := tracer.InitTracerProvider(ctx, cfg)
	if err != nil {
		logger.Fatalf("ошибка инициализации трассировки: %v", err)
	}
	defer tracerShutdown()

	logger.Info("Приложение запускается...")
	logger.Infof("Используется конфигурация из файла: %s", configPath)

	pool, err := initInfrastructure(ctx, cfg)
	if err != nil {
		logger.Fatalf("ошибка инициализации инфраструктуры: %v", err)
	}
	defer pool.Close()

	repos := initRepositories(pool)
	logger.Debug("Репозитории инициализированы успешно")

	services, cleanup := initServices(ctx, cfg, repos)
	defer cleanup()
	logger.Debug("Сервисы инициализированы успешно")

	kafkaCleanup := initKafka(ctx, cfg, repos.auditRepo)
	defer kafkaCleanup()
	logger.Debug("Kafka инициализирована успешно")

	app := router.InitFiberApp(ctx, services.orderService, repos.userRepo, services.auditLogger)
	serverShutdown := startServer(ctx, app, cfg.Server.Port)
	defer serverShutdown()

	grpcServerShutdown := startGrpcServer(cfg, repos.userRepo, services.orderService)
	defer grpcServerShutdown()

	waitForShutdownSignal()

	logger.Info("Сервер, кэш и аудит-система успешно остановлены")
}

// Структура для хранения всех репозиториев
type repositories struct {
	orderRepo *repository.PostgresOrderRepository
	userRepo  *repository.PostgresUserRepository
	auditRepo *repository.PostgresAuditRepository
}

// Структура для хранения всех сервисов
type services struct {
	orderService *service.OrderService
	auditLogger  *utils.AuditLogger
}

// Инициализация инфраструктуры (миграции, подключение к БД)
func initInfrastructure(ctx context.Context, cfg *config.Config) (*db.Pool, error) {
	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.Database.User, cfg.Database.Password, cfg.Database.Host,
		cfg.Database.Port, cfg.Database.Name)

	logger.Infof("Запуск миграций для БД: %s:%s/%s", cfg.Database.Host, cfg.Database.Port, cfg.Database.Name)
	if err := migrate.Run(dbURL, "up"); err != nil {
		return nil, fmt.Errorf("ошибка при выполнении миграций: %v", err)
	}
	logger.Debug("Миграции успешно выполнены")

	logger.Debug("Подключение к базе данных...")
	pool, err := db.New(ctx, dbURL)
	if err != nil {
		return nil, fmt.Errorf("ошибка подключения к базе данных: %v", err)
	}
	logger.Info("Подключение к базе данных установлено")

	return pool, nil
}

// Инициализация репозиториев
func initRepositories(pool *db.Pool) repositories {
	return repositories{
		orderRepo: repository.NewPostgresOrderRepository(pool),
		userRepo:  repository.NewPostgresUserRepository(pool),
		auditRepo: repository.NewPostgresAuditRepository(pool),
	}
}

// Инициализация сервисов
func initServices(ctx context.Context, cfg *config.Config, repos repositories) (services, func()) {
	logger.Debugf("Инициализация кэша типа: %s", cfg.CacheType.Name)
	ordersCache, err := utils.SetCashType(ctx, cfg, repos.orderRepo)
	if err != nil {
		logger.Fatalf("ошибка инициализации кэша: %v", err)
	}

	logger.Debug("Инициализация пользователя по умолчанию")
	utils.InitDefaultUser(repos.userRepo)

	logger.Infof("Настройка логгера аудита с параметрами: workers=%d, batchSize=%d", workersCount, batchSize)
	auditLogger := utils.NewAuditLogger(ctx, repos.auditRepo, workersCount, batchSize, batchTimeout)

	orderService := service.NewOrderService(repos.orderRepo, auditLogger, ordersCache)

	cleanup := func() {
		logger.Debug("Остановка логгера аудита...")
		auditLogger.Shutdown()
		logger.Debug("Логгер аудита остановлен")
	}

	return services{
		orderService: orderService,
		auditLogger:  auditLogger,
	}, cleanup
}

// Инициализация Kafka
func initKafka(ctx context.Context, cfg *config.Config, auditRepo *repository.PostgresAuditRepository) func() {
	logger.Infof("Создание Kafka продюсера для темы: %s, брокеры: %v", cfg.Kafka.AuditTopic, cfg.Kafka.Brokers)
	outboxProducer, err := kafka.NewOutboxProducer(cfg.Kafka.Brokers, cfg.Kafka.AuditTopic)
	if err != nil {
		logger.Fatalf("ошибка создания продюсера: %v", err)
	}

	logger.Debugf("Настройка Outbox воркер-пула: workers=%d, batchSize=%d, pollingRate=%v",
		outboxWorkersCount, outboxBatchSize, outboxPollingRate)
	outboxWorkerPool := kafka.NewOutboxWorkerPool(
		auditRepo,
		outboxProducer,
		outboxWorkersCount,
		outboxBatchSize,
		outboxPollingRate,
	)
	outboxWorkerPool.Start(ctx)
	logger.Debug("Outbox воркер-пул запущен")

	kafkaConsumer, err := kafka.NewConsumer(cfg.Kafka.Brokers, cfg.Kafka.AuditGroupID, []string{cfg.Kafka.AuditTopic})
	if err != nil {
		logger.Fatalf("ошибка создания консьюмера: %v", err)
	}
	kafkaConsumer.Start(ctx)
	logger.Debug("Kafka консьюмер запущен")

	return func() {
		logger.Debug("Закрытие Kafka соединений...")
		outboxProducer.Close()
		outboxWorkerPool.Stop()
		kafkaConsumer.Stop()
		logger.Debug("Kafka соединения закрыты")
	}
}

// Запуск HTTP-сервера
func startServer(ctx context.Context, app *fiber.App, port string) func() {
	go func() {
		logger.Infof("HTTP-сервер запущен на порту :%s", port)
		if err := app.Listen(fmt.Sprintf(":%s", port)); err != nil {
			logger.Fatalf("ошибка запуска HTTP-сервера: %v", err)
		}
	}()

	return func() {
		logger.Debug("Остановка HTTP-сервера...")
		shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		if err := app.ShutdownWithContext(shutdownCtx); err != nil {
			logger.Fatalf("ошибка при завершении HTTP-сервера: %v", err)
		}
		logger.Debug("HTTP-сервер остановлен")
	}
}

func startGrpcServer(cfg *config.Config, userRepo *repository.PostgresUserRepository, orderService *service.OrderService) func() {
	logger.Infof("Настройка gRPC сервера на хосте: %s, порт: %s", cfg.Database.Host, cfg.GrpcServer.Port)
	server := grpc.NewServer(cfg.Database.Host, cfg.GrpcServer.Port, userRepo, orderService)

	go func() {
		if err := server.Start(); err != nil {
			logger.Fatalf("ошибка запуска gRPC сервера: %v", err)
		}
	}()

	logger.Infof("gRPC сервер запущен на порту %s", cfg.GrpcServer.Port)

	return func() {
		logger.Debug("Остановка gRPC сервера...")
		server.Stop()
		logger.Debug("gRPC сервер остановлен")
	}
}

// Ожидание сигнала завершения
func waitForShutdownSignal() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	sig := <-sigCh
	logger.Infof("Получен сигнал завершения %v, выключаем сервер...", sig)
}

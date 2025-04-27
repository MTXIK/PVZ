package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"gitlab.ozon.dev/gojhw1/pkg/config"
	"gitlab.ozon.dev/gojhw1/pkg/logger"
	"gitlab.ozon.dev/gojhw1/pkg/model"
	"go.opentelemetry.io/otel"
)

var (
	// ErrOrderNotFoundInCache возникает при попытке получить заказ, который отсутствует в кэше
	ErrOrderNotFoundInCache = errors.New("заказ не найден в кэше")
	// ErrOrderExpired возникает при попытке получить заказ, срок хранения которого истек
	ErrOrderExpired = errors.New("срок хранения заказа истек")
	// ErrOrderNotCached возникает при неудачной попытке добавить заказ в кэш из-за некорректных параметров
	ErrOrderNotCached = errors.New("ошибка в параметраз заказа, он не добавлен в кэш")
	// ErrHistoryNotFoundInCache возникает при попытке получить историю заказов, которая отсутствует в кэше
	ErrHistoryNotFoundInCache = errors.New("история заказов не найдена в кэше")
)

// ordersRepository определяет интерфейс для получения актуальных заказов из репозитория
type ordersRepository interface {
	ListActual(ctx context.Context) ([]model.Order, error)
}

type redisConstants struct {
	orderKeyPrefix     string
	historyKey         string
	defaultOrderTTL    time.Duration
	historyTTL         time.Duration
	additionalDuration time.Duration
}

// RedisCache представляет реализацию кэша на основе Redis
type RedisCache struct {
	client    *redis.Client
	refreshMu *sync.Mutex

	consts redisConstants
}

// NewRedisCache создает и инициализирует новый экземпляр Redis-кэша
// addr - адрес Redis-сервера
// Возвращает указатель на созданный кэш и ошибку, если произошла ошибка при подключении
func NewRedisCache(ctx context.Context, addr string, cfg *config.Config) (*RedisCache, error) {
	logger.Infof("Создание Redis-кэша с адресом: %s", addr)

	redisDB := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	if err := redisotel.InstrumentTracing(redisDB, redisotel.WithTracerProvider(otel.GetTracerProvider())); err != nil {
		logger.Errorf("Ошибка инструментирования Redis клиента для трейсинга: %v", err)
	}

	cache := &RedisCache{
		client:    redisDB,
		refreshMu: &sync.Mutex{},
		consts: redisConstants{
			orderKeyPrefix:     cfg.CacheType.OrderKeyPrefix,
			historyKey:         cfg.CacheType.HistoryKeyPrefix,
			defaultOrderTTL:    time.Duration(cfg.CacheType.OrderTTL) * time.Minute,
			historyTTL:         time.Duration(cfg.CacheType.HistoryTTL) * time.Minute,
			additionalDuration: time.Duration(cfg.CacheType.AdditionalDuration) * time.Minute,
		},
	}

	if err := cache.client.Ping(ctx).Err(); err != nil {
		logger.Errorf("Ошибка подключения к Redis: %v", err)
		return nil, err
	}

	logger.Infof("Redis-кэш успешно подключен к серверу: %s", addr)
	logger.Debugf("Настройки Redis-кэша: TTL заказов=%v, TTL истории=%v",
		cache.consts.defaultOrderTTL, cache.consts.historyTTL)

	return cache, nil
}

// InitCache инициализирует кэш заказов данными из репозитория
// repo - репозиторий для получения актуальных заказов
// Возвращает ошибку, если произошла ошибка при загрузке или кэшировании заказов
func (c *RedisCache) InitCache(ctx context.Context, repo ordersRepository) error {
	logger.Info("Инициализация Redis кэша заказов...")

	orders, err := repo.ListActual(ctx)
	if err != nil {
		logger.Errorf("Ошибка загрузки заказов при инициализации кэша: %v", err)
		return fmt.Errorf("ошибка загрузки заказов при инициализации кэша: %w", err)
	}

	logger.Infof("Загружено %d актуальных заказов для кэширования", len(orders))

	for _, order := range orders {
		if err := c.SetOrder(ctx, order); err != nil {
			logger.Errorf("Ошибка при кэшировании заказа %d: %v", order.ID, err)
			return fmt.Errorf("ошибка при кэшировании заказа %d: %w", order.ID, err)
		}
	}

	logger.Info("Redis кэш заказов успешно инициализирован")

	return nil
}

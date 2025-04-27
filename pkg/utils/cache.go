package utils

import (
	"context"
	"fmt"
	"time"

	"gitlab.ozon.dev/gojhw1/pkg/cache"
	"gitlab.ozon.dev/gojhw1/pkg/config"
	"gitlab.ozon.dev/gojhw1/pkg/model"
)

type ordersRepository interface {
	ListActual(ctx context.Context) ([]model.Order, error)
	List(ctx context.Context, searchTerm string) ([]model.Order, error)
}

type orderCache interface {
	SetOrder(ctx context.Context, order model.Order) error
	DeleteOrder(ctx context.Context, orderID int64) error
	ClearOrderCache(ctx context.Context) error
	GetOrder(ctx context.Context, orderID int64) (model.Order, error)
	GetOrderHistory(ctx context.Context) ([]model.Order, error)
}

// SetCashType создает и инициализирует кэш заказов в зависимости от типа кэша, указанного в конфигурации
func SetCashType(ctx context.Context, cfg *config.Config, orderRepo ordersRepository) (orderCache, error) {
	switch cfg.CacheType.Name {
	case "redis":
		ordersCache, err := cache.NewRedisCache(ctx, fmt.Sprintf("%s:%s", cfg.Redis.Host, cfg.Redis.Port), cfg)
		if err != nil {
			return nil, fmt.Errorf("ошибка подключения к Redis: %v", err)
		}
		if err = ordersCache.InitCache(ctx, orderRepo); err != nil {
			return nil, fmt.Errorf("ошибка инициализации кэша: %v", err)
		}
		// Запуск фоновой задачи для обновления кэша
		ordersCache.StartHistoryCacheRefresh(ctx, orderRepo, time.Duration(cfg.CacheType.HistoryRefreshInterval)*time.Minute)

		return ordersCache, nil
	case "inmem":
		ordersCache := cache.NewInMemoryCache(ctx, cfg)
		if err := ordersCache.InitCache(ctx, orderRepo); err != nil {
			return nil, fmt.Errorf("ошибка инициализации кэша: %v", err)
		}
		// Запуск фоновой задачи для обновления кэша
		ordersCache.StartHistoryCacheRefresh(ctx, orderRepo, time.Duration(cfg.CacheType.HistoryRefreshInterval)*time.Minute)

		return ordersCache, nil
	}

	return nil, fmt.Errorf("неизвестный тип кэша: %s", cfg.CacheType.Name)
}

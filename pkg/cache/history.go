package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"gitlab.ozon.dev/gojhw1/pkg/logger"
	"gitlab.ozon.dev/gojhw1/pkg/model"
)

// historyRepository определяет интерфейс для получения истории заказов из репозитория
type historyRepository interface {
	List(ctx context.Context, searchTerm string) ([]model.Order, error)
}

// GetOrderHistory возвращает историю заказов из кэша
// Возвращает ошибку, если история не найдена в кэше или произошла другая ошибка
func (c *RedisCache) GetOrderHistory(ctx context.Context) ([]model.Order, error) {
	logger.Debug("Получение истории заказов из Redis-кэша")

	historyJSON, err := c.client.Get(ctx, c.consts.historyKey).Result()
	if err == redis.Nil {
		logger.Debug("История заказов не найдена в Redis-кэше")
		return nil, ErrHistoryNotFoundInCache
	} else if err != nil {
		logger.Errorf("Ошибка получения истории заказов из кэша: %v", err)
		return nil, fmt.Errorf("ошибка получения истории заказов из кэша: %w", err)
	}

	var orders []model.Order
	if err := json.Unmarshal([]byte(historyJSON), &orders); err != nil {
		logger.Errorf("Ошибка десериализации истории заказов: %v", err)
		return nil, fmt.Errorf("ошибка десериализации истории заказов: %w", err)
	}

	logger.Debugf("История заказов успешно получена из Redis-кэша (%d заказов)", len(orders))
	return orders, nil
}

// refreshHistoryCache обновляет кэш истории заказов данными из репозитория
// repo - репозиторий для получения истории заказов
// Использует мьютекс для предотвращения одновременного обновления
// Возвращает ошибку, если произошла ошибка при получении или сохранении данных
func (c *RedisCache) refreshHistoryCache(ctx context.Context, repo historyRepository) error {
	if !c.refreshMu.TryLock() {
		logger.Debug("Пропуск обновления кэша истории заказов - уже выполняется другим процессом")
		return nil //уже происходит обновление
	}
	defer c.refreshMu.Unlock()

	logger.Debug("Запуск обновления кэша истории заказов")

	orders, err := c.getHistoryFromRepo(ctx, repo, "")
	if err != nil {
		logger.Errorf("Ошибка получения истории заказов из репозитория: %v", err)
		return err
	}

	ordersJSON, err := json.Marshal(orders)
	if err != nil {
		logger.Errorf("Ошибка сериализации истории заказов: %v", err)
		return fmt.Errorf("ошибка сериализации истории заказов: %w", err)
	}

	if err := c.client.Set(ctx, c.consts.historyKey, ordersJSON, c.consts.historyTTL).Err(); err != nil {
		logger.Errorf("Ошибка установки истории заказов в кэш: %v", err)
		return fmt.Errorf("ошибка установки истории заказов в кэш: %w", err)
	}

	logger.Infof("Обновление кэша истории заказов выполнено: сохранено %d заказов с TTL %v",
		len(orders), c.consts.historyTTL)
	return nil
}

// getHistoryFromRepo получает историю заказов из репозитория с учетом поискового запроса
// repo - репозиторий для получения истории
// searchTerm - строка поиска
// Возвращает список заказов и ошибку, если она произошла
func (c *RedisCache) getHistoryFromRepo(ctx context.Context, repo historyRepository, searchTerm string) ([]model.Order, error) {
	logger.Debugf("Получение истории заказов из репозитория с поисковым запросом: '%s'", searchTerm)
	orders, err := repo.List(ctx, searchTerm)
	if err != nil {
		return nil, err
	}
	logger.Debugf("Получено %d заказов из репозитория", len(orders))
	return orders, nil
}

// StartHistoryCacheRefresh запускает фоновый процесс периодического обновления кэша истории заказов
// repo - репозиторий для получения истории
// interval - интервал времени между обновлениями
// Процесс завершается при закрытии контекста
func (c *RedisCache) StartHistoryCacheRefresh(ctx context.Context, repo historyRepository, interval time.Duration) {
	logger.Infof("Запуск обновления кэша истории заказов с интервалом %s", interval)

	// Первоначальное обновление кэша
	if err := c.refreshHistoryCache(ctx, repo); err != nil {
		logger.Errorf("Начальная ошибка обновления кэша истории заказов: %v", err)
	}

	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				timeout, cancel := context.WithTimeout(ctx, 30*time.Second)
				if err := c.refreshHistoryCache(timeout, repo); err != nil {
					logger.Errorf("Ошибка обновления кэша истории заказов: %v", err)
				} else {
					logger.Info("Кэш истории заказов успешно обновлен")
				}
				cancel()
			case <-ctx.Done():
				logger.Info("Процесс обновления кэша истории заказов остановлен из-за завершения контекста")
				ticker.Stop()
				return
			}
		}
	}()

	logger.Info("Фоновый процесс обновления кэша истории заказов запущен")
}

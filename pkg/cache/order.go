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

// SetOrder добавляет или обновляет заказ в Redis-кэше
// order - добавляемый/обновляемый заказ
// Устанавливает TTL на основе времени до дедлайна заказа
// Возвращает ошибку, если заказ просрочен или возникла другая ошибка
func (c *RedisCache) SetOrder(ctx context.Context, order model.Order) error {
	orderJSON, err := json.Marshal(order)
	if err != nil {
		logger.Errorf("Ошибка сериализации заказа %d: %v", order.ID, err)
		return fmt.Errorf("ошибка сериализации заказа %d: %w", order.ID, err)
	}

	key := fmt.Sprintf("%s%d", c.consts.orderKeyPrefix, order.ID)

	now := time.Now()
	if now.After(order.DeadlineAt) {
		logger.Warnf("Попытка кэширования просроченного заказа: ID=%d, дедлайн=%v",
			order.ID, order.DeadlineAt)
		return fmt.Errorf("%w: %d", ErrOrderNotCached, order.ID)
	}

	// Оставшееся время до дедлайна + небольшой запас (1 час)
	ttl := min(time.Until(order.DeadlineAt)+c.consts.additionalDuration, c.consts.defaultOrderTTL)

	logger.Debugf("Сохранение заказа в Redis-кэш: ID=%d, TTL=%v, дедлайн=%v",
		order.ID, ttl, order.DeadlineAt)

	if err := c.client.Set(ctx, key, orderJSON, ttl).Err(); err != nil {
		logger.Errorf("Ошибка записи заказа %d в Redis: %v", order.ID, err)
		return err
	}

	return nil
}

// GetOrder возвращает заказ с указанным ID из Redis-кэша
// orderID - идентификатор заказа
// Проверяет актуальность заказа и удаляет просроченные заказы
// Возвращает ошибку, если заказ не найден, просрочен или возникла другая ошибка
func (c *RedisCache) GetOrder(ctx context.Context, orderID int64) (model.Order, error) {
	key := fmt.Sprintf("%s%d", c.consts.orderKeyPrefix, orderID)

	logger.Debugf("Получение заказа из Redis-кэша: ID=%d, ключ=%s", orderID, key)

	orderJSON, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		logger.Debugf("Заказ с ID=%d не найден в кэше", orderID)
		return model.Order{}, fmt.Errorf("%w: %d", ErrOrderNotFoundInCache, orderID)
	} else if err != nil {
		logger.Errorf("Ошибка получения заказа %d из кэша: %v", orderID, err)
		return model.Order{}, fmt.Errorf("ошибка получения заказа %d из кэша: %w", orderID, err)
	}

	var order model.Order
	if err := json.Unmarshal([]byte(orderJSON), &order); err != nil {
		logger.Errorf("Ошибка десериализации заказа %d: %v", orderID, err)
		return model.Order{}, fmt.Errorf("ошибка десериализации заказа %d: %w", orderID, err)
	}

	if time.Now().After(order.DeadlineAt) {
		logger.Warnf("Заказ %d в кэше просрочен: дедлайн=%v, текущее время=%v",
			orderID, order.DeadlineAt, time.Now())
		if err := c.DeleteOrder(ctx, orderID); err != nil {
			logger.Warnf("Ошибка удаления просроченного заказа %d из кэша: %v", orderID, err)
		}
		return model.Order{}, fmt.Errorf("%w: %d", ErrOrderExpired, orderID)
	}

	logger.Debugf("Заказ %d успешно получен из Redis-кэша", orderID)
	return order, nil
}

// DeleteOrder удаляет заказ с указанным ID из Redis-кэша
// orderID - идентификатор заказа
// Возвращает ошибку, если возникла проблема при удалении
func (c *RedisCache) DeleteOrder(ctx context.Context, orderID int64) error {
	key := fmt.Sprintf("%s%d", c.consts.orderKeyPrefix, orderID)

	logger.Debugf("Удаление заказа из Redis-кэша: ID=%d, ключ=%s", orderID, key)

	if err := c.client.Del(ctx, key).Err(); err != nil {
		logger.Errorf("Ошибка удаления заказа %d из Redis: %v", orderID, err)
		return err
	}

	logger.Debugf("Заказ %d успешно удален из Redis-кэша", orderID)
	return nil
}

// ClearOrderCache полностью очищает кэш заказов в Redis
// Находит и удаляет все ключи с заказами
// Возвращает ошибку, если возникла проблема при очистке
func (c *RedisCache) ClearOrderCache(ctx context.Context) error {
	logger.Info("Очистка всего кэша заказов в Redis")

	pattern := c.consts.orderKeyPrefix + "*"
	keys, err := c.client.Keys(ctx, pattern).Result()
	if err != nil {
		logger.Errorf("Ошибка получения ключей при очистке кэша: %v", err)
		return fmt.Errorf("ошибка получения ключей при очистке кэша: %w", err)
	}

	if len(keys) > 0 {
		logger.Infof("Найдено %d заказов для удаления из Redis-кэша", len(keys))
		if err := c.client.Del(ctx, keys...).Err(); err != nil {
			logger.Errorf("Ошибка удаления ключей при очистке кэша: %v", err)
			return fmt.Errorf("ошибка удаления ключей при очистке кэша: %w", err)
		}
		logger.Infof("Успешно удалено %d заказов из Redis-кэша", len(keys))
	} else {
		logger.Info("В Redis-кэше не найдено заказов для удаления")
	}

	return nil
}

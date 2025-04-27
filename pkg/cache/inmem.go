package cache

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"time"

	"gitlab.ozon.dev/gojhw1/pkg/config"
	"gitlab.ozon.dev/gojhw1/pkg/logger"
	"gitlab.ozon.dev/gojhw1/pkg/model"
)

// orderEntry представляет запись о заказе в кэше с указанием TTL
type orderEntry struct {
	order model.Order
	ttl   time.Time
}

// historyEntry представляет запись об истории заказов в кэше с указанием TTL
type historyEntry struct {
	orders []model.Order
	ttl    time.Time
}

// lruItem представляет элемент в LRU списке для отслеживания использования заказов
type lruItem struct {
	ID    int64
	entry orderEntry
}

type inmemConstants struct {
	cleanupInterval      time.Duration
	inmemDefaultOrderTTL time.Duration
	inmemHistoryTTL      time.Duration
	maxCacheSize         int
	additionalDuration   time.Duration
}

// InMemoryCache представляет реализацию кэша в памяти с поддержкой LRU механизма
type InMemoryCache struct {
	lruList      *list.List
	ordersLruMap map[int64]*list.Element
	ordersMu     sync.RWMutex

	history   *historyEntry
	historyMu sync.RWMutex
	refreshMu sync.Mutex

	consts inmemConstants
}

// NewInMemoryCache создает и инициализирует новый экземпляр кэша в памяти
// Запускает фоновый процесс очистки устаревших записей
// Возвращает указатель на созданный кэш
func NewInMemoryCache(ctx context.Context, cfg *config.Config) *InMemoryCache {
	logger.Info("Создание in-memory кэша")

	cache := &InMemoryCache{
		lruList:      list.New(),
		ordersLruMap: make(map[int64]*list.Element),
		history:      nil,
		consts: inmemConstants{
			cleanupInterval:      time.Duration(cfg.CacheType.CleanupInterval) * time.Minute,
			inmemDefaultOrderTTL: time.Duration(cfg.CacheType.OrderTTL) * time.Minute,
			inmemHistoryTTL:      time.Duration(cfg.CacheType.HistoryTTL) * time.Minute,
			maxCacheSize:         cfg.CacheType.MaxCacheSize,
			additionalDuration:   time.Duration(cfg.CacheType.AdditionalDuration) * time.Minute,
		},
	}

	logger.Debugf("Настройки in-memory кэша: максимальный размер=%d, интервал очистки=%v, TTL заказов=%v",
		cache.consts.maxCacheSize, cache.consts.cleanupInterval, cache.consts.inmemDefaultOrderTTL)

	go cache.startCleanupTicker(ctx)
	logger.Info("Запущен фоновый процесс очистки устаревших записей в кэше")

	return cache
}

// InitCache инициализирует кэш заказов данными из репозитория
// repo - репозиторий для получения актуальных заказов
// Возвращает ошибку, если произошла ошибка при загрузке или кэшировании заказов
func (c *InMemoryCache) InitCache(ctx context.Context, repo ordersRepository) error {
	logger.Info("Инициализация in-memory кэша заказов...")

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

	logger.Info("In-memory кэш заказов успешно инициализирован")

	return nil
}

// startCleanupTicker запускает фоновый процесс периодической очистки устаревших записей в кэше
// Процесс завершается при закрытии контекста
func (c *InMemoryCache) startCleanupTicker(ctx context.Context) {
	logger.Infof("Запущен процесс очистки кэша с интервалом %v", c.consts.cleanupInterval)

	ticker := time.NewTicker(c.consts.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			select {
			case <-ctx.Done():
				return
			default:
				logger.Debug("Запущена плановая очистка устаревших записей в кэше")
				c.cleanupExpiredOrders()
			}
		case <-ctx.Done():
			logger.Info("Процесс очистки кэша остановлен из-за завершения контекста")
			return
		}
	}
}

// cleanupExpiredOrders удаляет просроченные заказы и историю из кэша
func (c *InMemoryCache) cleanupExpiredOrders() {
	now := time.Now()
	var expiredIDs []int64

	c.ordersMu.RLock()
	for _, elem := range c.ordersLruMap {
		item := elem.Value.(*lruItem)
		if now.After(item.entry.ttl) {
			expiredIDs = append(expiredIDs, item.ID)
		}
	}
	c.ordersMu.RUnlock()

	if len(expiredIDs) > 0 {
		logger.Infof("Найдено %d просроченных заказов для удаления из кэша", len(expiredIDs))

		c.ordersMu.Lock()
		for _, id := range expiredIDs {
			if elem, ok := c.ordersLruMap[id]; ok {
				delete(c.ordersLruMap, id)
				c.lruList.Remove(elem)
			}
		}
		c.ordersMu.Unlock()

		logger.Infof("Удалено %d просроченных заказов из кэша", len(expiredIDs))
	} else {
		logger.Debug("Просроченных заказов в кэше не найдено")
	}

	c.historyMu.RLock()
	historyExpired := c.history != nil && now.After(c.history.ttl)
	c.historyMu.RUnlock()

	if historyExpired {
		logger.Info("Кэш истории заказов признан просроченным")

		c.historyMu.Lock()
		c.history = nil
		c.historyMu.Unlock()

		logger.Info("Кэш истории заказов очищен")
	}
}

// SetOrder добавляет или обновляет заказ в кэше
// order - добавляемый/обновляемый заказ
// Возвращает ошибку, если заказ просрочен или возникла другая ошибка
// Реализует механизм LRU для ограничения размера кэша
func (c *InMemoryCache) SetOrder(ctx context.Context, order model.Order) error {
	select {
	case <-ctx.Done():
		logger.Errorf("Ошибка установки заказа %d в кэш: контекст завершен", order.ID)
		return fmt.Errorf("ошибка установки заказа %d в кэш: %w", order.ID, ctx.Err())
	default:
		now := time.Now()
		if now.After(order.DeadlineAt) {
			logger.Warnf("Попытка кэширования просроченного заказа: ID=%d, дедлайн=%v",
				order.ID, order.DeadlineAt)
			return fmt.Errorf("%w: %d", ErrOrderNotCached, order.ID)
		}

		ttl := min(time.Until(order.DeadlineAt)+c.consts.additionalDuration, c.consts.inmemDefaultOrderTTL)
		expiration := now.Add(ttl)

		logger.Debugf("Сохранение заказа в in-memory кэш: ID=%d, TTL=%v, дедлайн=%v",
			order.ID, ttl, order.DeadlineAt)

		c.ordersMu.Lock()
		defer c.ordersMu.Unlock()

		entry := orderEntry{
			order: order,
			ttl:   expiration,
		}

		if elem, ok := c.ordersLruMap[order.ID]; ok {
			item := elem.Value.(*lruItem)
			item.entry = entry
			c.lruList.MoveToFront(elem)
			logger.Debugf("Обновлен существующий заказ %d в кэше", order.ID)
			return nil
		}

		if c.lruList.Len() >= c.consts.maxCacheSize {
			lastElem := c.lruList.Back()
			if lastElem != nil {
				item := lastElem.Value.(*lruItem)
				logger.Debugf("Кэш заполнен, вытеснение наименее используемого заказа: ID=%d", item.ID)
				delete(c.ordersLruMap, item.ID)
				c.lruList.Remove(lastElem)
			}
		}

		item := &lruItem{
			ID:    order.ID,
			entry: entry,
		}
		elem := c.lruList.PushFront(item)
		c.ordersLruMap[order.ID] = elem

		logger.Debugf("Добавлен новый заказ %d в кэш", order.ID)

		return nil
	}
}

// GetOrder возвращает заказ с указанным ID из кэша
// orderID - идентификатор заказа
// Обновляет положение заказа в LRU списке при успешном получении
// Возвращает ошибку, если заказ не найден, просрочен или возникла другая ошибка
func (c *InMemoryCache) GetOrder(ctx context.Context, orderID int64) (model.Order, error) {
	select {
	case <-ctx.Done():
		logger.Errorf("Ошибка получения заказа %d из кэша: контекст завершен", orderID)
		return model.Order{}, fmt.Errorf("ошибка получения заказа %d из кэша: %w", orderID, ctx.Err())
	default:
		logger.Debugf("Получение заказа из in-memory кэша: ID=%d", orderID)

		c.ordersMu.RLock()
		elem, ok := c.ordersLruMap[orderID]
		if !ok {
			c.ordersMu.RUnlock()
			logger.Debugf("Заказ с ID=%d не найден в кэше", orderID)
			return model.Order{}, fmt.Errorf("%w: %d", ErrOrderNotFoundInCache, orderID)
		}
		c.ordersMu.RUnlock()

		item := elem.Value.(*lruItem)
		entry := item.entry

		now := time.Now()
		if now.After(entry.ttl) || now.After(entry.order.DeadlineAt) {
			logger.Warnf("Заказ %d в кэше просрочен: TTL=%v, дедлайн=%v, текущее время=%v",
				orderID, entry.ttl, entry.order.DeadlineAt, now)

			if err := c.DeleteOrder(ctx, orderID); err != nil {
				logger.Warnf("Ошибка удаления просроченного заказа %d из кэша: %v", orderID, err)
			}
			return model.Order{}, fmt.Errorf("%w: %d", ErrOrderExpired, orderID)
		}

		c.ordersMu.Lock()
		if _, ok := c.ordersLruMap[orderID]; ok {
			c.lruList.MoveToFront(elem)
			logger.Debugf("Заказ %d перемещен в начало LRU списка", orderID)
		}
		c.ordersMu.Unlock()

		logger.Debugf("Заказ %d успешно получен из in-memory кэша", orderID)
		return entry.order, nil
	}
}

// DeleteOrder удаляет заказ с указанным ID из кэша
// orderID - идентификатор заказа
// Возвращает ошибку, если заказ не найден или возникла другая ошибка
func (c *InMemoryCache) DeleteOrder(ctx context.Context, orderID int64) error {
	select {
	case <-ctx.Done():
		logger.Errorf("Ошибка удаления заказа %d из кэша: контекст завершен", orderID)
		return fmt.Errorf("ошибка удаления заказа %d из кэша: %w", orderID, ctx.Err())
	default:
		logger.Debugf("Удаление заказа из in-memory кэша: ID=%d", orderID)

		c.ordersMu.Lock()
		defer c.ordersMu.Unlock()

		elem, ok := c.ordersLruMap[orderID]
		if !ok {
			logger.Debugf("Заказ с ID=%d не найден в кэше при удалении", orderID)
			return fmt.Errorf("%w: %d", ErrOrderNotFoundInCache, orderID)
		}

		delete(c.ordersLruMap, orderID)
		c.lruList.Remove(elem)

		logger.Debugf("Заказ %d успешно удален из in-memory кэша", orderID)

		return nil
	}
}

// ClearOrderCache полностью очищает кэш заказов
// Возвращает ошибку, если возникла проблема при очистке
func (c *InMemoryCache) ClearOrderCache(ctx context.Context) error {
	select {
	case <-ctx.Done():
		logger.Errorf("Ошибка очистки кэша заказов: контекст завершен")
		return fmt.Errorf("ошибка очистки кэша заказов: %w", ctx.Err())
	default:
		logger.Info("Очистка всего in-memory кэша заказов")

		c.ordersMu.Lock()
		defer c.ordersMu.Unlock()

		logger.Infof("Удаление %d заказов из кэша", len(c.ordersLruMap))
		c.lruList.Init()
		c.ordersLruMap = make(map[int64]*list.Element)

		logger.Info("In-memory кэш заказов успешно очищен")
		return nil
	}
}

// GetOrderHistory возвращает историю заказов из кэша
// Возвращает ошибку, если история не найдена в кэше, просрочена или возникла другая ошибка
func (c *InMemoryCache) GetOrderHistory(ctx context.Context) ([]model.Order, error) {
	select {
	case <-ctx.Done():
		logger.Errorf("Ошибка получения истории заказов из кэша: контекст завершен")
		return nil, fmt.Errorf("ошибка получения истории заказов из кэша: %w", ctx.Err())
	default:
		logger.Debug("Получение истории заказов из in-memory кэша")

		c.historyMu.RLock()
		defer c.historyMu.RUnlock()

		if c.history == nil {
			logger.Debug("История заказов не найдена в кэше (nil)")
			return nil, fmt.Errorf("%w: история заказов не найдена в кэше", ErrHistoryNotFoundInCache)
		}

		if time.Now().After(c.history.ttl) {
			logger.Debug("История заказов в кэше просрочена")
			return nil, fmt.Errorf("%w: история заказов просрочена", ErrHistoryNotFoundInCache)
		}

		result := make([]model.Order, len(c.history.orders))
		copy(result, c.history.orders)

		logger.Debugf("История заказов успешно получена из кэша (%d заказов)", len(result))
		return result, nil
	}
}

// refreshHistoryCache обновляет кэш истории заказов данными из репозитория
// repo - репозиторий для получения истории заказов
// Использует мьютекс для предотвращения одновременного обновления
// Возвращает ошибку, если произошла ошибка при получении или сохранении данных
func (c *InMemoryCache) refreshHistoryCache(ctx context.Context, repo historyRepository) error {
	if !c.refreshMu.TryLock() {
		logger.Debug("Пропуск обновления кэша истории заказов - уже выполняется другим процессом")
		return nil
	}
	defer c.refreshMu.Unlock()

	logger.Debug("Запуск обновления кэша истории заказов")

	orders, err := c.getHistoryFromRepo(ctx, repo, "")
	if err != nil {
		logger.Errorf("Ошибка получения истории заказов из репозитория: %v", err)
		return err
	}

	now := time.Now()
	ttl := now.Add(c.consts.inmemHistoryTTL)

	entry := &historyEntry{
		orders: orders,
		ttl:    ttl,
	}

	c.historyMu.Lock()
	defer c.historyMu.Unlock()
	c.history = entry

	logger.Infof("Обновление кэша истории заказов выполнено: сохранено %d заказов с TTL %v",
		len(orders), c.consts.inmemHistoryTTL)
	return nil
}

// getHistoryFromRepo получает историю заказов из репозитория с учетом поискового запроса
// repo - репозиторий для получения истории
// searchTerm - строка поиска
// Возвращает список заказов и ошибку, если она произошла
func (c *InMemoryCache) getHistoryFromRepo(ctx context.Context, repo historyRepository, searchTerm string) ([]model.Order, error) {
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
func (c *InMemoryCache) StartHistoryCacheRefresh(ctx context.Context, repo historyRepository, interval time.Duration) {
	logger.Infof("Запуск обновления кэша истории заказов с интервалом %s", interval)

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

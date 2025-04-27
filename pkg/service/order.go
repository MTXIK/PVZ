package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"gitlab.ozon.dev/gojhw1/pkg/logger"
	"gitlab.ozon.dev/gojhw1/pkg/metrics"
	"gitlab.ozon.dev/gojhw1/pkg/model"
)

var (
	// ErrStorageDeadlinePassed - ошибка, возникающая когда срок хранения уже истек
	ErrStorageDeadlinePassed = errors.New("срок хранения в прошлом")
	// ErrOrderExists - ошибка, возникающая при попытке создать заказ с существующим ID
	ErrOrderExists = errors.New("заказ уже существует")
	// ErrDeadlineNotExpired - ошибка, возникающая при попытке вернуть заказ до истечения срока хранения
	ErrDeadlineNotExpired = errors.New("срок хранения заказа еще не истек")
	// ErrOrderAlreadyDelivered - ошибка, возникающая при попытке вернуть уже доставленный заказ
	ErrOrderAlreadyDelivered = errors.New("заказ уже доставлен клиенту, возврат невозможен")
	// ErrWrongCustomer - ошибка, возникающая при попытке получить заказ другого клиента
	ErrWrongCustomer = errors.New("заказ принадлежит другому клиенту")
	// ErrWrongState - ошибка, возникающая при попытке выдать заказ в неподходящем состоянии
	ErrWrongState = errors.New("заказ нельзя выдать – неверное состояние")
	// ErrStorageExpired - ошибка, возникающая при попытке получить заказ с истекшим сроком хранения
	ErrStorageExpired = errors.New("срок хранения заказа истек")
	// ErrNotDelivered - ошибка, возникающая при попытке вернуть недоставленный заказ
	ErrNotDelivered = errors.New("заказ еще не был выдан клиенту, возврат невозможен")
	// ErrReturnExpired - ошибка, возникающая когда срок возврата заказа истек
	ErrReturnExpired = errors.New("срок возврата заказа истек")
	// ErrOpenFile - ошибка при открытии файла с заказами
	ErrOpenFile = errors.New("ошибка при открытии файла принятия заказов")
	// ErrReadFile - ошибка при чтении данных из файла с заказами
	ErrReadFile = errors.New("ошибка при чтении файла принятия заказов")
	// ErrParseFile - ошибка при разборе содержимого файла с заказами
	ErrParseFile = errors.New("ошибка при разборе файла принятия заказов")
	// ErrInvalidDateFormat - ошибка при некорректном формате даты
	ErrInvalidDateFormat = errors.New("неверный формат даты или длительности")
	// ErrNegativeWeight - ошибка при указании отрицательного или нулевого веса
	ErrNegativeWeight = errors.New("вес должен быть положительным числом")
	// ErrNegativeCost - ошибка при указании отрицательной или нулевой стоимости
	ErrNegativeCost = errors.New("стоимость должна быть положительным числом")
	// ErrInvalidOrderID - ошибка при указании некорректного ID заказа
	ErrInvalidOrderID = errors.New("недопустимый ID заказа")
)

const ReturnedAt = 48 * time.Hour
const timeLayout = "2006-01-02T15:04:05"

type orderRepository interface {
	Create(ctx context.Context, order model.Order) error
	Update(ctx context.Context, order model.Order) error
	Delete(ctx context.Context, id int64) error
	GetByID(ctx context.Context, id int64) (model.Order, error)
	List(ctx context.Context, searchTerm string) ([]model.Order, error)
	ListWithCursor(ctx context.Context, cursorID int64, limit int, customerID int64, filterPVZ bool, searchTerm string) ([]model.Order, error)
	ListReturnsWithCursor(ctx context.Context, cursorID int64, limit int, searchTerm string) ([]model.Order, error)
}

type auditLogger interface {
	Log(ctx context.Context, log model.AuditLog)
	LogOrderStatusChange(ctx context.Context, orderID int64, oldStatus, newStatus string)
}

type orderCache interface {
	SetOrder(ctx context.Context, order model.Order) error
	DeleteOrder(ctx context.Context, orderID int64) error
	ClearOrderCache(ctx context.Context) error
	GetOrder(ctx context.Context, orderID int64) (model.Order, error)
	GetOrderHistory(ctx context.Context) ([]model.Order, error)
}

// OrderService - структура сервиса для работы с заказами
type OrderService struct {
	repo   orderRepository
	logger auditLogger
	cache  orderCache
}

// NewOrderService - создаёт новый сервис с переданным репозиторием
func NewOrderService(repo orderRepository, logger auditLogger, cache orderCache) *OrderService {
	return &OrderService{
		repo:   repo,
		logger: logger,
		cache:  cache,
	}
}

// AcceptOrder - принимает заказ, если он корректен и не просрочен
func (s *OrderService) AcceptOrder(ctx context.Context, id, customerID int64, deadline time.Time, weight, cost float64, packageType *model.PackageType, wrapper *model.WrapperType) error {
	now := time.Now()
	if id <= 0 {
		logger.Errorf("Невалидный ID заказа: %d", id)
		return fmt.Errorf("%w: %d", ErrInvalidOrderID, id)
	}
	if now.After(deadline) {
		logger.Errorf("Срок хранения заказа %d уже истек: %v (текущая дата: %v)", id, deadline, now)
		return fmt.Errorf("%w: %v \n Текущая дата: %v", ErrStorageDeadlinePassed, deadline, now)
	}
	if order, _ := s.repo.GetByID(ctx, id); order.ID == id {
		logger.Errorf("Заказ с ID %d уже существует", id)
		return fmt.Errorf("%w: Id %d", ErrOrderExists, id)
	}
	if weight <= 0 {
		logger.Errorf("Недопустимый вес заказа %d: %v", id, weight)
		return fmt.Errorf("%w: %v", ErrNegativeWeight, weight)
	}
	if cost <= 0 {
		logger.Errorf("Недопустимая стоимость заказа %d: %v", id, cost)
		return fmt.Errorf("%w: %v", ErrNegativeCost, cost)
	}

	finalCost := cost

	if packageType != nil {
		factory := newPackagerFactory()
		packager, err := factory.createPackager(packageType, wrapper)
		if err != nil {
			logger.Errorf("Ошибка создания упаковщика для заказа %d: %v", id, err)
			return fmt.Errorf("ошибка создания упаковщика: %w", err)
		}

		if err = packager.validateWeight(weight); err != nil {
			logger.Errorf("Ошибка проверки веса для упаковки %s заказа %d: %v", *packageType, id, err)
			return fmt.Errorf("ошибка проверки веса для упаковки %s: %w", *packageType, err)
		}

		finalCost += packager.getAdditionalCost()
		logger.Debugf("Финальная стоимость заказа %d после добавления упаковки: %v", id, finalCost)
	}

	order := model.Order{
		ID:          id,
		CustomerID:  customerID,
		DeadlineAt:  deadline,
		State:       model.StateAccepted,
		UpdatedAt:   now,
		Weight:      weight,
		Cost:        finalCost,
		PackageType: packageType,
		Wrapper:     wrapper,
	}

	if err := s.repo.Create(ctx, order); err != nil {
		logger.Errorf("Ошибка создания заказа %d в БД: %v", id, err)
		return err
	}

	if err := s.cache.SetOrder(ctx, order); err != nil {
		logger.Warnf("Ошибка сохранения заказа %d в кэше: %v", id, err)
		return err
	}

	s.logger.LogOrderStatusChange(ctx, id, "none", string(order.State))
	logger.Infof("Заказ %d успешно принят", id)

	metrics.OrdersAccepted.Inc()

	return nil
}

// ReturnOrderToCourier - возвращает заказ курьеру, если условия возврата соблюдены
func (s *OrderService) ReturnOrderToCourier(ctx context.Context, id int64) error {
	now := time.Now()
	var order model.Order

	order, err := s.cache.GetOrder(ctx, id)
	if err != nil {
		logger.Debugf("Ошибка получения заказа %d из кэша: %v, обращаемся к БД", id, err)
		order, err = s.repo.GetByID(ctx, id)
		if err != nil {
			logger.Errorf("Ошибка получения заказа %d из БД: %v", id, err)
			return fmt.Errorf("ошибка при возврата заказа курьеру Id %d: %w", id, err)
		}
	}

	if order.State == model.StateDelivered {
		logger.Errorf("Невозможно вернуть курьеру уже доставленный заказ %d", id)
		return fmt.Errorf("%w: ID %d", ErrOrderAlreadyDelivered, id)
	}
	if now.Before(order.DeadlineAt) && order.State != model.StateReturned {
		logger.Errorf("Срок хранения заказа %d еще не истек: %v (текущая дата: %v)", id, order.DeadlineAt, now)
		return fmt.Errorf("%w: %v\n текущая дата: %v", ErrDeadlineNotExpired, order.DeadlineAt, now)
	}

	state := order.State

	if err := s.cache.DeleteOrder(ctx, id); err != nil {
		logger.Warnf("Ошибка удаления заказа %d из кэша: %v", id, err)
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		logger.Errorf("Ошибка удаления заказа %d из БД: %v", id, err)
		return err
	}

	s.logger.LogOrderStatusChange(ctx, id, string(state), "deleted")
	logger.Infof("Заказ %d успешно возвращен курьеру", id)

	metrics.OrdersReturnedToCourier.Inc()

	return nil
}

// DeliverOrder - доставляет заказ клиенту, если заказ принадлежит клиенту и не просрочен
func (s *OrderService) DeliverOrder(ctx context.Context, id, customerID int64, now time.Time) error {
	order, err := s.cache.GetOrder(ctx, id)
	if err != nil {
		logger.Debugf("Ошибка получения заказа %d из кэша: %v, обращаемся к БД", id, err)
		order, err = s.repo.GetByID(ctx, id)
		if err != nil {
			logger.Errorf("Ошибка получения заказа %d из БД: %v", id, err)
			return fmt.Errorf("ошибка при доставке заказа Id %d: %w", id, err)
		}
	}

	if order.CustomerID != customerID {
		logger.Errorf("Заказ %d принадлежит другому клиенту (запрошен %d, владелец %d)",
			id, customerID, order.CustomerID)
		return fmt.Errorf("%w: ID %d", ErrWrongCustomer, id)
	}
	if order.State != model.StateAccepted {
		logger.Errorf("Невозможно выдать заказ %d в статусе %s", id, order.State)
		return fmt.Errorf("%w: ID %d", ErrWrongState, id)
	}
	if now.After(order.DeadlineAt) {
		logger.Errorf("Срок хранения заказа %d истек: %v (текущая дата: %v)", id, order.DeadlineAt, now)
		return fmt.Errorf("%w: %v \n Текущая дата: %v", ErrStorageExpired, order.DeadlineAt, now)
	}

	metricsUpdated := order.UpdatedAt

	oldState := order.State

	order.State = model.StateDelivered
	order.UpdatedAt = now
	order.DeliveredAt = &now

	if err := s.repo.Update(ctx, order); err != nil {
		logger.Errorf("Ошибка обновления заказа %d в БД: %v", id, err)
		return err
	}
	if err := s.cache.SetOrder(ctx, order); err != nil {
		logger.Warnf("Ошибка сохранения заказа %d в кэше после выдачи: %v", id, err)
		return err
	}

	s.logger.LogOrderStatusChange(ctx, id, string(oldState), string(order.State))
	logger.Infof("Заказ %d успешно выдан клиенту %d", id, customerID)

	metrics.OrdersDelivered.Inc()

	processingTime := order.DeliveredAt.Sub(metricsUpdated).Seconds()
	metrics.OrdersProcessingTime.Observe(processingTime)

	return nil
}

// ProcessReturnOrder - обрабатывает возврат заказа от клиента, если соблюдены условия возврата
func (s *OrderService) ProcessReturnOrder(ctx context.Context, id, customerID int64, now time.Time) error {
	order, err := s.cache.GetOrder(ctx, id)
	if err != nil {
		logger.Debugf("Ошибка получения заказа %d из кэша: %v, обращаемся к БД", id, err)
		order, err = s.repo.GetByID(ctx, id)
		if err != nil {
			logger.Errorf("Ошибка получения заказа %d из БД: %v", id, err)
			return fmt.Errorf("ошибка при возврате заказа Id %d: %w", id, err)
		}
	}

	if order.CustomerID != customerID {
		logger.Errorf("Заказ %d принадлежит другому клиенту (запрошен %d, владелец %d)",
			id, customerID, order.CustomerID)
		return fmt.Errorf("%w: ID %d", ErrWrongCustomer, id)
	}
	if order.State != model.StateDelivered {
		logger.Errorf("Невозможно вернуть заказ %d в статусе %s (требуется статус %s)",
			id, order.State, model.StateDelivered)
		return fmt.Errorf("%w: ID %d", ErrNotDelivered, id)
	}
	if now.Sub(*order.DeliveredAt) > ReturnedAt {
		logger.Errorf("Срок возврата заказа %d истек: доставлен %v, текущая дата %v, максимальный срок возврата %v",
			id, order.DeliveredAt, now, ReturnedAt)
		return fmt.Errorf("%w: %v \n Текущая дата: %v", ErrReturnExpired, order.DeliveredAt, now)
	}

	oldState := order.State

	order.State = model.StateReturned
	order.UpdatedAt = now
	order.ReturnedAt = &now

	if err := s.repo.Update(ctx, order); err != nil {
		logger.Errorf("Ошибка обновления заказа %d в БД при возврате: %v", id, err)
		return err
	}
	if err := s.cache.DeleteOrder(ctx, id); err != nil {
		logger.Warnf("Ошибка удаления заказа %d из кэша при возврате: %v", id, err)
		return err
	}

	s.logger.LogOrderStatusChange(ctx, id, string(oldState), string(order.State))
	logger.Infof("Заказ %d успешно возвращен клиентом %d", id, customerID)

	metrics.OrdersReturned.Inc()

	return nil
}

// OrderHistory - возвращает историю заказов с учетом поискового запроса
func (s *OrderService) OrderHistory(ctx context.Context, searchTerm string) ([]model.Order, error) {
	var orders []model.Order
	var err error

	// Если нет поискового запроса, пробуем использовать кэш
	if searchTerm == "" {
		orders, err = s.cache.GetOrderHistory(ctx)
		if err != nil {
			logger.Debugf("Не удалось получить историю из кеша: %v", err)
			// При ошибке кеша переключаемся на БД
			orders, err = s.repo.List(ctx, searchTerm)
			if err != nil {
				logger.Errorf("Ошибка получения списка заказов из БД: %v", err)
				return nil, fmt.Errorf("ошибка при получении списка заказов: %w", err)
			}
		}
	} else {
		// При наличии поискового запроса сразу идем в БД
		logger.Debugf("Получение истории заказов с поисковым запросом: %s", searchTerm)
		orders, err = s.repo.List(ctx, searchTerm)
		if err != nil {
			logger.Errorf("Ошибка получения списка заказов из БД с поисковым запросом %s: %v", searchTerm, err)
			return nil, fmt.Errorf("ошибка при получении списка заказов: %w", err)
		}
	}

	// Сортировка применяется один раз к итоговым данным
	sort.Slice(orders, func(i, j int) bool {
		return orders[i].UpdatedAt.After(orders[j].UpdatedAt)
	})

	logger.Infof("Получена история заказов, найдено %d записей", len(orders))
	return orders, nil
}

// AcceptOrdersFromFile - принимает заказы из файла с форматом JSON
func (s *OrderService) AcceptOrdersFromFile(ctx context.Context, filename string) error {
	logger.Infof("Начинаем импорт заказов из файла: %s", filename)

	orders, err := readOrdersFromFile(filename)
	if err != nil {
		logger.Errorf("Ошибка чтения заказов из файла %s: %v", filename, err)
		return err
	}

	logger.Infof("Успешно прочитано %d заказов из файла", len(orders))

	for _, order := range orders {
		deadline, err := parseDeadline(order.DeadlineAt)
		if err != nil {
			logger.Errorf("Ошибка парсинга дедлайна для заказа %d: %v", order.ID, err)
			return err
		}

		packageType, wrapper := processPackaging(order.PackageType, order.Wrapper)

		logger.Debugf("Обработка заказа из файла: ID=%d, CustomerID=%d, Weight=%v, Cost=%v",
			order.ID, order.CustomerID, order.Weight, order.Cost)

		if err = s.AcceptOrder(
			ctx,
			order.ID,
			order.CustomerID,
			deadline,
			order.Weight,
			order.Cost,
			packageType,
			wrapper,
		); err != nil {
			logger.Errorf("Ошибка принятия заказа %d из файла: %v", order.ID, err)
			return fmt.Errorf("ошибка при принятии заказа %d: %w", order.ID, err)
		}

		s.logger.LogOrderStatusChange(ctx, order.ID, "none", string(model.StateAccepted))
	}

	logger.Infof("Успешно импортировано %d заказов из файла %s", len(orders), filename)
	return nil
}

// GetOrderByID - находит заказ по его ID
func (s *OrderService) GetOrderByID(ctx context.Context, id int64) (model.Order, error) {
	logger.Debugf("Запрос заказа по ID: %d", id)

	order, err := s.cache.GetOrder(ctx, id)
	if err == nil {
		logger.Debugf("Заказ %d найден в кэше", id)
		return order, nil
	}

	// Если не нашли в кэше, обращаемся к БД
	logger.Debugf("Заказ %d не найден в кэше, обращаемся к БД", id)
	order, err = s.repo.GetByID(ctx, id)
	if err != nil {
		logger.Errorf("Ошибка получения заказа %d из БД: %v", id, err)
		return model.Order{}, err
	}

	if order.State != model.StateReturned && order.DeadlineAt.After(time.Now()) {
		if err := s.cache.SetOrder(ctx, order); err != nil {
			logger.Warnf("Ошибка кэширования заказа %d: %v", order.ID, err)
		} else {
			logger.Debugf("Заказ %d помещен в кэш", id)
		}
	}

	return order, nil
}

// ClearDatabase - очищает базу данных
func (s *OrderService) ClearDatabase(ctx context.Context) error {
	logger.Infof("Запрос на очистку базы данных заказов")

	orders, err := s.repo.List(ctx, "")
	if err != nil {
		logger.Errorf("Ошибка при получении списка заказов для очистки: %v", err)
		return fmt.Errorf("ошибка при получении списка заказов: %w", err)
	}

	logger.Infof("Найдено %d заказов для удаления", len(orders))

	for _, order := range orders {
		if err := s.repo.Delete(ctx, order.ID); err != nil {
			logger.Errorf("Ошибка при удалении заказа %d: %v", order.ID, err)
			return fmt.Errorf("ошибка при удалении заказа %d: %w", order.ID, err)
		}
		logger.Debugf("Заказ %d удален из БД", order.ID)
	}

	if err := s.cache.ClearOrderCache(ctx); err != nil {
		logger.Errorf("Ошибка при очистке кэша заказов: %v", err)
		return fmt.Errorf("ошибка при очистке кэша: %w", err)
	}

	logger.Infof("База данных заказов успешно очищена, удалено %d заказов", len(orders))
	return nil
}

// ListOrdersWithCursor - возвращает список заказов с использованием курсорной пагинации по ID
func (s *OrderService) ListOrdersWithCursor(ctx context.Context, cursorID int64, limit int, customerID int64, filterPVZ bool, searchTerm string) ([]model.Order, error) {
	logger.Debugf("Запрос списка заказов с курсором: cursorID=%d, limit=%d, customerID=%d, filterPVZ=%v, searchTerm=%s",
		cursorID, limit, customerID, filterPVZ, searchTerm)

	orders, err := s.repo.ListWithCursor(ctx, cursorID, limit, customerID, filterPVZ, searchTerm)
	if err != nil {
		logger.Errorf("Ошибка получения списка заказов с курсором: %v", err)
	} else {
		logger.Debugf("Получено %d заказов с использованием курсорной пагинации", len(orders))
	}

	return orders, err
}

// ListReturnsWithCursor - возвращает список возвращенных заказов с использованием курсорной пагинации по ID
func (s *OrderService) ListReturnsWithCursor(ctx context.Context, cursorID int64, limit int, searchTerm string) ([]model.Order, error) {
	logger.Debugf("Запрос списка возвратов с курсором: cursorID=%d, limit=%d, searchTerm=%s",
		cursorID, limit, searchTerm)

	returns, err := s.repo.ListReturnsWithCursor(ctx, cursorID, limit, searchTerm)
	if err != nil {
		logger.Errorf("Ошибка получения списка возвратов с курсором: %v", err)
	} else {
		logger.Debugf("Получено %d возвращенных заказов с использованием курсорной пагинации", len(returns))
	}

	return returns, err
}

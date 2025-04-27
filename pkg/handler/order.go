package handler

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"gitlab.ozon.dev/gojhw1/pkg/logger"
	"gitlab.ozon.dev/gojhw1/pkg/model"
)

// orderRequest описывает структуру запроса для создания нового заказа
type orderRequest struct {
	ID          int64   `json:"id"`
	CustomerID  int64   `json:"customer_id"`
	DeadlineAt  string  `json:"deadline_at"`
	Weight      float64 `json:"weight"`
	Cost        float64 `json:"cost"`
	PackageType string  `json:"package_type,omitempty"`
	Wrapper     string  `json:"wrapper,omitempty"`
}

// processRequest описывает структуру запроса для обработки заказов
type processRequest struct {
	CustomerID int64   `json:"customer_id"`
	Action     string  `json:"action"`
	OrderIDs   []int64 `json:"order_ids"`
}

// orderServiceInterface описывает интерфейс сервиса для работы с заказами
type orderServiceInterface interface {
	AcceptOrder(ctx context.Context, id, customerID int64, deadline time.Time, weight, cost float64, packageType *model.PackageType, wrapper *model.WrapperType) error
	ReturnOrderToCourier(ctx context.Context, id int64) error
	DeliverOrder(ctx context.Context, id, customerID int64, now time.Time) error
	ProcessReturnOrder(ctx context.Context, id, customerID int64, now time.Time) error
	OrderHistory(ctx context.Context, searchTerm string) ([]model.Order, error)
	AcceptOrdersFromFile(ctx context.Context, filename string) error
	GetOrderByID(ctx context.Context, id int64) (model.Order, error)
	ClearDatabase(ctx context.Context) error
	ListOrdersWithCursor(ctx context.Context, cursorID int64, limit int, customerID int64, filterPVZ bool, searchTerm string) ([]model.Order, error)
	ListReturnsWithCursor(ctx context.Context, cursorID int64, limit int, searchTerm string) ([]model.Order, error)
}

// OrderHandler - Обработчик запросов для заказов
type OrderHandler struct {
	service orderServiceInterface
}

// NewOrderHandler создает новый экземпляр обработчика заказов.
// Принимает сервис для работы с заказами.
func NewOrderHandler(service orderServiceInterface) *OrderHandler {
	return &OrderHandler{service: service}
}

// CreateOrder обрабатывает запрос на создание нового заказа.
// Принимает JSON с данными заказа и сохраняет его в системе.
// Возвращает созданный заказ или ошибку в случае неудачи.
func (h *OrderHandler) CreateOrder(c *fiber.Ctx) error {
	// Получаем контекст из запроса
	ctx := c.UserContext()

	var req orderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("Ошибка при разборе запроса: %v", err),
		})
	}

	deadline, packageType, wrapper, err := validateOrderRequest(req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if err = h.service.AcceptOrder(
		ctx,
		req.ID,
		req.CustomerID,
		deadline,
		req.Weight,
		req.Cost,
		packageType,
		wrapper,
	); err != nil {
		status, msg := processError(err)
		return c.Status(status).JSON(fiber.Map{
			"error": fmt.Sprintf("Ошибка при принятии заказа: %v", msg),
		})
	}

	order, err := h.service.GetOrderByID(ctx, req.ID)
	if err != nil {
		status, msg := processError(err)
		return c.Status(status).JSON(fiber.Map{
			"error": fmt.Sprintf("Ошибка при попытке получить сохраненный заказ: %v", msg),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(order)
}

// GetOrder обрабатывает запрос на получение информации о заказе по его ID.
// Возвращает детальную информацию о заказе или ошибку, если заказ не найден.
func (h *OrderHandler) GetOrder(c *fiber.Ctx) error {
	ctx := c.UserContext()

	orderID, err := parseOrderIDFromString(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	order, err := h.service.GetOrderByID(ctx, orderID)
	if err != nil {
		status, msg := processError(err)
		return c.Status(status).JSON(fiber.Map{
			"error": fmt.Sprintf("Ошибка при получении заказа: %v", msg),
		})
	}

	return c.Status(fiber.StatusOK).JSON(order)
}

// ReturnToCourier обрабатывает запрос на возврат заказа курьеру.
// Изменяет статус заказа и регистрирует операцию возврата.
func (h *OrderHandler) ReturnToCourier(c *fiber.Ctx) error {
	ctx := c.UserContext()

	orderID, err := parseOrderIDFromString(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if err = h.service.ReturnOrderToCourier(ctx, orderID); err != nil {
		status, msg := processError(err)
		return c.Status(status).JSON(fiber.Map{
			"error": fmt.Sprintf("Ошибка при возврате заказа курьеру: %v", msg),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": fmt.Sprintf("Заказ возвращен курьеру %d", orderID),
	})
}

// ProcessCustomer обрабатывает запрос на выполнение действий с заказами для указанного клиента.
// Поддерживает действия "handout" (выдача) и "return" (возврат).
func (h *OrderHandler) ProcessCustomer(c *fiber.Ctx) error {
	ctx := c.UserContext()

	var req processRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("Ошибка при разборе запроса: %v", err),
		})
	}

	if err := validateProcessRequest(req.Action, req.CustomerID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	now := time.Now()
	results := make([]map[string]any, 0, len(req.OrderIDs))

	for _, orderID := range req.OrderIDs {
		var err error

		switch req.Action {
		case "handout":
			err = h.service.DeliverOrder(ctx, orderID, req.CustomerID, now)
		case "return":
			err = h.service.ProcessReturnOrder(ctx, orderID, req.CustomerID, now)
		}

		if err != nil {
			status, msg := processError(err)
			results = append(results, map[string]any{
				"order_id": orderID,
				"error":    msg,
				"status":   status,
			})
		} else {
			results = append(results, map[string]any{
				"order_id": orderID,
				"message":  fmt.Sprintf("Заказ ID %d %s клиенту %d", orderID, req.Action, req.CustomerID),
				"status":   fiber.StatusOK,
			})
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"results": results,
	})
}

// ListOrders обрабатывает запрос на получение списка заказов с курсорной пагинацией по ID.
func (h *OrderHandler) ListOrders(c *fiber.Ctx) error {
	ctx := c.UserContext()

	// Парсим параметры запроса
	customerID, err := parseCustomerIDFromString(c.Query("customer_id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	cursorID, err := parseCursorFromString(c.Query("cursor"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	limit, err := parseLimitFromString(c.Query("limit"), defaultPageSize)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	filterPVZ := c.Query("pvz") == "true"
	searchTerm := c.Query("search", "")

	// Получаем заказы с использованием курсорной пагинации
	orders, err := h.service.ListOrdersWithCursor(ctx, cursorID, limit+1, customerID, filterPVZ, searchTerm)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Ошибка при получении заказов: %v", err),
		})
	}

	// Формируем ответ с пагинацией
	hasMore := len(orders) > limit
	var nextCursor string

	if hasMore {
		orders = orders[:limit]
	}

	if len(orders) > 0 {
		nextCursor = strconv.FormatInt(orders[len(orders)-1].ID, 10)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"orders":      orders,
		"has_more":    hasMore,
		"next_cursor": nextCursor,
	})
}

// ListReturns обрабатывает запрос на получение списка возвращенных заказов с курсорной пагинацией по ID.
func (h *OrderHandler) ListReturns(c *fiber.Ctx) error {
	ctx := c.UserContext()

	// Получение параметров с использованием новых функций
	cursorID, err := parseCursorFromString(c.Query("cursor"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	limit, err := parseLimitFromString(c.Query("limit"), defaultPageSize)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	searchTerm := c.Query("search", "")

	// Получаем возвраты с использованием курсорной пагинации
	returns, err := h.service.ListReturnsWithCursor(ctx, cursorID, limit+1, searchTerm)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Ошибка при получении возвратов: %v", err),
		})
	}

	// Проверяем, есть ли следующая страница
	hasMore := len(returns) > limit
	var nextCursor string

	if hasMore {
		// Удаляем дополнительную запись, которая нужна только для определения наличия следующей страницы
		returns = returns[:limit]
	}

	// Если у нас есть записи, устанавливаем курсор на ID последнего элемента текущего списка
	if len(returns) > 0 {
		nextCursor = strconv.FormatInt(returns[len(returns)-1].ID, 10)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"returns":     returns,
		"has_more":    hasMore,
		"next_cursor": nextCursor,
	})
}

// OrderHistory обрабатывает запрос на получение истории всех заказов.
// Возвращает полный список заказов с их текущими статусами.
func (h *OrderHandler) OrderHistory(c *fiber.Ctx) error {
	ctx := c.UserContext()
	searchTerm := c.Query("search", "")

	orders, err := h.service.OrderHistory(ctx, searchTerm)
	if err != nil {
		status, msg := processError(err)
		return c.Status(status).JSON(fiber.Map{
			"error": fmt.Sprintf("Ошибка при получении истории заказов: %v", msg),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"orders": orders,
		"total":  len(orders),
	})
}

// AcceptOrdersFromFile обрабатывает запрос на загрузку заказов из файла.
// Принимает файл с данными заказов, обрабатывает его и создает заказы в системе.
func (h *OrderHandler) AcceptOrdersFromFile(c *fiber.Ctx) error {
	ctx := c.UserContext()

	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("Ошибка при получении файла: %v", err),
		})
	}

	tempDir := "temp"
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		if err := os.Mkdir(tempDir, 0755); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": fmt.Sprintf("Ошибка при создании временной директории: %v", err),
			})
		}
	}

	tempFilePath := fmt.Sprintf("%s/%s", tempDir, file.Filename)
	if err = c.SaveFile(file, tempFilePath); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Ошибка при сохранении файла: %v", err),
		})
	}

	defer func() {
		if removeErr := os.Remove(tempFilePath); removeErr != nil {
			logger.Warnf("Ошибка при удалении временного файла %s: %v\n", tempFilePath, removeErr)
		}

		if removeErr := os.Remove(tempDir); removeErr != nil {
			logger.Warnf("Ошибка при удалении временной директории %s: %v\n", tempDir, removeErr)
		}
	}()

	if err = h.service.AcceptOrdersFromFile(ctx, tempFilePath); err != nil {
		status, msg := processError(err)
		return c.Status(status).JSON(fiber.Map{
			"error": fmt.Sprintf("Ошибка при обработке файла: %v", msg),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Заказы успешно обработаны",
	})
}

// ClearDatabase обрабатывает запрос на очистку базы данных.
// Удаляет все заказы и связанные с ними данные из системы.
func (h *OrderHandler) ClearDatabase(c *fiber.Ctx) error {
	ctx := c.UserContext()

	if err := h.service.ClearDatabase(ctx); err != nil {
		status, msg := processError(err)
		return c.Status(status).JSON(fiber.Map{
			"error": fmt.Sprintf("Ошибка при очистке базы данных: %v", msg),
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "База данных успешно очищена",
	})
}

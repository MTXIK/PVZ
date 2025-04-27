package handler

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"gitlab.ozon.dev/gojhw1/pkg/cache"
	"gitlab.ozon.dev/gojhw1/pkg/model"
	"gitlab.ozon.dev/gojhw1/pkg/repository"
	"gitlab.ozon.dev/gojhw1/pkg/service"
)

var (
	// ErrNegativeWeight возникает, когда указанный вес меньше или равен 0
	ErrNegativeWeight = errors.New("вес должен быть больше 0")
	// ErrNegativeCost возникает, когда указанная стоимость меньше или равна 0
	ErrNegativeCost = errors.New("стоимость должна быть больше 0")
	// ErrInvalidAction возникает при попытке выполнить неподдерживаемое действие
	ErrInvalidAction = errors.New("неизвестное действие")
	// ErrWrongDeadline возникает при некорректном формате дедлайна
	ErrWrongDeadline = errors.New("неправильный формат дедлайна")
	// ErrEmptyUsername возникает при попытке создать пользователя с пустым именем
	ErrEmptyUsername = errors.New("имя пользователя не может быть пустым")
	// ErrEmptyPassword возникает при попытке использовать пустой пароль
	ErrEmptyPassword = errors.New("пароль не может быть пустым")
	// ErrInvalidUserID возникает при передаче некорректного идентификатора пользователя
	ErrInvalidUserID = errors.New("неверный формат ID пользователя")
	// ErrUserIDMustBePositive возникает, когда ID пользователя не является положительным числом
	ErrUserIDMustBePositive = errors.New("ID пользователя должен быть положительным числом")
)

const timeLayout = "2006-01-02T15:04:05"
const defaultPageSize = 5

// parseDeadline - Парсинг дедлайна из строки
func parseDeadline(deadlineStr string) (time.Time, error) {
	if dur, err := time.ParseDuration(deadlineStr); err == nil {
		return time.Now().Add(dur), nil
	}

	deadline, err := time.Parse(timeLayout, deadlineStr)
	if err != nil {
		return time.Time{}, ErrWrongDeadline
	}
	return deadline, nil
}

// processError - Обработка ошибок и возврат соответствующего кода состояния
func processError(err error) (int, string) {
	switch {
	// Bad Request errors
	case errors.Is(err, service.ErrStorageDeadlinePassed),
		errors.Is(err, service.ErrDeadlineNotExpired),
		errors.Is(err, service.ErrNotDelivered),
		errors.Is(err, service.ErrOpenFile),
		errors.Is(err, service.ErrReadFile),
		errors.Is(err, service.ErrParseFile),
		errors.Is(err, service.ErrInvalidDateFormat),
		errors.Is(err, service.ErrNegativeWeight),
		errors.Is(err, service.ErrInvalidOrderID),
		errors.Is(err, service.ErrPackageWeightExceeded),
		errors.Is(err, service.ErrUnknownPackageType),
		errors.Is(err, service.ErrUnknownWrapperType),
		errors.Is(err, service.ErrNegativeCost):
		return fiber.StatusBadRequest, err.Error()

	// Conflict errors
	case errors.Is(err, service.ErrOrderExists),
		errors.Is(err, service.ErrOrderAlreadyDelivered),
		errors.Is(err, service.ErrWrongState):
		return fiber.StatusConflict, err.Error()

	// Forbidden errors
	case errors.Is(err, service.ErrWrongCustomer):
		return fiber.StatusForbidden, err.Error()

	// Gone errors
	case errors.Is(err, service.ErrStorageExpired),
		errors.Is(err, service.ErrReturnExpired),
		errors.Is(err, cache.ErrOrderExpired),
		errors.Is(err, cache.ErrOrderNotCached):
		return fiber.StatusGone, err.Error()

	// Not Found errors
	case errors.Is(err, repository.ErrOrdersNotFound),
		errors.Is(err, repository.ErrOrderNotFound),
		errors.Is(err, cache.ErrOrderNotFoundInCache),
		errors.Is(err, cache.ErrHistoryNotFoundInCache):
		return fiber.StatusNotFound, err.Error()

	// Default case for unhandled errors
	default:
		return fiber.StatusInternalServerError, err.Error()
	}
}

// parseCursorFromString извлекает и валидирует ID курсора из строки
func parseCursorFromString(cursorStr string) (int64, error) {
	if cursorStr == "" {
		return 0, nil
	}

	cursorID, err := strconv.ParseInt(cursorStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("неверный формат курсора: %v", err)
	}

	if cursorID < 0 {
		return 0, fmt.Errorf("ID курсора не может быть отрицательным")
	}

	return cursorID, nil
}

// parseLimitFromString извлекает и валидирует параметр limit из строки
func parseLimitFromString(limitStr string, defaultSize int) (int, error) {
	if limitStr == "" {
		return defaultSize, nil
	}

	parsedLimit, err := strconv.Atoi(limitStr)
	if err != nil {
		return 0, fmt.Errorf("неверный формат параметра limit")
	}

	if parsedLimit < 1 || parsedLimit > 100 {
		return 0, fmt.Errorf("значение limit должно быть от 1 до 100")
	}

	return parsedLimit, nil
}

// parseCustomerIDFromString извлекает и валидирует ID клиента из строки
func parseCustomerIDFromString(customerIDStr string) (int64, error) {
	if customerIDStr == "" {
		return 0, fmt.Errorf("ID клиента не указан")
	}

	customerID, err := strconv.ParseInt(customerIDStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("неверный формат ID клиента: %v", err)
	}

	if customerID <= 0 {
		return 0, fmt.Errorf("ID клиента должен быть больше 0")
	}

	return customerID, nil
}

// parseOrderIDFromString извлекает и валидирует ID заказа из строки
func parseOrderIDFromString(orderIDStr string) (int64, error) {
	if orderIDStr == "" {
		return 0, fmt.Errorf("ID заказа не указан")
	}

	orderID, err := strconv.ParseInt(orderIDStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("неверный формат ID заказа")
	}

	if orderID <= 0 {
		return 0, fmt.Errorf("ID заказа должен быть больше 0")
	}

	return orderID, nil
}

// validateOrderRequest проверяет корректность данных в запросе на создание заказа
func validateOrderRequest(req orderRequest) (time.Time, *model.PackageType, *model.WrapperType, error) {
	// Валидация веса
	if req.Weight <= 0 {
		return time.Time{}, nil, nil, ErrNegativeWeight
	}

	// Валидация стоимости
	if req.Cost <= 0 {
		return time.Time{}, nil, nil, ErrNegativeCost
	}

	// Валидация и парсинг даты (используем уже существующую функцию)
	deadline, err := parseDeadline(req.DeadlineAt)
	if err != nil {
		return time.Time{}, nil, nil, err
	}

	// Обработка типа упаковки и обертки
	var packageType *model.PackageType
	var wrapper *model.WrapperType

	if req.PackageType != "" {
		pt := model.PackageType(req.PackageType)
		packageType = &pt

		if req.Wrapper != "" {
			wt := model.WrapperType(req.Wrapper)
			wrapper = &wt
		}
	}

	return deadline, packageType, wrapper, nil
}

// validateProcessRequest проверяет корректность запроса на обработку заказа клиента
func validateProcessRequest(action string, id int64) error {
	if id <= 0 {
		return ErrInvalidUserID
	}

	if action != "handout" && action != "return" {
		return ErrInvalidAction
	}

	return nil
}

// validateCreateUserRequest проверяет корректность запроса на создание пользователя
func validateCreateUserRequest(req сreateUserRequest) error {
	if req.Username == "" {
		return ErrEmptyUsername
	}

	if req.Password == "" {
		return ErrEmptyPassword
	}

	return nil
}

// parseUserIDFromParams извлекает и валидирует ID пользователя из параметров запроса
func parseUserIDFromParams(idParam string) (int64, error) {
	if idParam == "" {
		return 0, ErrInvalidUserID
	}

	id, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil {
		return 0, ErrInvalidUserID
	}

	if id <= 0 {
		return 0, ErrUserIDMustBePositive
	}

	return id, nil
}

// validatePasswordRequest проверяет корректность запроса на обновление пароля
func validatePasswordRequest(password string) error {
	if password == "" {
		return ErrEmptyPassword
	}

	return nil
}

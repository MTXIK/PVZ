package router

import (
	"context"
	"time"

	"github.com/gofiber/contrib/otelfiber"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/basicauth"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gitlab.ozon.dev/gojhw1/pkg/handler"
	"gitlab.ozon.dev/gojhw1/pkg/model"
)

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

type userRepository interface {
	Create(ctx context.Context, user model.User, plainPassword string) error
	Update(ctx context.Context, user model.User) error
	UpdatePassword(ctx context.Context, userID int64, newPassword string) error
	Delete(ctx context.Context, id int64) error
	GetByID(ctx context.Context, id int64) (model.User, error)
	GetByUsername(ctx context.Context, username string) (model.User, error)
	List(ctx context.Context, searchTerm string) ([]model.User, error)
	CheckPassword(ctx context.Context, username, password string) bool
}

type auditLoggerInterface interface {
	Log(ctx context.Context, log model.AuditLog)
	LogOrderStatusChange(ctx context.Context, orderID int64, oldStatus, newStatus string)
	Shutdown()
}

// InitFiberApp инициализирует экземпляр приложения Fiber
func InitFiberApp(ctx context.Context, orderService orderServiceInterface, userRepo userRepository, auditLogger auditLoggerInterface) *fiber.App {

	// Создание экземпляра Fiber
	app := fiber.New(fiber.Config{
		AppName:               "PVZ API",
		DisableStartupMessage: false,
		IdleTimeout:           10 * time.Second,
		ReadTimeout:           30 * time.Second,
		WriteTimeout:          30 * time.Second,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			// Централизованная обработка ошибок
			code := fiber.StatusInternalServerError

			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}

			return c.Status(code).JSON(fiber.Map{
				"status":  "error",
				"message": err.Error(),
				"code":    code,
			})
		},
	})

	// Подключение middleware
	app.Use(recover.New()) // Восстановление после паники

	app.Use(MetricsMiddleware())

	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	// Настройка обработчиков
	orderHandler := handler.NewOrderHandler(orderService)
	userHandler := handler.NewUserHandler(userRepo)

	// Регистрация публичных маршрутов для пользователей (без аутентификации)
	app.Post("/api/v1/users/register", userHandler.CreateUser)

	// Настройка middleware для Basic Authentication
	authConfig := basicauth.Config{
		Realm: "Restricted Area",
		Authorizer: func(username, password string) bool {
			return userRepo.CheckPassword(ctx, username, password)
		},
		Unauthorized: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Требуется авторизация",
			})
		},
		ContextUsername: "username",
	}

	// Применяем Basic Auth middleware ко всем защищенным API маршрутам
	api := app.Group("/api/v1", basicauth.New(authConfig))
	api.Use(AuditMiddleware(auditLogger))
	api.Use(otelfiber.Middleware(otelfiber.WithServerName("pvz-app")))

	// Регистрация защищенных маршрутов для пользователей
	users := api.Group("/users")
	users.Get("/", userHandler.ListUsers)
	users.Get("/:id", userHandler.GetUser)
	users.Put("/:id", userHandler.UpdateUser)
	users.Delete("/:id", userHandler.DeleteUser)
	users.Put("/:id/password", userHandler.UpdatePassword)

	// Регистрация защищенных маршрутов для заказов
	orders := api.Group("/orders")
	orders.Post("/", orderHandler.CreateOrder)
	orders.Get("/", orderHandler.ListOrders)
	orders.Get("/history", orderHandler.OrderHistory)
	orders.Post("/accept", orderHandler.AcceptOrdersFromFile)
	orders.Get("/:id", orderHandler.GetOrder)
	orders.Delete("/:id/return", orderHandler.ReturnToCourier)
	orders.Put("/:id/process", orderHandler.ProcessCustomer)

	// Маршрут для возвратов
	returns := api.Group("/returns")
	returns.Get("/", orderHandler.ListReturns)

	// Маршрут для операций с базой данных
	db := api.Group("/db")
	db.Delete("/", orderHandler.ClearDatabase)

	return app
}

classDiagram
    class Order {
        -ID int64
        -CustomerID int64
        -State OrderState
        -Weight float64
        -Cost float64
        -PackageType PackageType
        -Wrapper WrapperType
        -DeadlineAt time.Time
        -UpdatedAt time.Time
        -DeliveredAt *time.Time
        -ReturnedAt *time.Time
    }

    class Repository {
        <<interface>>
        +Add(order Order) error
        +Update(order Order) error
        +Delete(id int64) error
        +FindByID(id int64) Order
        +List() []Order
        +SetAll(orders map[int64]Order)
        +GetAll() map[int64]Order
    }

    class InMemoryRepository {
        -orders map[int64]Order
        +Add(order Order) error
        +Update(order Order) error
        +Delete(id int64) error
        +FindByID(id int64) Order
        +List() []Order
        +SetAll(orders map[int64]Order)
        +GetAll() map[int64]Order
    }

    class OrderStorage {
        <<interface>>
        +Save(orders map[int64]Order) error
        +Load() (map[int64]Order, error)
    }

    class JSONStorage {
        -FilePath string
        +Save(orders map[int64]Order) error
        +Load() (map[int64]Order, error)
    }

    %% Сервисный слой
    class OrderService {
        -repo Repository
        +AcceptOrder(id, customerID, deadline, weight, cost, packageType, wrapper) error
        +ReturnOrderToCourier(id int64) error
        +DeliverOrder(id, customerID int64, now time.Time) error
        +ProcessReturnOrder(id, customerID int64, now time.Time) error
        +OrderHistory() []Order
        +ListReturns() []Order
        +ListOrders(customerID int64, lastN int, filterPVZ bool) []Order
    }

    %% Слой обработки команд
    class CommandsHandler {
        -service OrderService
        -storage OrderStorage
        -commands map[string]CommandFunc
        +Execute(command string, args []string) error
    }

    %% Слой ввода
    class InputHandler {
        -Terminal readline.Instance
        +ReadLine() (string, error)
        +ProcessLine(line string) (string, []string)
        +Close()
    }

    %% Основное приложение
    class App {
        -inputHandler InputHandler
        -cmdHandler CommandsHandler
        +StartAndWatch() error
        +Close()
    }

    %% Модель упаковки
    class Packager {
        <<interface>>
        +ValidateWeight(weight float64) error
        +GetAdditionalCost() float64
        +GetDescription() string
    }

    class BasicPackager {
        -description string
        -maxWeight float64
        -cost float64
        +ValidateWeight(weight float64) error
        +GetAdditionalCost() float64
        +GetDescription() string
    }

    class BagPackager {
        +NewBagPackager() *BagPackager
    }

    class BoxPackager {
        +NewBoxPackager() *BoxPackager
    }

    class FilmPackager {
        +NewFilmPackager() *FilmPackager
    }

    class WrapperDecorator {
        -packager Packager
        -description string
        -cost float64
        +ValidateWeight(weight float64) error
        +GetAdditionalCost() float64
        +GetDescription() string
    }

    class PackagerFactory {
        <<interface>>
        +CreatePackager(baseType *PackageType, wrapper *WrapperType) (Packager, error)
    }

    class DefaultPackagerFactory {
        +CreatePackager(baseType *PackageType, wrapper *WrapperType) (Packager, error)
    }

   %% Основные компоненты приложения
    App *-- CommandsHandler
    App *-- InputHandler


    %% Обработчик команд и его зависимости
    CommandsHandler *-- OrderService
    CommandsHandler *-- OrderStorage

    %% Хранилище и репозиторий
    JSONStorage ..|> OrderStorage
    OrderStorage --> Order : uses
    InMemoryRepository ..|> Repository
    Repository --> Order : uses

    %% Фабрика упаковщиков
    PackagerFactory <|.. DefaultPackagerFactory
    DefaultPackagerFactory ..> Packager

    %% Сервисный слой
    OrderService *-- Repository
    OrderService --> Order : uses
    OrderService --> PackagerFactory

    %% Иерархия упаковщиков
    Packager <|.. BasicPackager
    BasicPackager <|-- BagPackager
    BasicPackager <|-- BoxPackager
    BasicPackager <|-- FilmPackager

    %% Декоратор упаковщика
    Packager <|.. WrapperDecorator
    WrapperDecorator o-- Packager
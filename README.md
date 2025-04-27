# Менеджер ПВЗ (Пункт Выдачи Заказов)

REST API для управления пунктом выдачи заказов. Позволяет принимать заказы от курьеров, выдавать их клиентам и обрабатывать возвраты.

## Возможности

- Прием заказов от курьера (поштучно или из JSON-файла)
- Возврат заказов курьеру
- Выдача заказов клиентам
- Прием возвратов от клиентов
- Просмотр списка заказов с фильтрацией и поиском
- Просмотр списка возвратов с пагинацией и поиском
- Просмотр истории заказов с возможностью поиска
- Хранение данных в PostgreSQL

## API Эндпоинты

### Пользователи

#### Регистрация нового пользователя

```bash
curl -X POST http://localhost:9000/api/v1/users/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "newuser",
    "password": "password123",
    "role": "user"
  }'
```

**Параметры:**

- `username` - имя пользователя (обязательно)
- `password` - пароль (обязательно)
- `role` - роль пользователя (`admin` или `user`)

#### Получение списка пользователей

```bash
curl -X GET http://localhost:9000/api/v1/users \
  -u "admin:admin"
```

**Параметры запроса:**

- `search` - строка для поиска по имени пользователя или ID (опционально)

#### Получение информации о пользователе

```bash
curl -X GET http://localhost:9000/api/v1/users/1 \
  -u "admin:admin"
```

**Параметры пути:**

- `id` - идентификатор пользователя

#### Обновление информации о пользователе

```bash
curl -X PUT http://localhost:9000/api/v1/users/1 \
  -u "admin:admin" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "updated_user",
    "role": "admin"
  }'
```

**Параметры пути:**

- `id` - идентификатор пользователя

**Параметры запроса:**

- `username` - новое имя пользователя
- `role` - новая роль пользователя

#### Обновление пароля пользователя

```bash
curl -X PUT http://localhost:9000/api/v1/users/1/password \
  -u "admin:admin" \
  -H "Content-Type: application/json" \
  -d '{
    "password": "new_password123"
  }'
```

**Параметры пути:**

- `id` - идентификатор пользователя

**Параметры запроса:**

- `password` - новый пароль

#### Удаление пользователя

```bash
curl -X DELETE http://localhost:9000/api/v1/users/2 \
  -u "admin:admin"
```

**Параметры пути:**

- `id` - идентификатор пользователя

### Заказы

#### Создание нового заказа

```bash
curl -X POST http://localhost:9000/api/v1/orders \
  -u "admin:admin" \
  -H "Content-Type: application/json" \
  -d '{
    "id": 1,
    "customer_id": 1,
    "deadline_at": "2030-02-20T15:04:05",
    "weight": 5.0,
    "cost": 100.0,
    "package_type": "box",
    "wrapper": "film"
  }'
```

**Параметры запроса:**

- `id` - идентификатор заказа (обязательно)
- `customer_id` - идентификатор клиента (обязательно)
- `deadline_at` - срок выполнения заказа (формат ISO 8601)
- `weight` - вес заказа (должен быть больше 0)
- `cost` - стоимость (должна быть больше 0)
- `package_type` - тип упаковки (необязательно)
- `wrapper` - тип обёртки (необязательно)

#### Получение информации о заказе

```bash
curl -X GET http://localhost:9000/api/v1/orders/1 \
  -u "admin:admin"
```

**Параметры пути:**

- `id` - идентификатор заказа

#### Возврат заказа курьеру

```bash
curl -X DELETE http://localhost:9000/api/v1/orders/1/return \
  -u "admin:admin"
```

**Параметры пути:**

- `id` - идентификатор заказа

#### Обработка заказа для клиента (выдача или возврат)

```bash
curl -X PUT http://localhost:9000/api/v1/orders/1/process \
  -u "admin:admin" \
  -H "Content-Type: application/json" \
  -d '{
    "customer_id": 1,
    "action": "handout",
    "order_ids": [1, 2, 3]
  }'
```

**Параметры запроса:**

- `customer_id` - идентификатор клиента (обязательно)
- `action` - действие с заказом (`handout` - выдача, `return` - возврат)
- `order_ids` - массив идентификаторов заказов для обработки

#### Получение списка заказов

```bash
curl -X GET "http://localhost:9000/api/v1/orders?customer_id=1&limit=10" \
  -u "admin:admin"
```

**Параметры запроса:**

- `customer_id` - фильтр по идентификатору клиента (опционально)
- `limit` - количество записей на странице (от 1 до 100, по умолчанию 20)
- `cursor` - курсор для пагинации (ID заказа для начала выборки)
- `pvz` - фильтр по заказам в ПВЗ (если `true`)
- `search` - строка для поиска заказов по ID или ID клиента (опционально)

#### Получение списка возвратов

```bash
curl -X GET "http://localhost:9000/api/v1/returns?limit=10" \
  -u "admin:admin"
```

**Параметры запроса:**

- `limit` - количество записей на странице (от 1 до 100, по умолчанию 20)
- `cursor` - курсор для пагинации (ID заказа для начала выборки)
- `search` - строка для поиска возвратов по ID или ID клиента (опционально)

#### Получение истории заказов

```bash
curl -X GET http://localhost:9000/api/v1/orders/history \
  -u "admin:admin"
```

**Параметры запроса:**

- `search` - строка для поиска заказов по ID или ID клиента (опционально)

#### Загрузка заказов из файла

```bash
curl -X POST http://localhost:9000/api/v1/orders/accept \
  -u "admin:admin" \
  -F "file=@orders.json"
```

**Параметры формы:**

- `file` - JSON-файл с массивом заказов

> **Примечание**: В директории `/data` есть пример файла `example.json`, который можно использовать для тестирования загрузки заказов. Файл содержит 100 тестовых заказов с различными параметрами.

#### Очистка базы данных

```bash
curl -X DELETE http://localhost:9000/api/v1/db \
  -u "admin:admin"
```

## Формат JSON файла для импорта заказов

```json
[
  {
    "id": 1,
    "customer_id": 1,
    "deadline_at": "2030-02-20T15:04:05",
    "weight": 5.0,
    "cost": 100.0,
    "package_type": "box",
    "wrapper": "film"
  }
]
```

## gRPC API

Проект также предоставляет gRPC API для работы с пользователями и заказами.

### Запуск gRPC сервера

gRPC сервер запускается на порту 9001 (по умолчанию).

### Доступные сервисы

#### UserRPCHandler - Управление пользователями

- `CreateUser` - Создание нового пользователя
- `GetUser` - Получение информации о пользователе по ID
- `ListUsers` - Получение списка пользователей с возможностью поиска
- `UpdateUser` - Обновление информации о пользователе
- `UpdatePassword` - Обновление пароля пользователя
- `DeleteUser` - Удаление пользователя

#### OrderRPCHandler - Управление заказами

- `CreateOrder` - Создание нового заказа
- `GetOrder` - Получение информации о заказе по ID
- `ReturnToCourier` - Возврат заказа курьеру
- `ProcessCustomer` - Обработка действий с заказами для указанного клиента
- `ListOrders` - Получение списка заказов с курсорной пагинацией
- `ListReturns` - Получение списка возвращенных заказов с курсорной пагинацией
- `OrderHistory` - Получение истории всех заказов
- `AcceptOrdersFromFile` - Загрузка заказов из файла
- `ClearDatabase` - Очистка базы данных

### Примеры использования gRPC API с grpcurl

Для тестирования API можно использовать утилиту [grpcurl](https://github.com/fullstorydev/grpcurl)

#### Создание нового пользователя

```bash
grpcurl -plaintext -d '{"username": "newuser", "password": "password123", "role": "user"}' localhost:9001 proto.UserRPCHandler/CreateUser
```

#### Получение списка пользователей с аутентификацией

```bash
grpcurl -plaintext -H "Authorization: Basic $(echo -n 'admin:admin' | base64)" localhost:9001 proto.UserRPCHandler/ListUsers
```

#### Получение информации о пользователе по ID

```bash
grpcurl -plaintext -H "Authorization: Basic $(echo -n 'admin:admin' | base64)" -d '{"id": 1}' localhost:9001 proto.UserRPCHandler/GetUser
```

#### Создание нового заказа

```bash
grpcurl -plaintext -H "Authorization: Basic $(echo -n 'admin:admin' | base64)" -d '{"id": 1, "customer_id": 1, "deadline_at": "2030-02-20T15:04:05", "weight": 5.0, "cost": 100.0, "package_type": "PACKAGE_TYPE_BOX", "wrapper": "WRAPPER_TYPE_FILM"}' localhost:9001 proto.OrderRPCHandler/CreateOrder
```

#### Получение списка заказов с пагинацией

```bash
grpcurl -plaintext -H "Authorization: Basic $(echo -n 'admin:admin' | base64)" -d '{"cursor_id": 0, "limit": 10, "customer_id": 1}' localhost:9001 proto.OrderRPCHandler/ListOrders
```

#### Получение истории заказов

```bash
grpcurl -plaintext -H "Authorization: Basic $(echo -n 'admin:admin' | base64)" localhost:9001 proto.OrderRPCHandler/OrderHistory
```

### Аутентификация

gRPC API использует Basic Authentication. Для большинства методов требуется передавать заголовок авторизации:

```
Authorization: Basic <base64(username:password)>
```

Метод CreateUser (регистрация нового пользователя) доступен без аутентификации.

### Сгенерированные файлы Proto

Полное описание API доступно в Proto файлах в директории `/proto`:

- `user.proto` - Описание сервиса для работы с пользователями
- `order.proto` - Описание сервиса для работы с заказами

## Мониторинг с Prometheus и Grafana

Проект включает интеграцию с системами мониторинга Prometheus и Grafana для отслеживания работы сервиса.

### Доступные метрики

#### Бизнес метрики

- `pvz_orders_accepted_total` - общее количество принятых заказов
- `pvz_orders_delivered_total` - общее количество доставленных заказов клиентам
- `pvz_orders_returned_total` - общее количество возвращенных заказов
- `pvz_orders_returned_to_courier_total` - общее количество заказов, возвращенных курьеру
- `pvz_order_processing_seconds` - время обработки заказов (от принятия до доставки)

#### Технические метрики

- `http_requests_total` - общее количество HTTP-запросов с лейблами по методу, пути и статусу
- `http_request_duration_seconds` - длительность HTTP-запросов с лейблами по методу и пути

### Доступ к Prometheus и Grafana

После запуска проекта через Docker Compose:

- **Prometheus**: доступен по адресу [http://localhost:9090](http://localhost:9090)
- **Grafana**: доступна по адресу [http://localhost:3000](http://localhost:3000)
  - Логин: `admin`
  - Пароль: `admin`

### Готовые дашборды

В Grafana предустановлен дашборд для мониторинга сервиса ПВЗ со следующими панелями:

- Количество заказов (принятые, доставленные, возвращенные)
- Время обработки заказов (95-й процентиль и медиана)
- RPS по эндпоинтам API
- Статистика HTTP-ответов по кодам (2xx, 4xx, 5xx)
- Время ответа API (95-й процентиль)
- Графическая статистика по типам заказов

### Prometheus API

Вы можете напрямую обращаться к API Prometheus для получения метрик:

```bash
# Получение значения конкретной метрики
curl -G http://localhost:9090/metrics --data-urlencode "query=pvz_orders_accepted_total"

# Получение значений метрики за определенный период
curl -G http://localhost:9090/metrics \
  --data-urlencode "query=rate(pvz_orders_delivered_total[5m])" \
  --data-urlencode "start=2023-05-20T20:10:30.781Z" \
  --data-urlencode "end=2023-05-20T20:20:30.781Z" \
  --data-urlencode "step=15s"
```

## Трассировка с Jaeger

Проект интегрирован с Jaeger для распределенной трассировки запросов, что помогает отслеживать путь запроса через различные компоненты системы и диагностировать проблемы производительности.

### Интеграция

- Трассировка реализована с использованием OpenTelemetry (OTEL).
- Приложение отправляет трейсы в Jaeger Collector через OTLP HTTP протокол.
- Конфигурация Jaeger находится в `config.json` в секции `jaeger`.
- Сервис Jaeger запускается как часть Docker Compose стека (`compose.yml`).

### Доступ к Jaeger UI

После запуска проекта через Docker Compose:

- **Jaeger UI**: доступен по адресу [http://localhost:16686](http://localhost:16686)

В интерфейсе Jaeger можно:

- Выбрать сервис (`pvz-app` по умолчанию).
- Найти трейсы по различным параметрам (ID, теги, длительность).
- Визуализировать путь запроса и время, затраченное на каждом этапе.

## Makefile команды

- `make build` - сборка проекта с предварительной проверкой линтерами и форматированием
- `make run` - сборка и запуск приложения
- `make clean` - очистка артефактов сборки
- `make lint` - проверка кода линтерами (gocyclo и gocognit)
- `make fmt` - форматирование кода
- `make install-linters` - установка линтеров
- `make compose-up` - запуск всех Docker контейнеров
- `make compose-down` - остановка всех Docker контейнеров
- `make compose-down-vol` - остановка всех Docker контейнеров с удалением томов
- `make migrate-up` - применение миграций базы данных
- `make migrate-down` - откат миграций
- `make migrate-status` - проверка статуса миграций
- `make start` - полный запуск (БД + миграции + приложение)
- `make test-full` - запуск всех тестов
- `make test-unit` - запуск только юнит-тестов (исключая интеграционные)
- `make test-integration` - запуск всех интеграционных тестов
- `make test-integration-regular` - запуск обычных интеграционных тестов
- `make test-integration-suite` - запуск integration suite тестов
- `make mock` - генерация моков для тестирования
- `make cover` - генерация HTML-отчета о покрытии кода тестами
- `make proto` - генерация Go кода из proto-файлов

# Имя исполняемого файла
OUTPUT := PVZ

# Точка входа (путь к файлу main.go)
MAIN := cmd/main.go

# Порог сложности для линтеров
COMPLEXITY_THRESHOLD := 10

# Путь к установленным Go-бинарникам
GOPATH := $(shell go env GOPATH)
GOBIN := $(GOPATH)/bin

.PHONY: build run clean fmt lint install-linters compose-up compose-down compose-down-vol migrate-up migrate-down migrate-status start

# Запуск базы данных
compose-up:
	@echo "Запуск PostgreSQL..."
	@docker compose up -d

# Остановка базы данных
compose-down:
	@echo "Остановка PostgreSQL..."
	@docker compose down

compose-down-vol:
	@echo "Остановка PostgreSQL..."
	@docker compose down -v

# Установка линтеров
install-linters:
	@echo "Установка линтеров..."
	@go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
	@go install github.com/uudashr/gocognit/cmd/gocognit@latest

# Проверка линтерами
lint: install-linters
	@echo "\n"
	@echo "Проверка циклометрической сложности (gocyclo)..."
	@$(GOBIN)/gocyclo -over $(COMPLEXITY_THRESHOLD) . || true
	@echo "Проверка когнитивной сложности (gocognit)..."
	@$(GOBIN)/gocognit -over $(COMPLEXITY_THRESHOLD) . || true
	@echo "\n"

# Цель сборки: сначала линтинг и форматирование, затем создаём исполняемый файл
build: fmt #lint отключено
	@echo "Сборка проекта..."
	@go build -o $(OUTPUT) $(MAIN)

# Цель запуска: сначала сборка, затем запуск исполняемого файла
run: build
	@echo "Запуск проекта..."
	@./$(OUTPUT)

# Цель очистки: удаляет скомпилированный исполняемый файл
clean:
	@echo "Очистка проекта..."
	@rm -f $(OUTPUT)

# Форматирование кода
fmt:
	@echo "Форматирование кода..."
	@go fmt ./...

# Применение миграций
migrate-up:
	@echo "Применение миграций..."
	@go run cmd/migrate/main.go up

# Откат миграций
migrate-down:
	@echo "Откат миграций..."
	@go run cmd/migrate/main.go down

# Статус миграций
migrate-status:
	@echo "Статус миграций..."
	@go run cmd/migrate/main.go status

# Полный запуск (БД + миграции + приложение)
start: compose-up
	@echo "Ожидание готовности БД..."
	@sleep 2
	@$(MAKE) migrate-up
	@$(MAKE) run

.PHONY: test-full
test-full:
	@go test -v ./...

.PHONY: test-unit
test-unit:
	@echo "Запуск юнит-тестов..."
	@go test -short -v $(shell go list ./... | grep -v /tests/integration)


.PHONY: test-integration-suite
test-integration-suite: compose-up migrate-up
	@echo "Запуск integration suite тестов..."
	@go test -v ./tests/integration/suite_handler_test/...

.PHONY: test-integration-regular
test-integration-regular: compose-up migrate-up
	@echo "Запуск обычных integration тестов..."
	@go test -v ./tests/integration/handler_test/...

.PHONY: test-integration
test-integration: test-integration-regular test-integration-suite

.PHONY: mock
mock:
	@echo "Генерация моков..."
	@go install github.com/golang/mock/mockgen@latest
	@go generate ./...

.PHONY: cover
cover:
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out
	@rm coverage.out

.PHONY: cover-unit
cover-unit:
	@go test -short -coverprofile=coverage.out $(shell go list ./... | grep -v /tests/integration)
	@go tool cover -html=coverage.out
	@rm coverage.out

.PHONY: proto
proto:
	@echo "Генерация протофайлов..."
	@protoc --go_out=pkg/gen --go_opt=paths=source_relative \
        --go-grpc_out=pkg/gen --go-grpc_opt=paths=source_relative \
        proto/*.proto
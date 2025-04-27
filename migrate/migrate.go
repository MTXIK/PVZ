package migrate

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Run выполняет миграции базы данных
func Run(dbURL string, command string) error {
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return fmt.Errorf("не удалось открыть подключение к БД: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(migrationsFS)

	if err = goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("не удалось установить диалект: %w", err)
	}

	ctx := context.Background()
	if err = goose.RunContext(ctx, command, db, "migrations"); err != nil {
		return fmt.Errorf("ошибка выполнения миграций: %w", err)
	}

	return nil
}

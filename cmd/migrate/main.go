package main

import (
	"fmt"
	"log"
	"os"

	"gitlab.ozon.dev/gojhw1/migrate"
	"gitlab.ozon.dev/gojhw1/pkg/config"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Использование: migrate [up|down|status]")
		os.Exit(1)
	}

	command := os.Args[1]

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		cfg, err := config.Load("config.json")
		if err != nil {
			log.Fatalf("ошибка загрузки конфигурации: %v", err)
		}

		dbURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
			cfg.Database.User, cfg.Database.Password, cfg.Database.Host,
			cfg.Database.Port, cfg.Database.Name)
	}

	if err := migrate.Run(dbURL, command); err != nil {
		log.Fatalf("ошибка выполнения миграции: %v", err)
	}
}

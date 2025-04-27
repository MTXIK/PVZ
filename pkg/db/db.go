package db

import (
	"context"
	"fmt"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
)

// Pool представляет собой пул соединений с базой данных PostgreSQL
type Pool struct {
	*pgxpool.Pool
}

// New создает новый пул соединений с базой данных PostgreSQL
func New(ctx context.Context, dsn string) (*Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга URL базы данных: %w", err)
	}

	config.ConnConfig.Tracer = otelpgx.NewTracer(
		otelpgx.WithTrimSQLInSpanName(),                      // Опционально: обрезать SQL в имени спана
		otelpgx.WithTracerProvider(otel.GetTracerProvider()), // Используем глобальный провайдер
	)

	config.MaxConns = 100                    // Оптимальное количество соединений
	config.MinConns = 10                     // Поддерживаем минимальное число соединений
	config.MaxConnLifetime = 1 * time.Minute // Время жизни соединения
	config.MaxConnIdleTime = 5 * time.Minute // Время простоя

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("ошибка подключения к базе данных: %w", err)
	}

	if err = pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ошибка проверки соединения с базой данных: %w", err)
	}

	return &Pool{pool}, nil
}

// Close закрывает пул соединений
func (p *Pool) Close() {
	if p.Pool != nil {
		p.Pool.Close()
	}
}

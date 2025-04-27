package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Config - основная структура конфигурации приложения
type Config struct {
	Database   DatabaseConfig   `json:"database"`
	Server     ServerConfig     `json:"server"`
	Redis      RedisConfig      `json:"redis"`
	CacheType  CacheConfig      `json:"cache_type"`
	Kafka      KafkaConfig      `json:"kafka"`
	GrpcServer GrpcServerConfig `json:"grpc_server"`
	Logger     LoggerConfig     `json:"logger"`
	Jaeger     JaegerConfig     `json:"jaeger"`
}

// DatabaseConfig - конфигурация базы данных
type DatabaseConfig struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Name     string `json:"name"`
}

// ServerConfig - конфигурация HTTP-сервера
type ServerConfig struct {
	Host            string `json:"host"`
	Port            string `json:"port"`
	ReadTimeout     int    `json:"read_timeout"`     // в секундах
	WriteTimeout    int    `json:"write_timeout"`    // в секундах
	IdleTimeout     int    `json:"idle_timeout"`     // в секундах
	ShutdownTimeout int    `json:"shutdown_timeout"` // в секундах
}

// RedisConfig - конфигурация Redis
type RedisConfig struct {
	Host string `json:"host"`
	Port string `json:"port"`
}

// CacheConfig - конфигурация кэша
type CacheConfig struct {
	Name                   string `json:"name"`
	OrderKeyPrefix         string `json:"order_key_prefix,omitempty"`
	HistoryKeyPrefix       string `json:"history_key_prefix,omitempty"`
	OrderTTL               int    `json:"order_ttl"`                  // in minutes
	HistoryTTL             int    `json:"history_ttl"`                // in minutes
	MaxCacheSize           int    `json:"max_cache_size,omitempty"`   // in items
	AdditionalDuration     int    `json:"additional_duration"`        // in minutes
	CleanupInterval        int    `json:"cleanup_interval,omitempty"` // in minutes
	HistoryRefreshInterval int    `json:"history_refresh_interval"`   // in minutes
}

// KafkaConfig - конфигурация Kafka
type KafkaConfig struct {
	Brokers      []string `json:"brokers"`
	AuditTopic   string   `json:"audit_topic"`
	AuditGroupID string   `json:"audit_group_id"`
}

// GrpcServerConfig - конфигурация gRPC-сервера
type GrpcServerConfig struct {
	Port string `json:"port"`
	Host string `json:"host"`
}

// LoggerConfig - конфигурация логгера
type LoggerConfig struct {
	Level      string `json:"level"`
	OutputPath string `json:"output_path"`
	Encoding   string `json:"encoding"`
	DevMode    bool   `json:"dev_mode"`
}

type JaegerConfig struct {
	OtlpEndpoint string `json:"otlp_endpoint"`
	ServiceName  string `json:"service_name"`
}

// Load загружает конфигурацию из JSON-файла
func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("ошибка открытия файла конфигурации: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения файла конфигурации: %w", err)
	}

	var cfg Config
	if err = json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("ошибка декодирования файла конфигурации: %w", err)
	}

	// Устанавливаем значения по умолчанию, если они не определены
	setDefaults(&cfg)

	return &cfg, nil
}

// setDefaults устанавливает значения по умолчанию для параметров, которые не были заданы
func setDefaults(cfg *Config) {
	// Значения по умолчанию для сервера
	if cfg.Server.Host == "" {
		cfg.Server.Host = "127.0.0.1"
	}
	if cfg.Server.Port == "" {
		cfg.Server.Port = "9000"
	}
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 30 // 30 секунд
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 30 // 30 секунд
	}
	if cfg.Server.IdleTimeout == 0 {
		cfg.Server.IdleTimeout = 10 // 10 секунд
	}
	if cfg.Server.ShutdownTimeout == 0 {
		cfg.Server.ShutdownTimeout = 10 // 10 секунд
	}
	if cfg.CacheType.Name == "" {
		cfg.CacheType.Name = "inmem"
	}
}

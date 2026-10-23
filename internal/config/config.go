package config

import (
	"os"
	"strconv"
)

type Config struct {
	BaselinePort  int
	OptimizedPort int
	BaselineDSN   string
	OptimizedDSN  string
	RedisAddr     string
	NatsURL       string
	Environment   string
}

func Load() *Config {
	return &Config{
		BaselinePort:  getEnvAsInt("BASELINE_PORT", 8081),
		OptimizedPort: getEnvAsInt("OPTIMIZED_PORT", 8082),
		BaselineDSN:   getEnv("BASELINE_DSN", "postgres://apexstock:apexstock@postgres:5432/apexstock_baseline?sslmode=disable"),
		OptimizedDSN:  getEnv("OPTIMIZED_DSN", "postgres://apexstock:apexstock@postgres:5432/apexstock_optimized?sslmode=disable"),
		RedisAddr:     getEnv("REDIS_ADDR", "redis:6379"),
		NatsURL:       getEnv("NATS_URL", "nats://nats:4222"),
		Environment:   getEnv("ENVIRONMENT", "dev"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

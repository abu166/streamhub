package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Interval   time.Duration
	Workers    int
	PGHost     string
	PGPort     string
	PGUser     string
	PGPassword string
	PGDBName   string
}

func LoadConfig() *Config {
	intervalStr := os.Getenv("CLI_APP_TIMER_INTERVAL")
	if intervalStr == "" {
		intervalStr = "3m"
	}
	interval, _ := time.ParseDuration(intervalStr)

	workersStr := os.Getenv("CLI_APP_WORKERS_COUNT")
	if workersStr == "" {
		workersStr = "3"
	}
	workers, _ := strconv.Atoi(workersStr)

	return &Config{
		Interval:   interval,
		Workers:    workers,
		PGHost:     getEnv("POSTGRES_HOST", "localhost"),
		PGPort:     getEnv("POSTGRES_PORT", "5432"),
		PGUser:     getEnv("POSTGRES_USER", "postgres"),
		PGPassword: getEnv("POSTGRES_PASSWORD", "changem"),
		PGDBName:   getEnv("POSTGRES_DBNAME", "rsshub"),
	}
}

func getEnv(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}

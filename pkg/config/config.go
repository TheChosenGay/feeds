package config

import (
	"os"
	"strconv"
)

type Config struct {
	Postgres PostgresConfig
	MySQL    MySQLConfig
	Redis    RedisConfig
	Kafka    KafkaConfig
}

type PostgresConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
}

func (c PostgresConfig) DSN() string {
	return "host=" + c.Host +
		" port=" + strconv.Itoa(c.Port) +
		" user=" + c.User +
		" password=" + c.Password +
		" dbname=" + c.DBName +
		" sslmode=disable"
}

type MySQLConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
}

func (c MySQLConfig) DSN() string {
	return c.User + ":" + c.Password +
		"@tcp(" + c.Host + ":" + strconv.Itoa(c.Port) + ")/" +
		c.DBName + "?parseTime=true&charset=utf8mb4"
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type KafkaConfig struct {
	Brokers []string
}

func Load() *Config {
	return &Config{
		Postgres: PostgresConfig{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnvInt("POSTGRES_PORT", 5432),
			User:     getEnv("POSTGRES_USER", "feeds"),
			Password: getEnv("POSTGRES_PASSWORD", "feeds_dev"),
			DBName:   getEnv("POSTGRES_DB", "feeds"),
		},
		MySQL: MySQLConfig{
			Host:     getEnv("MYSQL_HOST", "localhost"),
			Port:     getEnvInt("MYSQL_PORT", 3306),
			User:     getEnv("MYSQL_USER", "feeds"),
			Password: getEnv("MYSQL_PASSWORD", "feeds_dev"),
			DBName:   getEnv("MYSQL_DB", "feeds"),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Kafka: KafkaConfig{
			Brokers: []string{getEnv("KAFKA_BROKERS", "localhost:9092")},
		},
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

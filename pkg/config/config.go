package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
)

type Config struct {
	Postgres PostgresConfig
	MySQL    MySQLConfig
	Redis    RedisConfig
	Kafka    KafkaConfig
	COS      COSConfig
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

// MigrateURL returns a postgres:// URL for golang-migrate.
func (c PostgresConfig) MigrateURL() string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   fmt.Sprintf("%s:%d", c.Host, c.Port),
		Path:   "/" + c.DBName,
	}
	q := u.Query()
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()
	return u.String()
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

type COSConfig struct {
	BucketURL string
	SecretID  string
	SecretKey string
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
		COS: COSConfig{
			BucketURL: getEnv("COS_BUCKET_URL", ""),
			SecretID:  getEnv("COS_SECRET_ID", ""),
			SecretKey: getEnv("COS_SECRET_KEY", ""),
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

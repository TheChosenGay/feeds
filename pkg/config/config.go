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
	Schema   string
}

func (c PostgresConfig) DSN() string {
	return "host=" + c.Host +
		" port=" + strconv.Itoa(c.Port) +
		" user=" + c.User +
		" password=" + c.Password +
		" dbname=" + c.DBName +
		" sslmode=disable" +
		" search_path=" + c.Schema
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

// Load reads config from environment variables. If service is non-empty, it is used
// as the PostgreSQL search_path schema (overriding POSTGRES_SCHEMA env).
func Load(service string) *Config {
	schema := service
	if schema == "" {
		schema = GetEnv("POSTGRES_SCHEMA", "public")
	}
	return &Config{
		Postgres: PostgresConfig{
			Host:     GetEnv("POSTGRES_HOST", "localhost"),
			Port:     GetEnvInt("POSTGRES_PORT", 5432),
			User:     GetEnv("POSTGRES_USER", "feeds"),
			Password: GetEnv("POSTGRES_PASSWORD", "feeds_dev"),
			DBName:   GetEnv("POSTGRES_DB", "feeds"),
			Schema:   schema,
		},
		MySQL: MySQLConfig{
			Host:     GetEnv("MYSQL_HOST", "localhost"),
			Port:     GetEnvInt("MYSQL_PORT", 3306),
			User:     GetEnv("MYSQL_USER", "feeds"),
			Password: GetEnv("MYSQL_PASSWORD", "feeds_dev"),
			DBName:   GetEnv("MYSQL_DB", "feeds"),
		},
		Redis: RedisConfig{
			Addr:     GetEnv("REDIS_ADDR", "localhost:6379"),
			Password: GetEnv("REDIS_PASSWORD", ""),
			DB:       GetEnvInt("REDIS_DB", 0),
		},
		Kafka: KafkaConfig{
			Brokers: []string{GetEnv("KAFKA_BROKERS", "localhost:9092")},
		},
		COS: COSConfig{
			BucketURL: GetEnv("COS_BUCKET_URL", ""),
			SecretID:  GetEnv("COS_SECRET_ID", ""),
			SecretKey: GetEnv("COS_SECRET_KEY", ""),
		},
	}
}

func GetEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func GetEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

package config

import "sync"

type Config struct {
	DBDriver    string `env:"DB_DRIVER"`
	DBHost      string `env:"DB_HOST"`
	DBUser      string `env:"DB_USER"`
	DBPassword  string `env:"DB_PASSWORD"`
	DBName      string `env:"DB_NAME"`
	DBPort      string `env:"DB_PORT"`
	DBPath      string `env:"DB_PATH"`
	Environment string `env:"ENV"`
	JWTSecret   string `env:"JWT_SECRET"`
	Port        string `env:"PORT"`
}

var (
	instance *Config
	once     sync.Once
)

// GetInstance returns a singleton instance of the application configuration.
func GetInstance() *Config {
	once.Do(func() {
		instance = &Config{}
	})
	return instance
}

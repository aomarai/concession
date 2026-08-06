package config

import (
	"context"

	"github.com/joho/godotenv"
	"github.com/sethvargo/go-envconfig"
)

type Config struct {
	DBDriver           string `env:"DB_DRIVER"`
	DBHost             string `env:"DB_HOST"`
	DBUser             string `env:"DB_USER"`
	DBPassword         string `env:"DB_PASSWORD"`
	DBName             string `env:"DB_NAME"`
	DBPort             string `env:"DB_PORT"`
	DBPath             string `env:"DB_PATH"`
	Environment        string `env:"ENV"`
	JWTSecret          string `env:"JWT_SECRET"`
	Port               string `env:"PORT"`
	CookieSecure       bool   `env:"COOKIE_SECURE,default=true"`
	GoogleClientID     string `env:"GOOGLE_CLIENT_ID"`
	GoogleClientSecret string `env:"GOOGLE_CLIENT_SECRET"`
	GoogleRedirectURL  string `env:"GOOGLE_REDIRECT_URL"`
}

func Load(ctx context.Context) (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{}
	if err := envconfig.Process(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

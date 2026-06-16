package config

import (
	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

func Load[T any](cfg *T) *T {
	godotenv.Load()

	if err := env.Parse(cfg); err != nil {
		panic(err)
	}

	return cfg
}

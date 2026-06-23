package main

import (
	"log"
	"os"
	"github.com/BurntSushi/toml"
)

type ServerConfig struct {
	URL            string `toml:"url"`
	HealthEndpoint string `toml:"health_endpoint"`
}
type Config struct {
	Server struct {
		HealthCheckInterval int            `toml:"health_check_interval"`
		ServerList          []ServerConfig `toml:"servers"`
	} `toml:"server"`
}

func LoadConfig(filename string) Config {
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Fatal("Failed to read config: ", err)
	}

	var config Config
	if err := toml.Unmarshal(data, &config); err != nil {
		log.Fatal("Failed to parse config: ", err)
	}

	return config
}

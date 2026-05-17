package main
import (
	"fmt"
	"log"

	"github.com/BurntSushi/toml"
)



type Config struct {
}

var config Config
_, err = toml.Decode(data)
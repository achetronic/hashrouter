package config

import (
	"hashrouter/api"
	"os"

	"gopkg.in/yaml.v3"
)

// Marshal TODO
func Marshal(config api.ConfigT) (bytes []byte, err error) {
	bytes, err = yaml.Marshal(config)
	return bytes, err
}

// Unmarshal TODO
func Unmarshal(bytes []byte) (config api.ConfigT, err error) {
	err = yaml.Unmarshal(bytes, &config)
	return config, err
}

// ReadFile TODO
func ReadFile(filepath string) (config api.ConfigT, err error) {
	var fileBytes []byte
	fileBytes, err = os.ReadFile(filepath)
	if err != nil {
		return config, err
	}

	config, err = Unmarshal(fileBytes)

	return config, err
}

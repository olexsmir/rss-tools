package app

import (
	"encoding/json"
	"os"
)

type Config struct {
	Port      int    `json:"port"`
	AuthToken string `json:"auth_token"`
	TGUserID  int64  `json:"tg_userid"`
	TGToken   string `json:"tg_token"`
}

func NewConfig(fpath string) (*Config, error) {
	// TODO per source config

	configFile, err := os.ReadFile(fpath)
	if err != nil {
		return nil, err
	}

	var config Config
	err = json.Unmarshal(configFile, &config)
	return &config, err
}

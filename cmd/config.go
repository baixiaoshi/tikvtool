package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Address   []string `json:"address"`
	PDAddress []string `json:"pd_address"`
	User      string   `json:"user"`
	Password  string   `json:"passwd"`
}

func LoadConfig(configPath string) (*Config, error) {
	// 如果没有指定配置文件路径，使用默认配置
	if configPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return getDefaultConfig(), nil
		}
		configPath = filepath.Join(homeDir, ".tikvtool.json")
	}

	// 检查文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		config := getDefaultConfig()
		// 创建默认配置文件
		if err := SaveConfig(configPath, config); err != nil {
			fmt.Printf("Warning: Could not create config file: %v\n", err)
		} else {
			fmt.Printf("Created default config file: %s\n", configPath)
			fmt.Println("Please edit the config file with your TiKV connection details.")
		}
		return config, nil
	}

	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %v", err)
	}
	defer file.Close()

	var config Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	return &config, nil
}

func SaveConfig(configPath string, config *Config) error {
	file, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("failed to create config file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	return nil
}

func getDefaultConfig() *Config {
	return &Config{
		Address:   []string{"172.16.0.10:2379"},
		PDAddress: []string{"172.16.0.10:2379"},
		User:      "",
		Password:  "",
	}
}

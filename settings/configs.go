package settings

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"os"
)

type ConfigurationFile struct {
	Database struct {
		Type     string `yaml:"type"`
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		Database string `yaml:"database"`
	} `yaml:"database"`
	Tools struct {
		SerpApi struct {
			Token string `yaml:"token"`
		} `yaml:"serp-api"`
		ProxyCrawl struct {
			Token string `yaml:"token"`
		} `yaml:"proxy-crawl"`
	} `yaml:"tools"`
	VectorDBs []VectorDBConfigurationSection `yaml:"vector-dbs"`
	Compute   []struct {
		Endpoint     string `yaml:"endpoint"`
		Type         string `yaml:"type"`
		MaxBatchSize int    `yaml:"max-batch-size"`
	} `yaml:"compute"`
}

type VectorDBConfigurationSection struct {
	Type     string `yaml:"type"`
	Endpoint string `yaml:"endpoint"`
	APIToken string `yaml:"api-token"`
}

func ProcessConfigurationFile(path string) (*ConfigurationFile, error) {
	// read YAML file
	config := &ConfigurationFile{}

	yamlText, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error loading configuration file %s: %v", path, err)
	}

	err = yaml.Unmarshal(yamlText, config)
	if err != nil {
		return nil, fmt.Errorf("error parsing configuration file %s: %v", path, err)
	}

	return config, nil
}

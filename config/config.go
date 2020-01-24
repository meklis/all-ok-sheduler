package config

import (
	"fmt"
	"github.com/meklis/all-ok-sheduler/shedule"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"strings"
)

type Configuration struct {
	Database struct {
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		UserName string `yaml:"username"`
		Password string `yaml:"password"`
		DbName   string `yaml:"database_name"`
	} `yaml:"database"`
	Shedule shedule.SheduleConfig `yaml:"shedule"`
	Logger  struct {
		Console struct {
			Enabled        bool `yaml:"enabled"`
			EnabledColor   bool `yaml:"enable_color"`
			LogLevel       int  `yaml:"log_level"`
			PrintDebugLine bool `yaml:"print_file"`
		} `yaml:"console"`
	} `yaml:"logger"`
}

func LoadConfig(path string, Config *Configuration) error {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	yamlConfig := string(bytes)
	for _, e := range os.Environ() {
		pair := strings.SplitN(e, "=", 2)
		yamlConfig = strings.ReplaceAll(yamlConfig, fmt.Sprintf("${%v}", pair[0]), pair[1])
	}
	fmt.Printf(`Loaded config from %v with env readed:
%v
`, path, yamlConfig)
	err = yaml.Unmarshal([]byte(yamlConfig), &Config)
	if err != nil {
		return err
	}
	return nil
}

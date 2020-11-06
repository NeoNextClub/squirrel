package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"squirrel/log"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

type config struct {
	// MySQL configs.
	User     string
	Password string
	Hostname string
	Port     string
	Database string

	// Label sets log output prefix.
	Label string

	RPCs []string `mapstructure:"rpc_url"`

	// Workers sets the number of goroutines that will be created for data processing.
	// Recommend value: 3.
	Workers int

	// AliyunMail is an optional config which will be used in mail alert package.
	AliyunMail AliyunMailConfig `mapstructure:"aliyun_mail"`
}

// AliyunMailConfig is the struct for aliyun mail configs.
type AliyunMailConfig struct {
	AccountName     string
	Region          string
	AccessKeyID     string
	AccessKeySecret string
	Receiver        []string
}

var cfg config

// Load creates a single.
func Load(display bool) {
	viper.SetConfigName("config")
	viper.AddConfigPath("./config")
	// Incase test cases require loading configs.
	viper.AddConfigPath("../config")

	if err := load(display); err != nil {
		panic(err)
	}

	if err := check(); err != nil {
		panic(err)
	}

	update()

	log.UpdatePrefix(GetLabel())

	viper.WatchConfig()
	viper.OnConfigChange(onConfigChange)
}

func load(display bool) error {
	err := viper.ReadInConfig()
	if err != nil {
		return err
	}

	err = viper.Unmarshal(&cfg)
	if err != nil {
		return err
	}

	if display {
		configContent, _ := json.MarshalIndent(cfg, "", "    ")
		log.Println(string(configContent))
	}

	return nil
}

func update() {
	for i := 0; i < len(cfg.RPCs); i++ {
		rpc := cfg.RPCs[i]
		if !strings.HasPrefix(rpc, "http") {
			cfg.RPCs[i] = "http://" + rpc
		}
	}
}

// GetDbConnStr returns mysql connection string.
func GetDbConnStr() string {
	str := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s",
		cfg.User,
		cfg.Password,
		cfg.Hostname,
		cfg.Port,
		cfg.Database,
	)

	params := []string{
		"charset=utf8mb4",
		"parseTime=True",
		"loc=Local",
		"maxAllowedPacket=52428800",
		"multiStatements=True",
	}

	if len(params) > 0 {
		str = fmt.Sprintf("%s?%s", str, strings.Join(params, "&"))
	}

	return str
}

// GetLabel returns custome label as console output prefix.
func GetLabel() string {
	return cfg.Label
}

// GetRPCs returns all rpc urls from config.
func GetRPCs() []string {
	return cfg.RPCs
}

// GetGoroutines returns the number of working goroutines.
func GetGoroutines() int {
	return cfg.Workers
}

// LoadAliyunMailConfig performs a basic check on aliyun mail config.
func LoadAliyunMailConfig() error {
	return checkAliyunMail()
}

// GetAliyunMailConfig returns aliyun mail configs.
func GetAliyunMailConfig() AliyunMailConfig {
	return cfg.AliyunMail
}

func check() error {
	if err := checkWorker(); err != nil {
		return err
	}

	if err := checkRPCs(); err != nil {
		return err
	}

	return nil
}

func checkWorker() error {
	if cfg.Workers < 1 {
		return errors.New("value of 'goroutine' must greater than or equal to 1")
	}
	return nil
}

func checkRPCs() error {
	if len(cfg.RPCs) < 1 {
		return errors.New("at least 1 rpc server url must be set")
	}

	for _, rpc := range cfg.RPCs {
		if strings.HasPrefix(rpc, "http") {
			_, err := url.Parse(rpc)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func checkAliyunMail() error {
	m := cfg.AliyunMail

	if m.AccountName == "" {
		return errors.New("aliyun mail account name cannot be empty")
	}

	if m.Region == "" {
		return errors.New("aliyun mail region cannot be empty")
	}

	if m.AccessKeyID == "" {
		return errors.New("aliyun mail accessKeyID cannot be empty")
	}

	if m.AccessKeySecret == "" {
		return errors.New("aliyun mail accessKeySecret cannot be empty")
	}

	if len(m.Receiver) == 0 {
		return errors.New("aliyun mail receiver cannot be empty")
	}

	return nil
}

func onConfigChange(e fsnotify.Event) {
	log.Printf("Config file change detected: %s", e.Name)

	const stdErr = "Failed to read new configuration, current configuration stay unchanged"

	if err := viper.ReadInConfig(); err != nil {
		log.Printf("%s: %s", stdErr, err)
		return
	}

	if err := load(true); err != nil {
		log.Printf("%s: %s", stdErr, err)
		return
	}

	if err := check(); err != nil {
		log.Printf("%s: %s", stdErr, err)
		return
	}

	log.UpdatePrefix(GetLabel())
}

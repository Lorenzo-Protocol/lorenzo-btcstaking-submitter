package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	lrzcfg "github.com/Lorenzo-Protocol/lorenzo-sdk/v2/config"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

const AppName = "submitter"

const (
	MinConfirmationDepth = 1
)

type Config struct {
	TxRelayer    TxRelayerConfig    `mapstructure:"tx-relayer"`
	BNBTxRelayer BNBTxRelayerConfig `mapstructure:"bnb-tx-relayer"`

	Database Database             `mapstructure:"database"`
	Lorenzo  lrzcfg.LorenzoConfig `mapstructure:"lorenzo"`
}

type Database struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
}

type TxRelayerConfig struct {
	BtcApiEndpoint    string `mapstructure:"btcApiEndpoint"`
	ConfirmationDepth uint64 `mapstructure:"confirmationDepth"`
	NetParams         string `mapstructure:"netParams"`
}

func (cfg *TxRelayerConfig) Validate() error {
	if cfg.ConfirmationDepth < MinConfirmationDepth {
		return fmt.Errorf("confirmationDepth must be larger than %d", MinConfirmationDepth)
	}
	if cfg.BtcApiEndpoint == "" {
		return errors.New("BtcApiEndpoint is empty")
	}

	return nil
}

type BNBTxRelayerConfig struct {
	RpcUrl              string `mapstructure:"rpcUrl"`
	PlanStakeHubAddress string `mapstructure:"planStakeHubAddress"`
	ConfirmationDepth   uint64 `mapstructure:"confirmationDepth"`
}

func (cfg *BNBTxRelayerConfig) Validate() error {
	if len(cfg.RpcUrl) == 0 {
		return fmt.Errorf("rpcUrl is empty")
	}

	return nil
}

func (cfg *Config) Validate() error {
	if err := cfg.TxRelayer.Validate(); err != nil {
		return err
	}
	if err := cfg.BNBTxRelayer.Validate(); err != nil {
		return err
	}

	fillLorenzoConfigDefaultValueIfNotSet(&cfg.Lorenzo)
	if err := cfg.Lorenzo.Validate(); err != nil {
		return err
	}

	return nil
}

func (cfg *Config) CreateLogger(debugMode bool) (*zap.Logger, error) {
	return NewRootLogger("auto", debugMode)
}

func fillLorenzoConfigDefaultValueIfNotSet(lorenzo *lrzcfg.LorenzoConfig) {
	if lorenzo == nil {
		return
	}
	if lorenzo.AccountPrefix == "" {
		lorenzo.AccountPrefix = "lrz"
	}
	if lorenzo.GasAdjustment == 0 {
		lorenzo.GasAdjustment = 1.5
	}
	if lorenzo.GasPrices == "" {
		lorenzo.GasPrices = "0alrz"
	}
	if lorenzo.Timeout == 0 {
		lorenzo.Timeout = time.Second * 20
	}
	if lorenzo.OutputFormat == "" {
		lorenzo.OutputFormat = "json"
	}
	if lorenzo.SignModeStr == "" {
		lorenzo.SignModeStr = "direct"
	}
}

// NewConfig returns a fully parsed Config object from a given file directory
func NewConfig(configFile string) (Config, error) {
	if _, err := os.Stat(configFile); err == nil { // the given file exists, parse it
		viper.SetConfigFile(configFile)
		if err := viper.ReadInConfig(); err != nil {
			return Config{}, err
		}
		var cfg Config
		if err := viper.Unmarshal(&cfg); err != nil {
			return Config{}, err
		}
		if err := cfg.Validate(); err != nil {
			return Config{}, err
		}
		return cfg, err
	} else if errors.Is(err, os.ErrNotExist) { // the given config file does not exist, return error
		return Config{}, fmt.Errorf("no config file found at %s", configFile)
	} else { // other errors
		return Config{}, err
	}
}

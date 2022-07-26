package config

import (
	yaml "gopkg.in/yaml.v2"
	"io/ioutil"

	"github.com/tappleby/slack_auth_proxy/slack"
)

type TokenConfig struct {
	BotToken   string `yaml:"bot"`
	OAuthToken string `yaml:"oauth"`
}

type Config struct {
	Listen    string `yaml:"listen"`
	ListenTLS string `yaml:"listen_tls"`

	Team string `yaml:"team"`

	Database struct {
		DSN string `yaml:"dsn"`
	} `yaml:"database"`

	BotTokens []TokenConfig `yaml:"bot_tokens"`

	Slack struct {
		ClientId     string `yaml:"client_id"`
		ClientSecret string `yaml:"client_secret"`
	} `yaml:"slack"`

	Cookies struct {
		AuthenticationKey string `yaml:"authentication_key"`
		EncryptionKey     string `yaml:"encryption_key"`
	} `yaml:"cookies"`

	Data string `yaml:"data"`

	SyncIntervalMinute int `yaml:"sync_interval_minute"`
	SyncRecentDay int `yaml:"sync_recent_day"`
}

func Load(path string) (*Config, error) {
	var c Config
	if err := c.Load(path); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) NewSlackOAuthClient(redirectUri string) *slack.OAuthClient {
	client := slack.NewOAuthClient(c.Slack.ClientId, c.Slack.ClientSecret, redirectUri)
	return client
}

func (c *Config) Load(path string) error {
	var err error
	var b []byte
	if b, err = ioutil.ReadFile(path); err != nil {
		return err
	}

	if err = yaml.Unmarshal(b, &c); err != nil {
		return err
	}

	if c.Data == "" {
		c.Data = "."
	}

	if c.Listen == "" {
		c.Listen = "127.0.0.1:8080"
	}

	if c.SyncIntervalMinute <= 0 {
		c.SyncIntervalMinute = 60
	}

	if c.SyncRecentDay <= 0 {
		c.SyncRecentDay = 30
	}

	err = c.init()
	return err
}

// initialize connections and auth
func (c *Config) init() error {
	return nil
}

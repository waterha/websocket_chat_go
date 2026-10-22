package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	ServerURL string `json:"serverURL"`
	Origin    string `json:"origin"`
}

func (c Config) Validate() (Config, error) {
	server, err := cleanOrigin(c.ServerURL)
	if err != nil {
		return Config{}, fmt.Errorf("服务器地址：%w", err)
	}
	origin := strings.TrimSpace(c.Origin)
	if origin == "" {
		origin = server
	}
	origin, err = cleanOrigin(origin)
	if err != nil {
		return Config{}, fmt.Errorf("网站地址：%w", err)
	}
	return Config{ServerURL: server, Origin: origin}, nil
}

func cleanOrigin(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", fmt.Errorf("请输入完整的 http:// 或 https:// 地址，不包含账号、路径或参数")
	}
	u.Path = ""
	return u.String(), nil
}

func Load(path string) (Config, error) {
	c := Config{ServerURL: "http://127.0.0.1:8080", Origin: "http://127.0.0.1:4173"}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &c); err != nil {
			return Config{}, fmt.Errorf("读取桌面设置：%w", err)
		}
	} else if !os.IsNotExist(err) {
		return Config{}, err
	}
	if value := os.Getenv("IM_DESKTOP_SERVER"); value != "" {
		c.ServerURL = value
		c.Origin = value
	}
	if value := os.Getenv("IM_DESKTOP_ORIGIN"); value != "" {
		c.Origin = value
	}
	return c.Validate()
}

func Save(path string, config Config) error {
	c, err := config.Validate()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

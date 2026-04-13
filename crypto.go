package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	masterKeyEnv = "CONFIG_MASTER_KEY"
	keyLen       = 32
	nonceLen     = 12
)

type encryptedConfigFile struct {
	Version    int    `json:"version"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func getMasterKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv(masterKeyEnv))
	if raw == "" {
		return nil, errors.New("missing CONFIG_MASTER_KEY")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid CONFIG_MASTER_KEY base64: %w", err)
	}
	if len(key) != keyLen {
		return nil, fmt.Errorf("CONFIG_MASTER_KEY must decode to %d bytes", keyLen)
	}
	return key, nil
}

func encryptConfig(cfg ConfigRequest) ([]byte, error) {
	key, err := getMasterKey()
	if err != nil {
		return nil, err
	}

	plain, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nil, nonce, plain, nil)
	enc := encryptedConfigFile{
		Version:    1,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}
	return json.MarshalIndent(enc, "", "  ")
}

func decryptConfig(data []byte) (ConfigRequest, error) {
	var enc encryptedConfigFile
	if err := json.Unmarshal(data, &enc); err != nil {
		return ConfigRequest{}, err
	}
	if enc.Version != 1 {
		return ConfigRequest{}, fmt.Errorf("unsupported config version: %d", enc.Version)
	}

	key, err := getMasterKey()
	if err != nil {
		return ConfigRequest{}, err
	}

	nonce, err := base64.StdEncoding.DecodeString(enc.Nonce)
	if err != nil {
		return ConfigRequest{}, fmt.Errorf("invalid nonce: %w", err)
	}
	if len(nonce) != nonceLen {
		return ConfigRequest{}, fmt.Errorf("invalid nonce length: %d", len(nonce))
	}
	ciphertext, err := base64.StdEncoding.DecodeString(enc.Ciphertext)
	if err != nil {
		return ConfigRequest{}, fmt.Errorf("invalid ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return ConfigRequest{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return ConfigRequest{}, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return ConfigRequest{}, fmt.Errorf("decrypt failed: %w", err)
	}

	var cfg ConfigRequest
	if err := json.Unmarshal(plain, &cfg); err != nil {
		return ConfigRequest{}, err
	}
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.APISecret = strings.TrimSpace(cfg.APISecret)
	return cfg, nil
}

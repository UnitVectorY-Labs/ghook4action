package main

import (
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	gh "github.com/UnitVectorY-Labs/ghook4action/internal/github"
)

// configuredGitHubClient accepts one complete authentication mode. Private key
// and token contents never appear in errors, flags, or application logs.
func configuredGitHubClient() (*gh.Client, error) {
	appID := strings.TrimSpace(os.Getenv("GITHUB_APP_ID"))
	installation := strings.TrimSpace(os.Getenv("GITHUB_APP_INSTALLATION_ID"))
	keyPath := os.Getenv("GITHUB_APP_PRIVATE_KEY_FILE")
	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	tokenPath := os.Getenv("GITHUB_TOKEN_FILE")
	if appID != "" || installation != "" || keyPath != "" {
		if token != "" || tokenPath != "" {
			return nil, errors.New("GitHub App credentials cannot be combined with GITHUB_TOKEN or GITHUB_TOKEN_FILE")
		}
		if appID == "" || installation == "" || keyPath == "" {
			return nil, errors.New("GITHUB_APP_ID, GITHUB_APP_INSTALLATION_ID, and GITHUB_APP_PRIVATE_KEY_FILE are all required for App authentication")
		}
		id, err := strconv.ParseInt(installation, 10, 64)
		if err != nil || id <= 0 {
			return nil, errors.New("GITHUB_APP_INSTALLATION_ID must be a positive integer")
		}
		key, err := readCredential(keyPath)
		if err != nil {
			return nil, errors.New("cannot read GITHUB_APP_PRIVATE_KEY_FILE")
		}
		return gh.NewAppClient(appID, id, key)
	}
	if tokenPath != "" {
		if token != "" {
			return nil, errors.New("set only one of GITHUB_TOKEN and GITHUB_TOKEN_FILE")
		}
		data, err := readCredential(tokenPath)
		if err != nil {
			return nil, errors.New("cannot read GITHUB_TOKEN_FILE")
		}
		token = strings.TrimSpace(string(data))
	}
	if token == "" {
		return nil, errors.New("configure GitHub App credentials or GITHUB_TOKEN_FILE/GITHUB_TOKEN")
	}
	return gh.NewClient(token), nil
}

func readCredential(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("cannot open credential file")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, errors.New("invalid credential file")
	}
	return data, nil
}

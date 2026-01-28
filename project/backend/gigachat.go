package main

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	gigaChatToken     string
	gigaChatTokenExp  time.Time
	gigaChatTokenLock sync.Mutex
)

type GigaChatAuthResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   int64  `json:"expires_at"`
}

type GigaChatRequest struct {
	Model    string             `json:"model"`
	Messages []GigaChatMessage  `json:"messages"`
}

type GigaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type GigaChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func getGigaChatToken() (string, error) {
	gigaChatTokenLock.Lock()
	defer gigaChatTokenLock.Unlock()

	if gigaChatToken != "" && time.Now().Before(gigaChatTokenExp) {
		return gigaChatToken, nil
	}

	clientID := os.Getenv("GIGACHAT_CLIENT_ID")
	clientSecret := os.Getenv("GIGACHAT_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" ||
		clientID == "your_gigachat_client_id_here" ||
		clientSecret == "your_gigachat_client_secret_here" {
		return "", fmt.Errorf("GigaChat credentials not configured")
	}

	credentials := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))

	data := url.Values{}
	data.Set("scope", "GIGACHAT_API_PERS")

	req, err := http.NewRequest("POST", "https://ngw.devices.sberbank.ru:9443/api/v2/oauth", strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create auth request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+credentials)
	req.Header.Set("RqUID", fmt.Sprintf("%d", time.Now().UnixNano()))

	// GigaChat uses self-signed certificates
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read auth response: %w", err)
	}

	var authResp GigaChatAuthResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		return "", fmt.Errorf("failed to parse auth response: %w", err)
	}

	gigaChatToken = authResp.AccessToken
	gigaChatTokenExp = time.Unix(authResp.ExpiresAt/1000, 0).Add(-time.Minute)

	return gigaChatToken, nil
}

func sendToGigaChat(message string) (string, error) {
	token, err := getGigaChatToken()
	if err != nil {
		return "GigaChat API not configured. Please add your credentials to the .env file.", nil
	}

	reqBody := GigaChatRequest{
		Model: "GigaChat",
		Messages: []GigaChatMessage{
			{Role: "user", Content: message},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://gigachat.devices.sberbank.ru/api/v1/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var gigaResp GigaChatResponse
	if err := json.Unmarshal(body, &gigaResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(gigaResp.Choices) == 0 {
		return "", fmt.Errorf("empty response from API")
	}

	return gigaResp.Choices[0].Message.Content, nil
}

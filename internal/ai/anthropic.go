// Package ai 实现 Anthropic Messages API 客户端与 bug 智能分析。
// 兼容任何 Anthropic 协议端点（官方 API / 阿里云百炼兼容模式 / 代理网关）。
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const anthropicVersion = "2023-06-01"

type Client struct {
	BaseURL string // 如 https://token-plan.cn-beijing.maas.aliyuncs.com/apps/anthropic
	APIKey  string
	Model   string
	HTTP    *http.Client
}

func NewClient(baseURL, apiKey, model string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		HTTP:    &http.Client{Timeout: 120 * time.Second},
	}
}

type TextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ImageSource struct {
	Type      string `json:"type"` // base64
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type ImageBlock struct {
	Type   string      `json:"type"`
	Source ImageSource `json:"source"`
}

type message struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string 或 []Block
}

type sendRequest struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	System      string    `json:"system,omitempty"`
	Messages    []message `json:"messages"`
	Temperature float64   `json:"temperature"`
}

type sendResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Send 发送一条 user 消息。content 传 string（纯文本）或 []any（TextBlock/ImageBlock 混合）。
func (c *Client) Send(ctx context.Context, system string, content any, maxTokens int) (string, error) {
	if c.APIKey == "" {
		return "", fmt.Errorf("未配置 AI API Key，请在设置里填写")
	}
	if c.Model == "" {
		return "", fmt.Errorf("未配置 AI 模型，请在设置里填写")
	}
	body, err := json.Marshal(sendRequest{
		Model:       c.Model,
		MaxTokens:   maxTokens,
		System:      system,
		Messages:    []message{{Role: "user", Content: content}},
		Temperature: 0.2,
	})
	if err != nil {
		return "", err
	}
	url := c.BaseURL + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "remember-fix/0.1")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("AI 请求失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("读取 AI 响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("AI 返回 %d: %s", resp.StatusCode, truncate(string(raw), 500))
	}
	var out sendResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("解析 AI 响应失败: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("AI 错误(%s): %s", out.Error.Type, out.Error.Message)
	}
	var sb strings.Builder
	for _, blk := range out.Content {
		if blk.Type == "text" {
			sb.WriteString(blk.Text)
		}
	}
	if sb.Len() == 0 {
		return "", fmt.Errorf("AI 响应为空")
	}
	return sb.String(), nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

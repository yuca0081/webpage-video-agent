// Package llm：LLM 作业的 API 实现（OpenAI 兼容协议，DeepSeek/GLM/Qwen 通吃）。
// 产物一律过 schema 校验（contract 包），不信任模型输出。
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

type Role string

const (
	RoleDialogue Role = "dialogue" // 主管对话（便宜快）
	RolePlan     Role = "plan"     // 分镜规划（中档）
	RoleVisual   Role = "visual"   // 合成物生成（最强）
)

// Provider 一个 OpenAI 兼容供应商。M0 主选 DeepSeek；GLM/Qwen 换 BaseURL+Key+Model 即可。
type Provider struct {
	Name    string
	BaseURL string
	APIKey  string
	Model   string
}

// FromEnv 按环境变量构造供应商。缺 key 返回错误（调用方转 ErrAwaitLLM 降级会话模式）。
func FromEnv(role Role) (*Provider, error) {
	base := os.Getenv("DEEPSEEK_BASE_URL")
	if base == "" {
		base = "https://api.deepseek.com/v1"
	}
	switch role {
	case RolePlan, RoleDialogue, RoleVisual: // M0 单模型起步：bake-off 后按档分模型
		key := os.Getenv("DEEPSEEK_API_KEY")
		if key == "" {
			return nil, errors.New("DEEPSEEK_API_KEY 未配置")
		}
		model := os.Getenv("DEEPSEEK_MODEL")
		if model == "" {
			model = "deepseek-chat"
		}
		return &Provider{Name: "deepseek", BaseURL: base, APIKey: key, Model: model}, nil
	}
	return nil, fmt.Errorf("未知角色 %s", role)
}

// GenerateJSON 一次 JSON 作业：system+user → 模型 → 剥掉 markdown 围栏 → json.Unmarshal 到 out。
// 返回本次 token 用量（成本记账）；重试时的用量取最后一次成功调用。
func (p *Provider) GenerateJSON(ctx context.Context, system, user string, out any) (openai.Usage, error) {
	cli := openai.NewClientWithConfig(cfg(p))
	var lastErr error
	var usage openai.Usage
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
		resp, err := cli.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: p.Model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: system},
				{Role: openai.ChatMessageRoleUser, Content: user},
			},
			ResponseFormat: &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONObject},
			Temperature:    0.7,
		})
		if err != nil {
			lastErr = err
			continue // 网络/限流类错误重试
		}
		usage = resp.Usage
		content := stripFence(resp.Choices[0].Message.Content)
		if err := json.Unmarshal([]byte(content), out); err != nil {
			lastErr = fmt.Errorf("模型输出不是合法 JSON: %w（原文前 200 字: %.200s）", err, content)
			continue
		}
		return usage, nil
	}
	return usage, fmt.Errorf("LLM 作业 3 次尝试均失败: %w", lastErr)
}

func cfg(p *Provider) openai.ClientConfig {
	c := openai.DefaultConfig(p.APIKey)
	c.BaseURL = p.BaseURL
	return c
}

func stripFence(s string) string {
	s = strings.TrimSpace(s)
	for _, fence := range []string{"```json", "```JSON", "```"} {
		if i := strings.Index(s, fence); i >= 0 {
			s = s[i+len(fence):]
			if j := strings.LastIndex(s, "```"); j >= 0 {
				s = s[:j]
			}
			return strings.TrimSpace(s)
		}
	}
	return s
}

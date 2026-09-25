// Package llm：LLM 作业的 API 实现（OpenAI 兼容协议：GLM 编码套餐 / DeepSeek / Qwen 通吃）。
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

// Provider 一个 OpenAI 兼容供应商。默认 GLM 编码套餐；换 DeepSeek/按量计费平台改 BaseURL+Key+Model 即可。
type Provider struct {
	Name    string
	BaseURL string
	APIKey  string
	Model   string
}

// FromEnv 按环境变量构造供应商：LLM_API_KEY / LLM_BASE_URL / LLM_MODEL（旧名 DEEPSEEK_* 仍兼容）。
// 默认 GLM 编码套餐端点 + glm-5.3-flash。缺 key 返回错误（调用方转 ErrAwaitLLM 降级会话模式）。
func FromEnv(role Role) (*Provider, error) {
	get := func(newName, oldName string) string {
		if v := os.Getenv(newName); v != "" {
			return v
		}
		return os.Getenv(oldName)
	}
	switch role {
	case RolePlan, RoleDialogue, RoleVisual: // M0 单模型起步：bake-off 后按档分模型
		key := get("LLM_API_KEY", "DEEPSEEK_API_KEY")
		if key == "" {
			return nil, errors.New("LLM_API_KEY 未配置（旧名 DEEPSEEK_API_KEY 亦未配置）")
		}
		base := get("LLM_BASE_URL", "DEEPSEEK_BASE_URL")
		if base == "" {
			base = "https://open.bigmodel.cn/api/coding/paas/v4"
		}
		model := get("LLM_MODEL", "DEEPSEEK_MODEL")
		if model == "" {
			model = "glm-5.3-flash"
		}
		return &Provider{Name: "llm", BaseURL: base, APIKey: key, Model: model}, nil
	}
	return nil, fmt.Errorf("未知角色 %s", role)
}

// GenerateJSON 一次 JSON 作业：system+user → 模型 → 剥掉 markdown 围栏 → json.Unmarshal 到 out。
// 返回本次 token 用量（成本记账）；重试时的用量取最后一次成功调用。
func (p *Provider) GenerateJSON(ctx context.Context, system, user string, out any) (openai.Usage, error) {
	return p.generate(ctx, system,
		[]openai.ChatMessagePart{{Type: openai.ChatMessagePartTypeText, Text: user}}, out, 0.7)
}

// GenerateJSONVision 多模态 JSON 作业：user 文本 + 若干图片（data URL）→ 模型，解析同 GenerateJSON。
// 评审类作业用低温：要的是稳定判断不是发散。
func (p *Provider) GenerateJSONVision(ctx context.Context, system, user string, imageURLs []string, out any) (openai.Usage, error) {
	parts := make([]openai.ChatMessagePart, 0, len(imageURLs)+1)
	for _, u := range imageURLs {
		parts = append(parts, openai.ChatMessagePart{
			Type:     openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{URL: u},
		})
	}
	parts = append(parts, openai.ChatMessagePart{Type: openai.ChatMessagePartTypeText, Text: user})
	return p.generate(ctx, system, parts, out, 0.2)
}

func (p *Provider) generate(ctx context.Context, system string, userParts []openai.ChatMessagePart, out any, temperature float32) (openai.Usage, error) {
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
				{Role: openai.ChatMessageRoleUser, MultiContent: userParts},
			},
			ResponseFormat: &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONObject},
			Temperature:    temperature,
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

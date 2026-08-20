package bedrock

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

type Client struct {
	bc         *bedrockruntime.Client
	http       *http.Client
	baseURL    string
	ChatModel  string
	EmbedModel string
	Dim        int
	MaxTokens  int
}

func New(ctx context.Context, region, chatModel, embed string, dim int) (*Client, error) {
	maxTokens := 1024
	if v := os.Getenv("LLM_MAX_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxTokens = n
		}
	}
	baseURL := strings.TrimRight(os.Getenv("OPENAI_BASE_URL"), "/")
	if baseURL != "" {
		return &Client{
			http:       &http.Client{Timeout: 120 * time.Second},
			baseURL:    baseURL,
			ChatModel:  envOr(os.Getenv("OPENAI_CHAT_MODEL"), chatModel),
			EmbedModel: envOr(os.Getenv("OPENAI_EMBED_MODEL"), embed),
			Dim:        dim,
			MaxTokens:  maxTokens,
		}, nil
	}
	cfg, err := awscfg.LoadDefaultConfig(ctx, awscfg.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return &Client{
		bc:         bedrockruntime.NewFromConfig(cfg),
		ChatModel:  chatModel,
		EmbedModel: embed,
		Dim:        dim,
		MaxTokens:  maxTokens,
	}, nil
}

func envOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	if os.Getenv("STILLMIND_STUB") == "1" {
		return stubEmbed(text, c.Dim), nil
	}
	if c.http != nil {
		return c.embedOpenAI(ctx, text)
	}
	if c.bc == nil {
		return stubEmbed(text, c.Dim), nil
	}
	body := `{"inputText":` + jsonString(text) + `}`
	out, err := c.bc.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(c.EmbedModel),
		ContentType: aws.String("application/json"),
		Body:        []byte(body),
	})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := jsonUnmarshal(out.Body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Embedding) == 0 {
		return nil, errors.New("empty embedding")
	}
	return resp.Embedding, nil
}

func (c *Client) embedOpenAI(ctx context.Context, text string) ([]float32, error) {
	payload, err := jsonMarshal(map[string]any{
		"model": c.EmbedModel,
		"input": text,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embeddings: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := jsonDecode(b, &out); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 || len(out.Data[0].Embedding) == 0 {
		return nil, errors.New("empty embedding")
	}
	return out.Data[0].Embedding, nil
}

type ChatMessage struct {
	Role    string
	Content string
}

func (c *Client) Chat(ctx context.Context, sys string, msgs []ChatMessage) (string, error) {
	if os.Getenv("STILLMIND_STUB") == "1" {
		return stubChat(sys, msgs), nil
	}
	if c.http != nil {
		return c.chatOpenAI(ctx, sys, msgs)
	}
	if c.bc == nil {
		return stubChat(sys, msgs), nil
	}
	var sb strings.Builder
	sb.WriteString(`{"anthropic_version":"bedrock-2023-05-31","max_tokens":`)
	sb.WriteString(strconv.Itoa(c.MaxTokens))
	sb.WriteString(`,"temperature":0.5`)
	if sys != "" {
		sb.WriteString(`,"system":`)
		sb.WriteString(jsonString(sys))
	}
	sb.WriteString(`,"messages":[`)
	for i, m := range msgs {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{"role":`)
		sb.WriteString(jsonString(m.Role))
		sb.WriteString(`,"content":`)
		sb.WriteString(jsonString(m.Content))
		sb.WriteByte('}')
	}
	sb.WriteString(`]}`)

	out, err := c.bc.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(c.ChatModel),
		ContentType: aws.String("application/json"),
		Body:        []byte(sb.String()),
	})
	if err != nil {
		return "", err
	}
	var resp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := jsonUnmarshal(out.Body, &resp); err != nil {
		return "", err
	}
	if len(resp.Content) == 0 {
		return "", errors.New("empty chat response")
	}
	return resp.Content[0].Text, nil
}

func (c *Client) chatOpenAI(ctx context.Context, sys string, msgs []ChatMessage) (string, error) {
	om := make([]map[string]string, 0, len(msgs)+1)
	if sys != "" {
		om = append(om, map[string]string{"role": "system", "content": sys})
	}
	for _, m := range msgs {
		om = append(om, map[string]string{"role": m.Role, "content": m.Content})
	}
	payload, err := jsonMarshal(map[string]any{
		"model":       c.ChatModel,
		"messages":    om,
		"max_tokens":  c.MaxTokens,
		"temperature": 0.5,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("chat: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if err := jsonDecode(b, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == "" {
		return "", errors.New("empty chat response")
	}
	return out.Choices[0].Message.Content, nil
}

func stubChat(sys string, msgs []ChatMessage) string {
	last := ""
	for _, m := range msgs {
		if m.Role == "user" {
			last = m.Content
		}
	}
	if strings.Contains(strings.ToLower(last), "plan") {
		return "Plan drafted. I'll save it and offer a CSV export. For a calm evening, try a 5-minute box-breath at 21:00 and a 10-minute body scan before sleep."
	}
	if strings.Contains(strings.ToLower(last), "remember") || strings.Contains(strings.ToLower(last), "last time") {
		return "I remember notes about your evenings — you preferred shorter sessions and lower lights. I'll keep that in mind."
	}
	return "Take a slow breath in for four, hold for four, release for six. I'll save this session and your reflections."
}

func stubEmbed(text string, dim int) []float32 {
	if dim <= 0 {
		dim = 1024
	}
	out := make([]float32, dim)
	seed := sha256.Sum256([]byte(strings.ToLower(text)))
	for i := 0; i < dim; i++ {
		bs := seed[i%len(seed)]
		b := binary.BigEndian.Uint32([]byte{bs, byte(i), byte(i >> 8), 0})
		f := float32(b%997) / 997.0
		out[i] = float32(math.Tanh(float64(f)))
	}
	var norm float32
	for _, v := range out {
		norm += v * v
	}
	norm = float32(math.Sqrt(float64(norm))) + 1e-9
	for i := range out {
		out[i] /= norm
	}
	return out
}

func jsonString(s string) string {
	b, _ := jsonMarshal(s)
	return string(b)
}

func jsonUnmarshal(b []byte, v any) error { return jsonDecode(b, v) }

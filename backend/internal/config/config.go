package config

import (
	"os"
	"strconv"
)

type Config struct {
	CRDBURL    string
	Port       string
	DemoTenant string
	DemoUser   string

	AWSRegion    string
	BedrockModel string
	BedrockEmbed string
	EmbedDim     int

	OpenAIBaseURL    string
	OpenAIChatModel  string
	OpenAIEmbedModel string

	CompactBudgetTokens      int
	CompactPressureThreshold float64
	CompactMinIntervalS      int

	LLMMaxTokens int
}

func Load() Config {
	return Config{
		CRDBURL:    env("CRDB_URL", "postgresql://root@localhost:26257/defaultdb?sslmode=disable"),
		Port:       env("PORT", "8080"),
		DemoTenant: env("DEMO_TENANT_ID", "demo-tenant"),
		DemoUser:   env("DEMO_USER_ID", "demo-user"),

		AWSRegion:    env("AWS_REGION", "us-east-1"),
		BedrockModel: env("BEDROCK_MODEL", "anthropic.claude-3-haiku-20240307-v1:0"),
		BedrockEmbed: env("BEDROCK_EMBED_MODEL", "amazon.titan-embed-text-v2:0"),
		EmbedDim:     atoi(env("EMBED_DIM", "1024"), 1024),

		OpenAIBaseURL:    env("OPENAI_BASE_URL", ""),
		OpenAIChatModel:  env("OPENAI_CHAT_MODEL", "llama3.2:1b"),
		OpenAIEmbedModel: env("OPENAI_EMBED_MODEL", "snowflake-arctic-embed"),

		CompactBudgetTokens:      atoi(env("COMPACT_BUDGET_TOKENS", "4000"), 4000),
		CompactPressureThreshold: atof(env("COMPACT_PRESSURE_THRESHOLD", "0.85"), 0.85),
		CompactMinIntervalS:      atoi(env("COMPACT_MIN_INTERVAL_S", "60"), 60),

		LLMMaxTokens: atoi(env("LLM_MAX_TOKENS", "1024"), 1024),
	}
}

func (c Config) UserID() string { return c.DemoTenant + ":" + c.DemoUser }

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func atoi(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}

func atof(s string, def float64) float64 {
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return v
	}
	return def
}

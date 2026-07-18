package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

// ========== 环境变量加载（和 CarAgent 风格一致）==========

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" { return v }
	return fallback
}
func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" { n, _ := strconv.Atoi(v); return n }
	return fallback
}
func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" { f, _ := strconv.ParseFloat(v, 64); return f }
	return fallback
}
func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" { b, _ := strconv.ParseBool(v); return b }
	return fallback
}

// ========== 全局配置 ==========

var global *Config

type Config struct {
	App       AppConfig
	MySQL     MySQLConfig
	Milvus    MilvusConfig
	LLM       LLMConfig
	Embedding EmbeddingConfig
	Reranker  RerankerConfig
	RAG       RAGConfig
	Redis     RedisConfig
	Log       LogConfig
	Mineru    MineruConfig
	SaToken   SaTokenConfig
	Memory    MemoryConfig
	Guidance  GuidanceConfig
	Ingestion IngestionConfig
	Mcp       McpConfig
}

// MemoryConfig 对话记忆配置（对话记忆配置）
type MemoryConfig struct {
	HistoryKeepTurns  int  // 加载最近 N 轮（N*2 条消息）
	TitleMaxLength    int  // 会话标题最大字符数（rune）
	SummaryEnabled    bool // 摘要压缩开关
	SummaryStartTurns int  // 用户消息数达到 N 触发摘要（需 > HistoryKeepTurns）
	SummaryMaxChars   int  // 摘要最大字符数
}

// GuidanceConfig 歧义引导配置（歧义引导配置）
type GuidanceConfig struct {
	Enabled             bool
	AmbiguityScoreRatio float64 // 次高/最高 分数比值 ≥ 此值直接判歧义
	AmbiguityMargin     float64 // [ratio-margin, ratio) 区间走 LLM 二次确认
	MaxOptions          int     // 引导选项最大数
}

type SaTokenConfig struct {
	TokenName string
	Timeout   int
}

type AppConfig struct {
	Name  string
	Env   string
	Debug bool
	Host  string
	Port  int
}
type MySQLConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	PoolSize int
	Echo     bool
}
type MilvusConfig struct {
	Host           string
	Port           int
	CollectionName string
}
type LLMConfig struct {
	Provider    string // openai / deepseek / qwen / glm
	Temperature float64
	MaxTokens   int

	// OpenAI (Mimo)
	OpenAIKey     string
	OpenAIBaseURL string
	OpenAIModel   string

	// DeepSeek
	DeepSeekKey     string
	DeepSeekBaseURL string
	DeepSeekModel   string

	// Qwen
	QwenKey     string
	QwenBaseURL string
	QwenModel   string

	// GLM
	GLMKey     string
	GLMBaseURL string
	GLMModel   string
}
type EmbeddingConfig struct {
	Model   string
	Device  string
	HTTPURL string
}
type RerankerConfig struct {
	Model   string
	Device  string
	HTTPURL string
}
type RAGConfig struct {
	TopK          int
	RerankTopK    int
	QueryRewrite  bool
	RerankEnabled bool
	DefaultTopK   int
	Search        SearchConfig
}

type SearchConfig struct {
	DefaultTopK int
	Channels    ChannelsConfig
}
type ChannelsConfig struct {
	VectorGlobal  VectorGlobalConfig
	IntentDirected IntentDirectedConfig
	WebSearch     WebSearchConfig
}
type VectorGlobalConfig struct {
	Enabled                       bool
	ConfidenceThreshold           float64
	SingleIntentSupplementThreshold float64
	TopKMultiplier                int
	CandidateBudget               int
}
type IntentDirectedConfig struct {
	Enabled        bool
	MinIntentScore float64
	TopKMultiplier int
}
type RedisConfig struct {
	Host     string
	Port     int
	DB       int
	Password string
}
type LogConfig struct {
	Level  string
	File   string
}
type MineruConfig struct {
	APIToken string
	DataDir  string // 文件管理根目录，默认 "data"
}

type IngestionConfig struct {
	ChunkSize      int // 分块大小（字符），默认 1024
	ChunkOverlap   int // 重叠大小（字符），默认 50
	EmbedBatchSize int // 嵌入批大小，默认 32
}

type WebSearchConfig struct {
	Enabled        bool
	APIKey         string
	APIURL         string
	Count          int
	TimeoutSeconds int
}

type McpConfig struct {
	Servers []McpServerConfig
}

type McpServerConfig struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// ========== Provider 模型解析 ==========

// ProviderModel 根据 provider 名称解析对应的模型/Key/URL
type ProviderModel struct {
	Name    string
	Key     string
	BaseURL string
	Model   string
}

func (c *LLMConfig) Resolve(provider string) ProviderModel {
	// 默认用 GLM
	if provider == "" { provider = "glm" }

	switch strings.ToLower(provider) {
	case "openai", "mimo":
		return ProviderModel{
			Name: "openai", Key: c.OpenAIKey,
			BaseURL: c.OpenAIBaseURL, Model: c.OpenAIModel,
		}
	case "deepseek":
		return ProviderModel{
			Name: "deepseek", Key: c.DeepSeekKey,
			BaseURL: c.DeepSeekBaseURL, Model: c.DeepSeekModel,
		}
	case "qwen":
		return ProviderModel{
			Name: "qwen", Key: c.QwenKey,
			BaseURL: c.QwenBaseURL, Model: c.QwenModel,
		}
	case "glm":
		return ProviderModel{
			Name: "glm", Key: c.GLMKey,
			BaseURL: c.GLMBaseURL, Model: c.GLMModel,
		}
	default:
		return ProviderModel{
			Name: "glm", Key: c.GLMKey,
			BaseURL: c.GLMBaseURL, Model: c.GLMModel,
		}
	}
}

// PrimaryProvider 返回主用 provider
func (c *LLMConfig) PrimaryProvider() string {
	if c.Provider != "" { return c.Provider }
	return "glm"
}

// ========== DSN ==========

func (c *MySQLConfig) DSN() string {
	return c.User + ":" + c.Password + "@tcp(" + c.Host + ":" + strconv.Itoa(c.Port) + ")/" + c.Database + "?charset=utf8mb4&parseTime=True&loc=Local"
}
func (c *MilvusConfig) URI() string {
	return "http://" + c.Host + ":" + strconv.Itoa(c.Port)
}

// ========== 加载入口 ==========

func Load() *Config {
	cfg := &Config{
		App: AppConfig{
			Name:  envStr("APP_NAME", "goRAGENT"),
			Env:   envStr("APP_ENV", "development"),
			Debug: envBool("APP_DEBUG", true),
			Host:  envStr("APP_HOST", "0.0.0.0"),
			Port:  envInt("APP_PORT", 9090),
		},
		MySQL: MySQLConfig{
			Host:     envStr("MYSQL_HOST", "localhost"),
			Port:     envInt("MYSQL_PORT", 3306),
			User:     envStr("MYSQL_USER", "root"),
			Password: envStr("MYSQL_PASSWORD", "123456"),
			Database: envStr("MYSQL_DATABASE", "ragent"),
			PoolSize: envInt("MYSQL_POOL_SIZE", 10),
			Echo:     envBool("MYSQL_ECHO", false),
		},
		Milvus: MilvusConfig{
			Host:           envStr("MILVUS_HOST", "localhost"),
			Port:           envInt("MILVUS_PORT", 19530),
			CollectionName: envStr("MILVUS_COLLECTION_NAME", "ragent_knowledge"),
		},
		Memory: MemoryConfig{
			HistoryKeepTurns:  envInt("MEMORY_HISTORY_KEEP_TURNS", 8),
			TitleMaxLength:    envInt("MEMORY_TITLE_MAX_LENGTH", 30),
			SummaryEnabled:    envBool("MEMORY_SUMMARY_ENABLED", true),
			SummaryStartTurns: envInt("MEMORY_SUMMARY_START_TURNS", 9),
			SummaryMaxChars:   envInt("MEMORY_SUMMARY_MAX_CHARS", 200),
		},
		Guidance: GuidanceConfig{
			Enabled:             envBool("GUIDANCE_ENABLED", true),
			AmbiguityScoreRatio: envFloat("GUIDANCE_AMBIGUITY_SCORE_RATIO", 0.8),
			AmbiguityMargin:     envFloat("GUIDANCE_AMBIGUITY_MARGIN", 0.15),
			MaxOptions:          envInt("GUIDANCE_MAX_OPTIONS", 6),
		},
		LLM: LLMConfig{
			Provider:    envStr("LLM_PROVIDER", "glm"),
			Temperature: envFloat("LLM_TEMPERATURE", 0.1),
			MaxTokens:   envInt("LLM_MAX_TOKENS", 2000),

			OpenAIKey:     envStr("OPENAI_API_KEY", ""),
			OpenAIBaseURL: envStr("OPENAI_BASE_URL", "https://api.openai.com/v1"),
			OpenAIModel:   envStr("OPENAI_MODEL", "gpt-4o-mini"),

			DeepSeekKey:     envStr("DEEPSEEK_API_KEY", ""),
			DeepSeekBaseURL: envStr("DEEPSEEK_BASE_URL", "https://api.deepseek.com"),
			DeepSeekModel:   envStr("DEEPSEEK_MODEL", "deepseek-chat"),

			QwenKey:     envStr("QWEN_API_KEY", ""),
			QwenBaseURL: envStr("QWEN_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1"),
			QwenModel:   envStr("QWEN_MODEL", "qwen-plus"),

			GLMKey:     envStr("GLM_API_KEY", ""),
			GLMBaseURL: envStr("GLM_BASE_URL", "https://open.bigmodel.cn/api/paas/v4"),
			GLMModel:   envStr("GLM_MODEL", "glm-4-flash"),
		},
		Embedding: EmbeddingConfig{
			Model:  envStr("EMBEDDING_MODEL", "BAAI/bge-small-zh-v1.5"),
			Device: envStr("EMBEDDING_DEVICE", "cpu"),
			HTTPURL: envStr("EMBEDDING_HTTP_URL", "http://localhost:19531"),
		},
		Reranker: RerankerConfig{
			Model:  envStr("RERANKER_MODEL", "Qwen/Qwen3-Reranker-0.6B"),
			Device: envStr("RERANKER_DEVICE", "cpu"),
			HTTPURL: envStr("RERANK_HTTP_URL", "http://localhost:19531"),
		},
		RAG: RAGConfig{
			TopK:          envInt("RAG_TOP_K", 10),
			RerankTopK:    envInt("RAG_RERANK_TOP_K", 5),
			QueryRewrite:  envBool("RAG_QUERY_REWRITE_ENABLED", true),
			RerankEnabled: envBool("RAG_RERANK_ENABLED", true),
			DefaultTopK:   10,
			Search: SearchConfig{
				DefaultTopK: 10,
				Channels: ChannelsConfig{
					VectorGlobal: VectorGlobalConfig{
						Enabled: true, ConfidenceThreshold: 0.6,
						SingleIntentSupplementThreshold: 0.8,
						TopKMultiplier: 3, CandidateBudget: 100,
					},
					IntentDirected: IntentDirectedConfig{
						Enabled: true, MinIntentScore: 0.4, TopKMultiplier: 2,
					},
					WebSearch: WebSearchConfig{
						Enabled:        envBool("WEB_SEARCH_ENABLED", false),
						APIKey:         envStr("WEB_SEARCH_API_KEY", ""),
						APIURL:         envStr("WEB_SEARCH_API_URL", "https://api.ydc-index.io/search"),
						Count:          envInt("WEB_SEARCH_COUNT", 5),
						TimeoutSeconds: envInt("WEB_SEARCH_TIMEOUT_SECONDS", 10),
					},
				},
			},
		},
		Redis: RedisConfig{
			Host:     envStr("REDIS_HOST", "localhost"),
			Port:     envInt("REDIS_PORT", 6379),
			DB:       envInt("REDIS_DB", 0),
			Password: envStr("REDIS_PASSWORD", ""),
		},
		Log: LogConfig{
			Level: envStr("LOG_LEVEL", "DEBUG"),
			File:  envStr("LOG_FILE", "logs/ragent.log"),
		},
		Mineru: MineruConfig{
			APIToken: envStr("MINERU_API_TOKEN", ""),
			DataDir:  envStr("MINERU_DATA_DIR", "data"),
		},
		Ingestion: IngestionConfig{
			ChunkSize:      envInt("INGESTION_CHUNK_SIZE", 1024),
			ChunkOverlap:   envInt("INGESTION_CHUNK_OVERLAP", 50),
			EmbedBatchSize: envInt("INGESTION_EMBED_BATCH_SIZE", 32),
		},
	}

	// MCP 服务器列表（JSON env var）
	var mcpServers []McpServerConfig
	if raw := os.Getenv("MCP_SERVERS"); raw != "" {
		json.Unmarshal([]byte(raw), &mcpServers)
	}
	cfg.Mcp = McpConfig{Servers: mcpServers}
	global = cfg
	return cfg
}

func Get() *Config { return global }


package llm

import "time"

const PROVIDER_ANTHROPIC = "anthropic"
const PROVIDER_OPENAI = "openai"
const PROVIDER_GOOGLE = "google"
const PROVIDER_DEEPSEEK = "deepseek"
const PROVIDER_OLLAMA = "ollama"
const PROVIDER_UNSLOTH = "unsloth"

const MODEL_ANTHOPIC_CLAUDE_OPUS_4_5 = "claude-opus-4-5"
const MODEL_ANTHOPIC_CLAUDE_SONNET_4_5 = "claude-sonnet-4-5"
const MODEL_ANTHOPIC_CLAUDE_SONNET_4_5_1M = "claude-sonnet-4-5-1m"
const MODEL_ANTHOPIC_CLAUDE_HAIKU_4_5 = "claude-haiku-4-5"
const MODEL_OPENAI_GPT_5_2_PRO = "gpt-5.2-pro"
const MODEL_OPENAI_GPT_5_2_CODEX = "gpt-5.2-codex"
const MODEL_OPENAI_CODEX_MINI = "codex-mini-latest"
const MODEL_GOOGLE_GEMINI_3_PRO = "gemini-3-pro"
const MODEL_GOOGLE_GEMINI_3_PRO_1M = "gemini-3-pro-1m"
const MODEL_GOOGLE_GEMINI_3_FLASH = "gemini-3-flash"
const MODEL_DEEPSEEK_CHAT = "deepseek-chat"
const MODEL_DEEPSEEK_REASONER = "deepseek-reasoner"
const MODEL_OLLAMA_QWEN3_CODER = "qwen3-coder"
const MODEL_UNSLOTH_QWEN3_27B = "unsloth/Qwen3.8-27B-GGUF"

const DEFAULT_UNSLOTH_API_HOST = "http://127.0.0.1:8888"

type Model struct {
	ModelID         string    `json:"model_id"`
	ModelAlias      string    `json:"model_alias"`
	Description     string    `json:"description"`
	Pricing         Pricing   `json:"pricing"`
	ContextWindow   int64     `json:"context_window"`
	MaxOutputTokens int64     `json:"max_output_tokens"`
	KnowledgeCutoff time.Time `json:"knowledge_cutoff"`
}

type Pricing struct {
	InputMTok        float32 `json:"input_mtok"`
	OutputMTok       float32 `json:"output_mtok"`
	CacheReadMTok    float32 `json:"cache_read_mtok"`
	CacheWriteMTok   float32 `json:"cache_write_mtok"`
	CacheWrite1HMTok float32 `json:"cache_write1h_mtok"` // only anthropic
}

var MODELS = map[string][]Model{
	PROVIDER_ANTHROPIC: []Model{
		{
			ModelID:     "claude-opus-4-5-20251101",
			ModelAlias:  MODEL_ANTHOPIC_CLAUDE_OPUS_4_5,
			Description: "Premium model combining maximum intelligence with practical performance",
			Pricing: Pricing{
				InputMTok:        5,
				OutputMTok:       25,
				CacheReadMTok:    0.5,
				CacheWriteMTok:   6.25,
				CacheWrite1HMTok: 10,
			},
			ContextWindow:   200000,
			MaxOutputTokens: 64000,
			KnowledgeCutoff: time.Date(2025, time.May, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			ModelID:     "claude-sonnet-4-5-20250929",
			ModelAlias:  MODEL_ANTHOPIC_CLAUDE_SONNET_4_5,
			Description: "Our smart model for complex agents and coding",
			Pricing: Pricing{
				InputMTok:        3,
				OutputMTok:       15,
				CacheReadMTok:    0.3,
				CacheWriteMTok:   3.75,
				CacheWrite1HMTok: 6,
			},
			ContextWindow:   200000,
			MaxOutputTokens: 64000,
			KnowledgeCutoff: time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			ModelID:     "claude-sonnet-4-5-20250929",
			ModelAlias:  MODEL_ANTHOPIC_CLAUDE_SONNET_4_5_1M,
			Description: "Our smart model for complex agents and coding with 1M context window",
			Pricing: Pricing{
				InputMTok:        6,
				OutputMTok:       22.5,
				CacheReadMTok:    0.6,
				CacheWriteMTok:   7.5,
				CacheWrite1HMTok: 12,
			},
			ContextWindow:   1000000,
			MaxOutputTokens: 64000,
			KnowledgeCutoff: time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			ModelID:     "claude-haiku-4-5-20251001",
			ModelAlias:  MODEL_ANTHOPIC_CLAUDE_HAIKU_4_5,
			Description: "Our fastest model with near-frontier intelligence",
			Pricing: Pricing{
				InputMTok:        1,
				OutputMTok:       5,
				CacheReadMTok:    0.1,
				CacheWriteMTok:   1.25,
				CacheWrite1HMTok: 2,
			},
			ContextWindow:   200000,
			MaxOutputTokens: 64000,
			KnowledgeCutoff: time.Date(2025, time.February, 1, 0, 0, 0, 0, time.UTC),
		},
	},
	PROVIDER_OPENAI: []Model{
		{
			ModelID:     "gpt-5.2-pro-2025-12-11",
			ModelAlias:  MODEL_OPENAI_GPT_5_2_PRO,
			Description: "Version of GPT-5.2 that produces smarter and more precise responses.",
			Pricing: Pricing{
				InputMTok:  21,
				OutputMTok: 168,
			},
			ContextWindow:   400000,
			MaxOutputTokens: 128000,
			KnowledgeCutoff: time.Date(2025, time.August, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			ModelID:     "gpt-5.2-codex",
			ModelAlias:  MODEL_OPENAI_GPT_5_2_CODEX,
			Description: "The best model for coding and agentic tasks across industries",
			Pricing: Pricing{
				InputMTok:     1.75,
				OutputMTok:    14,
				CacheReadMTok: 0.175,
			},
			ContextWindow:   400000,
			MaxOutputTokens: 128000,
			KnowledgeCutoff: time.Date(2025, time.August, 31, 0, 0, 0, 0, time.UTC),
		},
		{
			ModelID:     "codex-mini-latest",
			ModelAlias:  MODEL_OPENAI_CODEX_MINI,
			Description: "Fast reasoning model optimized for the Codex CLI",
			Pricing: Pricing{
				InputMTok:     1.5,
				OutputMTok:    6,
				CacheReadMTok: 0.375,
			},
			ContextWindow:   400000,
			MaxOutputTokens: 128000,
			KnowledgeCutoff: time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC),
		},
	},
	PROVIDER_GOOGLE: []Model{
		{
			ModelID:     "gemini-3-pro-preview",
			ModelAlias:  MODEL_GOOGLE_GEMINI_3_PRO,
			Description: "The best model in the world for multimodal understanding, and our most powerful agentic and vibe-coding model yet, delivering richer visuals and deeper interactivity, all built on a foundation of state-of-the-art reasoning.",
			Pricing: Pricing{
				InputMTok:     2,
				OutputMTok:    12,
				CacheReadMTok: 0.2,
			},
			ContextWindow:   200000,
			MaxOutputTokens: 65536,
			KnowledgeCutoff: time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			ModelID:     "gemini-3-pro-preview",
			ModelAlias:  MODEL_GOOGLE_GEMINI_3_PRO_1M,
			Description: "The best model in the world for multimodal understanding, and our most powerful agentic and vibe-coding model yet, delivering richer visuals and deeper interactivity, all built on a foundation of state-of-the-art reasoning.",
			Pricing: Pricing{
				InputMTok:     4,
				OutputMTok:    28,
				CacheReadMTok: 0.4,
			},
			ContextWindow:   1048576,
			MaxOutputTokens: 65536,
			KnowledgeCutoff: time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			ModelID:     "gemini-3-flash-preview",
			ModelAlias:  MODEL_GOOGLE_GEMINI_3_FLASH,
			Description: "Our most balanced model built for speed, scale, and frontier intelligence.",
			Pricing: Pricing{
				InputMTok:     2,
				OutputMTok:    12,
				CacheReadMTok: 0.2,
			},
			ContextWindow:   1048576,
			MaxOutputTokens: 65536,
			KnowledgeCutoff: time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
	},
	PROVIDER_DEEPSEEK: []Model{
		{
			ModelID:     "deepseek-chat",
			ModelAlias:  MODEL_DEEPSEEK_CHAT,
			Description: "Infererence ",
			Pricing: Pricing{
				InputMTok:     0.28,
				OutputMTok:    0.42,
				CacheReadMTok: 0.028,
			},
			ContextWindow:   128000,
			MaxOutputTokens: 8000,
			KnowledgeCutoff: time.Time{},
		},
		{
			ModelID:     "deepseek-reasoner",
			ModelAlias:  MODEL_DEEPSEEK_REASONER,
			Description: "",
			Pricing: Pricing{
				InputMTok:     0.28,
				OutputMTok:    0.42,
				CacheReadMTok: 0.028,
			},
			ContextWindow:   128000,
			MaxOutputTokens: 8000,
			KnowledgeCutoff: time.Time{},
		},
	},
	PROVIDER_OLLAMA: []Model{
		{
			ModelID:     "qwen3-coder",
			ModelAlias:  MODEL_OLLAMA_QWEN3_CODER,
			Description: "",
			Pricing: Pricing{
				InputMTok:        0,
				OutputMTok:       0,
				CacheReadMTok:    0,
				CacheWriteMTok:   0,
				CacheWrite1HMTok: 0,
			},
			ContextWindow:   0,
			MaxOutputTokens: 0,
			KnowledgeCutoff: time.Time{},
		},
	},
	PROVIDER_UNSLOTH: []Model{
		{
			ModelID:     MODEL_UNSLOTH_QWEN3_27B,
			ModelAlias:  MODEL_UNSLOTH_QWEN3_27B,
			Description: "Locally served model behind an unsloth studio OpenAI compatible endpoint",
			Pricing: Pricing{
				InputMTok:        0,
				OutputMTok:       0,
				CacheReadMTok:    0,
				CacheWriteMTok:   0,
				CacheWrite1HMTok: 0,
			},
			ContextWindow:   13312,
			MaxOutputTokens: 13312,
			KnowledgeCutoff: time.Time{},
		},
	},
}

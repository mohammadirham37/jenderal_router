package store

// ProviderTemplate adalah template bawaan untuk menambah provider cepat
// (FR-1.1). Harga adalah perkiraan per 1 juta token (USD) dan dapat diedit.
type ProviderTemplate struct {
	Type     string             `json:"type"`
	Name     string             `json:"name"`
	Prefix   string             `json:"prefix"`
	BaseURL  string             `json:"base_url"`
	Settings CredentialSettings `json:"settings"`
	Models   []TemplateModel    `json:"models"`
	DocsURL  string             `json:"docs_url,omitempty"`
}

// TemplateModel satu model awal dalam template.
type TemplateModel struct {
	UpstreamName  string  `json:"upstream_name"`
	DisplayName   string  `json:"display_name,omitempty"`
	PriceInPer1M  float64 `json:"price_in_per_1m"`
	PriceOutPer1M float64 `json:"price_out_per_1m"`
	ContextWindow int     `json:"context_window"`
	Tools         bool    `json:"tools"`
	Vision        bool    `json:"vision"`
}

// ProviderTemplates daftar template bawaan: 11 cloud + LlamaStash lokal.
var ProviderTemplates = []ProviderTemplate{
	{
		Type: ProviderOpenAI, Name: "OpenAI", Prefix: "oa",
		BaseURL:  "https://api.openai.com/v1",
		Settings: CredentialSettings{Strategy: CredStrategyRoundRobin},
		DocsURL:  "https://platform.openai.com/api-keys",
		Models: []TemplateModel{
			{UpstreamName: "gpt-5.4", DisplayName: "GPT-5.4", PriceInPer1M: 2.5, PriceOutPer1M: 10, ContextWindow: 400000, Tools: true, Vision: true},
			{UpstreamName: "gpt-5.4-mini", DisplayName: "GPT-5.4 Mini", PriceInPer1M: 0.25, PriceOutPer1M: 1, ContextWindow: 400000, Tools: true, Vision: true},
			{UpstreamName: "gpt-4.1", DisplayName: "GPT-4.1", PriceInPer1M: 2, PriceOutPer1M: 8, ContextWindow: 1047576, Tools: true, Vision: true},
			{UpstreamName: "gpt-4.1-mini", DisplayName: "GPT-4.1 Mini", PriceInPer1M: 0.4, PriceOutPer1M: 1.6, ContextWindow: 1047576, Tools: true, Vision: true},
			{UpstreamName: "o4-mini", DisplayName: "o4-mini", PriceInPer1M: 1.1, PriceOutPer1M: 4.4, ContextWindow: 200000, Tools: true},
		},
	},
	{
		Type: ProviderAnthropic, Name: "Anthropic", Prefix: "an",
		BaseURL:  "https://api.anthropic.com",
		Settings: CredentialSettings{Strategy: CredStrategyRoundRobin},
		DocsURL:  "https://console.anthropic.com/settings/keys",
		Models: []TemplateModel{
			{UpstreamName: "claude-sonnet-4-6", DisplayName: "Claude Sonnet 4.6", PriceInPer1M: 3, PriceOutPer1M: 15, ContextWindow: 200000, Tools: true, Vision: true},
			{UpstreamName: "claude-opus-4-6", DisplayName: "Claude Opus 4.6", PriceInPer1M: 15, PriceOutPer1M: 75, ContextWindow: 200000, Tools: true, Vision: true},
			{UpstreamName: "claude-haiku-4-5", DisplayName: "Claude Haiku 4.5", PriceInPer1M: 1, PriceOutPer1M: 5, ContextWindow: 200000, Tools: true, Vision: true},
		},
	},
	{
		Type: ProviderGemini, Name: "Google Gemini", Prefix: "gm",
		BaseURL:  "https://generativelanguage.googleapis.com/v1beta",
		Settings: CredentialSettings{Strategy: CredStrategyRoundRobin},
		DocsURL:  "https://aistudio.google.com/apikey",
		Models: []TemplateModel{
			{UpstreamName: "gemini-2.5-pro", DisplayName: "Gemini 2.5 Pro", PriceInPer1M: 1.25, PriceOutPer1M: 10, ContextWindow: 1048576, Tools: true, Vision: true},
			{UpstreamName: "gemini-2.5-flash", DisplayName: "Gemini 2.5 Flash", PriceInPer1M: 0.30, PriceOutPer1M: 2.5, ContextWindow: 1048576, Tools: true, Vision: true},
			{UpstreamName: "gemini-2.5-flash-lite", DisplayName: "Gemini 2.5 Flash Lite", PriceInPer1M: 0.10, PriceOutPer1M: 0.4, ContextWindow: 1048576, Tools: true},
		},
	},
	{
		Type: ProviderOpenAICompat, Name: "OpenRouter", Prefix: "or",
		BaseURL:  "https://openrouter.ai/api/v1",
		Settings: CredentialSettings{Strategy: CredStrategyRoundRobin},
		DocsURL:  "https://openrouter.ai/keys",
		Models: []TemplateModel{
			{UpstreamName: "openrouter/auto", DisplayName: "Auto (best)", ContextWindow: 200000, Tools: true},
			{UpstreamName: "deepseek/deepseek-chat-v3.1", DisplayName: "DeepSeek V3.1", PriceInPer1M: 0.28, PriceOutPer1M: 0.88, ContextWindow: 163840, Tools: true},
			{UpstreamName: "qwen/qwen3-235b-a22b", DisplayName: "Qwen3 235B", PriceInPer1M: 0.2, PriceOutPer1M: 0.6, ContextWindow: 131072, Tools: true},
		},
	},
	{
		Type: ProviderOpenAICompat, Name: "Groq", Prefix: "gq",
		BaseURL:  "https://api.groq.com/openai/v1",
		Settings: CredentialSettings{Strategy: CredStrategyRoundRobin},
		DocsURL:  "https://console.groq.com/keys",
		Models: []TemplateModel{
			{UpstreamName: "llama-3.3-70b-versatile", DisplayName: "Llama 3.3 70B", ContextWindow: 131072, Tools: true},
			{UpstreamName: "llama-3.1-8b-instant", DisplayName: "Llama 3.1 8B", ContextWindow: 131072, Tools: true},
		},
	},
	{
		Type: ProviderOpenAICompat, Name: "DeepSeek", Prefix: "ds",
		BaseURL:  "https://api.deepseek.com/v1",
		Settings: CredentialSettings{Strategy: CredStrategyRoundRobin},
		DocsURL:  "https://platform.deepseek.com/api_keys",
		Models: []TemplateModel{
			{UpstreamName: "deepseek-chat", DisplayName: "DeepSeek Chat", PriceInPer1M: 0.27, PriceOutPer1M: 1.1, ContextWindow: 131072, Tools: true},
			{UpstreamName: "deepseek-reasoner", DisplayName: "DeepSeek Reasoner", PriceInPer1M: 0.55, PriceOutPer1M: 2.19, ContextWindow: 131072},
		},
	},
	{
		Type: ProviderOpenAICompat, Name: "Mistral", Prefix: "mi",
		BaseURL:  "https://api.mistral.ai/v1",
		Settings: CredentialSettings{Strategy: CredStrategyRoundRobin},
		DocsURL:  "https://console.mistral.ai/api-keys",
		Models: []TemplateModel{
			{UpstreamName: "mistral-large-latest", DisplayName: "Mistral Large", PriceInPer1M: 2, PriceOutPer1M: 6, ContextWindow: 131072, Tools: true},
			{UpstreamName: "mistral-small-latest", DisplayName: "Mistral Small", PriceInPer1M: 0.2, PriceOutPer1M: 0.6, ContextWindow: 131072, Tools: true},
		},
	},
	{
		Type: ProviderOpenAICompat, Name: "Together", Prefix: "tg",
		BaseURL:  "https://api.together.xyz/v1",
		Settings: CredentialSettings{Strategy: CredStrategyRoundRobin},
		DocsURL:  "https://api.together.ai/settings/api-keys",
		Models: []TemplateModel{
			{UpstreamName: "meta-llama/Llama-3.3-70B-Instruct-Turbo", DisplayName: "Llama 3.3 70B Turbo", PriceInPer1M: 0.88, PriceOutPer1M: 0.88, ContextWindow: 131072, Tools: true},
			{UpstreamName: "Qwen/Qwen2.5-72B-Instruct-Turbo", DisplayName: "Qwen2.5 72B Turbo", PriceInPer1M: 1.2, PriceOutPer1M: 1.2, ContextWindow: 32768, Tools: true},
		},
	},
	{
		Type: ProviderOpenAICompat, Name: "GLM (Zhipu)", Prefix: "glm",
		BaseURL:  "https://open.bigmodel.cn/api/paas/v4",
		Settings: CredentialSettings{Strategy: CredStrategyRoundRobin},
		DocsURL:  "https://open.bigmodel.cn/usercenter/apikeys",
		Models: []TemplateModel{
			{UpstreamName: "glm-4.6", DisplayName: "GLM-4.6", PriceInPer1M: 0.6, PriceOutPer1M: 2.2, ContextWindow: 200000, Tools: true},
			{UpstreamName: "glm-4.5-air", DisplayName: "GLM-4.5 Air", PriceInPer1M: 0.2, PriceOutPer1M: 1.1, ContextWindow: 131072, Tools: true},
		},
	},
	{
		Type: ProviderOpenAICompat, Name: "MiniMax", Prefix: "mm",
		BaseURL:  "https://api.minimax.chat/v1",
		Settings: CredentialSettings{Strategy: CredStrategyRoundRobin},
		DocsURL:  "https://platform.minimaxi.com",
		Models: []TemplateModel{
			{UpstreamName: "MiniMax-M2", DisplayName: "MiniMax M2", ContextWindow: 200000, Tools: true},
		},
	},
	{
		Type: ProviderOpenAICompat, Name: "Kimi (Moonshot)", Prefix: "km",
		BaseURL:  "https://api.moonshot.ai/v1",
		Settings: CredentialSettings{Strategy: CredStrategyRoundRobin},
		DocsURL:  "https://platform.moonshot.cn/console/api-keys",
		Models: []TemplateModel{
			{UpstreamName: "kimi-k2-0905-preview", DisplayName: "Kimi K2", PriceInPer1M: 0.6, PriceOutPer1M: 2.5, ContextWindow: 262144, Tools: true},
			{UpstreamName: "moonshot-v1-128k", DisplayName: "Moonshot V1 128K", PriceInPer1M: 0.85, PriceOutPer1M: 0.85, ContextWindow: 131072, Tools: true},
		},
	},
	{
		// Provider lokal di server yang sama (PRD §6). Data tidak keluar server.
		Type: ProviderLlamaStash, Name: "LlamaStash (lokal)", Prefix: "local",
		BaseURL: "http://127.0.0.1:11435/v1",
		Settings: CredentialSettings{
			Strategy: CredStrategyRoundRobin, LocalConcurrency: 2, LocalQueueTimeout: 30000,
			ConnectTimeoutMs: 5000, ColdStartTimeoutMs: 120000,
		},
		DocsURL: "https://llamastash.dev/",
		Models:  []TemplateModel{}, // diisi via sinkronisasi GET /v1/models (FR-6.2)
	},
}

// FindTemplate mencari template berdasarkan prefix.
func FindTemplate(prefix string) *ProviderTemplate {
	for i := range ProviderTemplates {
		if ProviderTemplates[i].Prefix == prefix {
			return &ProviderTemplates[i]
		}
	}
	return nil
}

// SeedProvidersFromTemplate membuat provider + model dari template.
func (s *Store) SeedProvidersFromTemplate(t *ProviderTemplate) (*Provider, error) {
	p, err := s.CreateProvider(t.Type, t.Name, t.Prefix, t.BaseURL, t.Settings, true)
	if err != nil {
		return nil, err
	}
	for _, tm := range t.Models {
		if _, err := s.CreateModel(p.ID, tm.UpstreamName, "", orDisplayName(tm), tm.PriceInPer1M, tm.PriceOutPer1M, tm.ContextWindow, ModelCap{Tools: tm.Tools, Vision: tm.Vision}, true); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func orDisplayName(tm TemplateModel) string {
	if tm.DisplayName != "" {
		return tm.DisplayName
	}
	return tm.UpstreamName
}

package handlers

// OpenAI-compatible request/response types.
// Permite que clientes como OpenClaude, Aider, Continue, Open WebUI e LiteLLM
// utilizem esta API como drop-in replacement de uma API OpenAI.

// ── Request Types ───────────────────────────────────────────────────────────

// OpenAIChatRequest é o payload do POST /v1/chat/completions.
type OpenAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Stream      bool            `json:"stream,omitempty"`
	Temperature *float64        `json:"temperature,omitempty"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
	ToolChoice  any             `json:"tool_choice,omitempty"`
}

// OpenAIMessage representa uma mensagem no formato OpenAI.
// Content pode ser string ou []ContentPart (multimodal).
type OpenAIMessage struct {
	Role       string          `json:"role"`
	Content    any             `json:"content"`
	Name       string          `json:"name,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

// ContentPart representa uma parte de conteúdo multimodal.
type ContentPart struct {
	Type     string    `json:"type"` // "text" ou "image_url"
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL contém a URL de uma imagem para mensagens multimodal.
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// OpenAITool descreve uma tool disponível no formato OpenAI.
type OpenAITool struct {
	Type     string         `json:"type"`
	Function OpenAIFunction `json:"function"`
}

// OpenAIFunction descreve a function de uma tool OpenAI.
type OpenAIFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// OpenAIToolCall representa uma chamada de tool feita pelo assistant.
type OpenAIToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function OpenAIFuncCall `json:"function"`
}

// OpenAIFuncCall contém nome e argumentos de uma chamada de function.
type OpenAIFuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ── Response Types ──────────────────────────────────────────────────────────

// OpenAIChatResponse é a resposta do POST /v1/chat/completions (non-streaming).
type OpenAIChatResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
}

// OpenAIChoice representa uma escolha na resposta.
type OpenAIChoice struct {
	Index        int            `json:"index"`
	Message      *OpenAIMessage `json:"message,omitempty"`
	Delta        *OpenAIDelta   `json:"delta,omitempty"`
	FinishReason *string        `json:"finish_reason"`
}

// OpenAIDelta é o fragmento incremental em streaming SSE.
type OpenAIDelta struct {
	Role      string          `json:"role,omitempty"`
	Content   string          `json:"content,omitempty"`
	ToolCalls []OpenAIToolCall `json:"tool_calls,omitempty"`
}

// OpenAIUsage contém métricas de uso de tokens.
type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ── Models Types ────────────────────────────────────────────────────────────

// OpenAIModelsResponse é a resposta do GET /v1/models.
type OpenAIModelsResponse struct {
	Object string        `json:"object"`
	Data   []OpenAIModel `json:"data"`
}

// OpenAIModel representa um modelo no formato OpenAI.
type OpenAIModel struct {
	ID         string   `json:"id"`
	Object     string   `json:"object"`
	Created    int64    `json:"created"`
	OwnedBy   string   `json:"owned_by"`
	Permission []string `json:"permission"`
	Root       string   `json:"root"`
	Parent     *string  `json:"parent"`
}

// ── Error Types ─────────────────────────────────────────────────────────────

// OpenAIErrorResponse é o formato de erro padrão da API OpenAI.
type OpenAIErrorResponse struct {
	Error OpenAIErrorDetail `json:"error"`
}

// OpenAIErrorDetail contém os detalhes do erro.
type OpenAIErrorDetail struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param"`
	Code    *string `json:"code"`
}

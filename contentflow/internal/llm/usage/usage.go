package usage

// Usage holds token counts returned by the provider after a completion call.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

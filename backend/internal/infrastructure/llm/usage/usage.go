package usage

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	// FinishReason is the provider's stop/finish reason (e.g. "max_tokens",
	// "length"); used to detect silently truncated responses.
	FinishReason string `json:"finish_reason"`
}

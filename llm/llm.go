package llm

func NewClient(provider string) (LLM, error) {
	var llm LLM
	if provider == "anthropic" {
		llm = NewAnthropicClient()
	}

	return llm, nil
}

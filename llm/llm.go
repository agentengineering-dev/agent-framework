package llm

func NewClient(provider string) (LLM, error) {
	var llm LLM
	if provider == "anthropic" {
		llm = NewAnthropicClient()
	} else if provider == "openai" {
		llm = NewOpenAILLM()
	}

	return llm, nil
}

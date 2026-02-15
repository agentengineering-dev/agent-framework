package llm

import "fmt"

func NewClient(provider string) (LLM, error) {
	var llm LLM
	var err error
	switch provider {
	case "google":
		llm, err = NewGoogleClient()
		if err != nil {
			return nil, err
		}
	case "anthropic":
		llm = NewAnthropicClient()
	case "openai":
		llm = NewOpenAILLM()
	case "ollama":
		llm, err = NewOllamaLLM()
	case "deepseek":
		llm = NewDeepSeekLLM()
	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
	return llm, nil
}

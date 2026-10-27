// Package amazonbedrock provides the Enterprise Deferred Amazon Bedrock integration
// via the OpenAI-compatible Chat Completions path (Bedrock API key). Native SigV4/IAM
// and Converse/Agents runtimes remain out of scope until a separate ADR.
package amazonbedrock

import (
	"github.com/kprompt/kprompt/internal/llm"
)

// RuntimeOpenAIBaseURL returns the regional Bedrock OpenAI-compat base URL.
func RuntimeOpenAIBaseURL(region string) (string, error) {
	return llm.RuntimeOpenAIBaseURL(region)
}

// NewCompat returns an llm.Provider backed by Bedrock's OpenAI-compatible endpoint.
// region is required when baseURL is empty (see llm.ResolveBedrockBaseURL).
func NewCompat(apiKey, baseURL, region, model string) (llm.Provider, error) {
	bu := baseURL
	if bu == "" && region != "" {
		var err error
		bu, err = llm.RuntimeOpenAIBaseURL(region)
		if err != nil {
			return nil, err
		}
	}
	return llm.New("bedrock", apiKey, bu, model)
}

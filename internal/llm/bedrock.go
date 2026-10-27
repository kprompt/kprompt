package llm

import (
	"fmt"
	"os"
	"strings"
)

// RuntimeOpenAIBaseURL returns the Amazon Bedrock OpenAI-compatible Chat Completions base URL
// for the given AWS region (bedrock-runtime endpoint).
func RuntimeOpenAIBaseURL(region string) (string, error) {
	region = strings.TrimSpace(region)
	if region == "" {
		return "", fmt.Errorf("bedrock: region required")
	}
	return fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/openai/v1", region), nil
}

// ResolveBedrockBaseURL selects the Bedrock OpenAI-compat base URL from config override,
// KPROMPT_OPENAI_BASE_URL, or a region from KPROMPT_BEDROCK_REGION / AWS_REGION / AWS_DEFAULT_REGION.
func ResolveBedrockBaseURL(configBaseURL string) (string, error) {
	if bu := strings.TrimRight(strings.TrimSpace(configBaseURL), "/"); bu != "" {
		return bu, nil
	}
	if bu := strings.TrimRight(os.Getenv("KPROMPT_OPENAI_BASE_URL"), "/"); bu != "" {
		return bu, nil
	}
	region := firstEnv("KPROMPT_BEDROCK_REGION", "AWS_REGION", "AWS_DEFAULT_REGION")
	if region == "" {
		return "", fmt.Errorf("provider bedrock requires base_url (config or KPROMPT_OPENAI_BASE_URL) or region (KPROMPT_BEDROCK_REGION / AWS_REGION)")
	}
	return RuntimeOpenAIBaseURL(region)
}

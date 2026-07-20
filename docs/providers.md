# LLM providers

Select with `--provider` / `~/.kprompt/config.yaml` (`provider`, `model`, optional `base_url`).

API keys are **environment variables only** (never stored in the config file).

| Provider | `--provider` | Env key(s) | Default model | Notes |
|----------|--------------|------------|---------------|-------|
| OpenAI | `openai` | `KPROMPT_OPENAI_API_KEY` / `OPENAI_API_KEY` | `gpt-4o-mini` | Also `KPROMPT_OPENAI_BASE_URL` for proxies; Team orgs can store keys in app `/secrets` and `kprompt secrets pull` |
| Anthropic | `anthropic` | `KPROMPT_ANTHROPIC_API_KEY` / `ANTHROPIC_API_KEY` | `claude-sonnet-4-20250514` | Messages API |
| Google Gemini | `gemini` | `KPROMPT_GEMINI_API_KEY` / `GEMINI_API_KEY` / `GOOGLE_API_KEY` | `gemini-2.0-flash` | AI Studio key |
| Groq | `groq` | `KPROMPT_GROQ_API_KEY` / `GROQ_API_KEY` | `llama-3.3-70b-versatile` | OpenAI-compatible |
| Mistral | `mistral` | `KPROMPT_MISTRAL_API_KEY` / `MISTRAL_API_KEY` | `mistral-small-latest` | OpenAI-compatible |
| DeepSeek | `deepseek` | `KPROMPT_DEEPSEEK_API_KEY` / `DEEPSEEK_API_KEY` | `deepseek-chat` | OpenAI-compatible |
| OpenRouter | `openrouter` | `KPROMPT_OPENROUTER_API_KEY` / `OPENROUTER_API_KEY` | `openai/gpt-4o-mini` | OpenAI-compatible |
| Together | `together` | `KPROMPT_TOGETHER_API_KEY` / `TOGETHER_API_KEY` | Llama 3.1 8B Turbo | OpenAI-compatible |
| Ollama (local) | `ollama` | optional | `llama3.2` | `http://127.0.0.1:11434/v1` — no key required |
| Generic OpenAI-compat | `openai-compatible` | `KPROMPT_OPENAI_API_KEY` | — | **Requires** `base_url` |

## Examples

```bash
# OpenAI
export KPROMPT_OPENAI_API_KEY=sk-...
kprompt "list deployments"

# Anthropic
export KPROMPT_ANTHROPIC_API_KEY=sk-ant-...
kprompt --provider anthropic "explain why api is crashing"

# Gemini
export KPROMPT_GEMINI_API_KEY=...
kprompt --provider gemini --model gemini-2.0-flash "deploy redis"

# Groq
export KPROMPT_GROQ_API_KEY=...
kprompt --provider groq "scale api to 3"

# Local Ollama (ollama serve + ollama pull llama3.2)
kprompt --provider ollama --model llama3.2 "list pods"

# Azure / custom gateway
export KPROMPT_OPENAI_API_KEY=...
export KPROMPT_OPENAI_BASE_URL=https://YOUR_RESOURCE.openai.azure.com/openai/v1
kprompt --provider openai-compatible --model gpt-4o "list services"
```

## Config file (`~/.kprompt/config.yaml`)

```yaml
provider: gemini
model: gemini-2.0-flash
# base_url: https://api.groq.com/openai/v1   # optional override for openai-compatible presets
namespace: default
```

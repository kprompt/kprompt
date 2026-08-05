# LLM providers

Select with `--provider` / `~/.kprompt/config.yaml` (`provider`, `model`, optional `base_url`), or run **`kprompt init`** once.

kprompt does **not** sell API keys. For natural-language plans you either:

1. **Ollama locally ($0)** — `kprompt init --ollama` (no cloud key)
2. **BYOK** — `kprompt init --provider …` + your own env key (never stored in the config file)

With **no** `provider` in config and no `--provider` flag, the CLI is **unconfigured** (it does not silently default to OpenAI). Bare `kprompt` coaches you toward `init`.

Optional Team `kp_…` tokens (`kprompt login`) are for org policy/audit — not LLM inference.

| Provider | `--provider` | Env key(s) | Default model | Notes |
|----------|--------------|------------|---------------|-------|
| Ollama (local) | `ollama` | none required | `llama3.2` | **$0 first path** — `http://127.0.0.1:11434/v1` |
| OpenAI | `openai` | `KPROMPT_OPENAI_API_KEY` / `OPENAI_API_KEY` | `gpt-4o-mini` | Also `KPROMPT_OPENAI_BASE_URL` for proxies; Team orgs can store keys in app `/secrets` and `kprompt secrets pull` |
| Anthropic | `anthropic` | `KPROMPT_ANTHROPIC_API_KEY` / `ANTHROPIC_API_KEY` | `claude-sonnet-4-20250514` | Messages API |
| Google Gemini | `gemini` | `KPROMPT_GEMINI_API_KEY` / `GEMINI_API_KEY` / `GOOGLE_API_KEY` | `gemini-2.0-flash` | AI Studio key; see free-tier notes below |
| Groq | `groq` | `KPROMPT_GROQ_API_KEY` / `GROQ_API_KEY` | `llama-3.3-70b-versatile` | OpenAI-compatible |
| xAI (Grok) | `xai` | `KPROMPT_XAI_API_KEY` / `XAI_API_KEY` | `grok-4.5` | OpenAI-compatible |
| Cerebras | `cerebras` | `KPROMPT_CEREBRAS_API_KEY` / `CEREBRAS_API_KEY` | `gpt-oss-120b` | OpenAI-compatible, low-latency |
| Mistral | `mistral` | `KPROMPT_MISTRAL_API_KEY` / `MISTRAL_API_KEY` | `mistral-small-latest` | OpenAI-compatible |
| DeepSeek | `deepseek` | `KPROMPT_DEEPSEEK_API_KEY` / `DEEPSEEK_API_KEY` | `deepseek-chat` | OpenAI-compatible |
| Moonshot (Kimi K3) | `moonshot` | `KPROMPT_MOONSHOT_API_KEY` / `MOONSHOT_API_KEY` | `kimi-k3` | OpenAI-compatible |
| OpenRouter | `openrouter` | `KPROMPT_OPENROUTER_API_KEY` / `OPENROUTER_API_KEY` | `openai/gpt-4o-mini` | OpenAI-compatible |
| Together | `together` | `KPROMPT_TOGETHER_API_KEY` / `TOGETHER_API_KEY` | Llama 3.1 8B Turbo | OpenAI-compatible |
| Generic OpenAI-compat | `openai-compatible` | `KPROMPT_OPENAI_API_KEY` | — | **Requires** `base_url` |

## Gemini free tier (honest)

AI Studio keys often start on a **free tier** with daily / per-minute quotas (input tokens, requests). Exceeding them returns **HTTP 429** — that is Google’s limit, not a kprompt bug.

```bash
export KPROMPT_GEMINI_API_KEY=...   # from https://aistudio.google.com/apikey
kprompt config set provider gemini
kprompt config set model gemini-2.0-flash   # docs default; or a current Flash / Flash-Lite id
```

| Symptom | What to do |
|---------|------------|
| `429` / `generate_content_free_tier_*` | Wait for quota reset, enable billing on that Google project (calls become paid), or switch to Ollama |
| `404` model “no longer available to new users” | Pick a current model id (e.g. `gemini-3.1-flash-lite` / `gemini-3.5-flash`) — older `gemini-2.5-flash-lite` may be closed to new keys |
| Works in CLI, Team `/run` fails | Bridge uses the **same machine** env/secrets — export the key (or `secrets pull`) where `kprompt run listen` runs |

Prefer **Ollama** when you want $0 with no cloud quota:

```bash
ollama pull llama3.2
kprompt init --ollama
# equivalent: kprompt config set provider ollama && kprompt config set model llama3.2
```

Monitor Google quotas: [ai.dev/rate-limit](https://ai.dev/rate-limit).

## Examples

```bash
# Local Ollama ($0 — ollama serve + ollama pull llama3.2)
kprompt init --ollama
kprompt "list pods"
# or one-shot: kprompt --provider ollama --model llama3.2 "list pods"

# OpenAI
export KPROMPT_OPENAI_API_KEY=sk-...
kprompt init --provider openai
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

# xAI / Grok
export KPROMPT_XAI_API_KEY=...
kprompt --provider xai "explain why api is crashlooping"

# Cerebras
export KPROMPT_CEREBRAS_API_KEY=...
kprompt --provider cerebras "list pods"

# Moonshot / Kimi K3
export KPROMPT_MOONSHOT_API_KEY=...
kprompt --provider moonshot "explain why api is crashlooping"

# Azure / custom gateway
export KPROMPT_OPENAI_API_KEY=...
export KPROMPT_OPENAI_BASE_URL=https://YOUR_RESOURCE.openai.azure.com/openai/v1
kprompt --provider openai-compatible --model gpt-4o "list services"
```

## Config file (`~/.kprompt/config.yaml`)

```yaml
provider: ollama
model: llama3.2
# provider: gemini
# model: gemini-2.0-flash
# base_url: https://api.groq.com/openai/v1   # optional override for openai-compatible presets
namespace: default
```

# Amazon Bedrock (Enterprise Deferred, compat path)

kprompt ships **P-008** as an OpenAI-compatible Chat Completions integration against the
[`bedrock-runtime`](https://docs.aws.amazon.com/bedrock/latest/userguide/inference-chat-completions.html)
endpoint. Authentication uses an **Amazon Bedrock API key** (Bearer token), matching kprompt's
BYOK env-key model.

## CLI (named preset)

```bash
export KPROMPT_BEDROCK_API_KEY=...          # or AWS_BEARER_TOKEN_BEDROCK
export KPROMPT_BEDROCK_REGION=us-east-1     # or AWS_REGION / AWS_DEFAULT_REGION
kprompt config set provider bedrock
kprompt config set model openai.gpt-oss-120b-1:0
kprompt "list pods in kube-system"
```

Override the regional endpoint explicitly when needed:

```bash
export KPROMPT_OPENAI_BASE_URL=https://bedrock-runtime.us-west-2.amazonaws.com/openai/v1
kprompt --provider bedrock --model openai.gpt-oss-20b-1:0 "explain this deployment"
```

## Go (enterprise package)

For embedders that prefer the enterprise import path:

```go
import "github.com/kprompt/kprompt/enterprise/provider/amazonbedrock"

p, err := amazonbedrock.NewCompat(apiKey, "", "us-east-1", "openai.gpt-oss-120b-1:0")
```

`NewCompat` returns the shared `internal/llm.Provider` interface used by the CLI pipeline.

## Honest non-goals (deferred)

- **SigV4 / IAM** — kprompt does not read `~/.aws/credentials` for LLM calls today.
- **Bedrock Converse / Agents** — not implemented; use Chat Completions compat or a gateway.
- **Gateway-only Bedrock** — if you already front Bedrock with a generic OpenAI proxy, keep using
  `--provider openai-compatible` + `base_url` (see [providers.md](../providers.md)).

See also [kprompt on AWS (EKS)](https://kprompt.ai/blog/kprompt-on-aws-eks).

package intent

import (
	"regexp"
	"strings"

	"github.com/kprompt/kprompt/internal/tools/crossplane"
)

var crossplanePromptPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bcrossplane\b`),
	regexp.MustCompile(`(?i)\b(provision|claim)\b.*\b(postgres|postgresql|database|bucket|s3|redis|cloud)\b`),
	regexp.MustCompile(`(?i)\b(postgres|postgresql|database|bucket|s3|redis)\b.*\b(provision|claim|crossplane)\b`),
	regexp.MustCompile(`(?i)\bprovision\s+(a\s+)?(postgres|postgresql|database|bucket|redis)\b`),
	regexp.MustCompile(`(?i)\bcloud\s+(resource|claim)\b`),
}

// LooksLikeCrossplanePrompt reports natural-language that needs Crossplane claims (T-042).
func LooksLikeCrossplanePrompt(prompt string) bool {
	for _, re := range crossplanePromptPatterns {
		if re.MatchString(prompt) {
			return true
		}
	}
	return false
}

// NormalizeCrossplane maps provision/claim prompts onto kind=crossplane.
func NormalizeCrossplane(in Intent, prompt string) Intent {
	if !LooksLikeCrossplanePrompt(prompt) {
		return in
	}
	switch in.Kind {
	case KindDeploy, KindInstall, KindUnknown, KindCrossplane, KindGet:
		in.Kind = KindCrossplane
	default:
		if in.Kind != KindCrossplane {
			return in
		}
	}
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	p := strings.ToLower(prompt)
	if _, ok := in.StringParam("resource"); !ok {
		switch {
		case strings.Contains(p, "redis") || strings.Contains(p, "cache"):
			in.Params["resource"] = "redis"
		case strings.Contains(p, "bucket") || strings.Contains(p, "s3"):
			in.Params["resource"] = "bucket"
		case strings.Contains(p, "postgres") || strings.Contains(p, "postgresql") || strings.Contains(p, "database"):
			in.Params["resource"] = "postgres"
		default:
			in.Params["resource"] = "postgres"
		}
	}
	if strings.TrimSpace(in.Target.Kind) == "" {
		resource, _ := in.StringParam("resource")
		switch resource {
		case "bucket":
			in.Target.Kind = "Bucket"
		case "redis":
			in.Target.Kind = "RedisInstance"
		default:
			in.Target.Kind = "PostgreSQLInstance"
		}
	}
	if strings.TrimSpace(in.Target.Name) == "" {
		resource, _ := in.StringParam("resource")
		in.Target.Name = crossplane.DefaultClaimName(resource)
	}
	return in
}

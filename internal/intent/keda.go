package intent

import (
	"regexp"
	"strings"

	"github.com/kprompt/kprompt/internal/tools/keda"
)

var kedaPromptPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bkeda\b`),
	regexp.MustCompile(`(?i)\bscaled\s*object\b`),
	regexp.MustCompile(`(?i)\bscale\s+to\s+zero\b.*\b(keda|event|queue)\b`),
	regexp.MustCompile(`(?i)\b(event[- ]driven|scale[- ]to[- ]zero)\b`),
	regexp.MustCompile(`(?i)\bautoscal(e|ing)\b.*\b(queue|redis|http|event)\b`),
	regexp.MustCompile(`(?i)\b(queue|redis)\b.*\b(autoscale|scaledobject|keda)\b`),
}

// LooksLikeKEDAPrompt reports natural-language that needs KEDA ScaledObjects (T-040).
func LooksLikeKEDAPrompt(prompt string) bool {
	for _, re := range kedaPromptPatterns {
		if re.MatchString(prompt) {
			return true
		}
	}
	return false
}

// NormalizeKEDA maps KEDA/scale-to-zero prompts onto kind=keda (not KindScale).
func NormalizeKEDA(in Intent, prompt string) Intent {
	if !LooksLikeKEDAPrompt(prompt) {
		return in
	}
	switch in.Kind {
	case KindScale, KindDeploy, KindUnknown, KindKEDA, KindGet:
		in.Kind = KindKEDA
	default:
		if in.Kind != KindKEDA {
			return in
		}
	}
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	p := strings.ToLower(prompt)
	if _, ok := in.StringParam("trigger"); !ok {
		switch {
		case strings.Contains(p, "redis") || strings.Contains(p, "queue"):
			in.Params["trigger"] = "redis"
		case strings.Contains(p, "cron") || strings.Contains(p, "idle"):
			in.Params["trigger"] = "cron"
		case strings.Contains(p, "http") || strings.Contains(p, "prometheus"):
			in.Params["trigger"] = "cpu"
		default:
			in.Params["trigger"] = "cpu"
		}
	}
	if strings.Contains(p, "scale to zero") || strings.Contains(p, "scale-to-zero") ||
		strings.Contains(p, "to zero") || strings.Contains(p, "to 0") {
		if _, ok := in.Params["minReplicas"]; !ok {
			in.Params["minReplicas"] = 0
		}
	}
	if strings.TrimSpace(in.Target.Kind) == "" {
		in.Target.Kind = "ScaledObject"
	}
	if strings.TrimSpace(in.Target.Name) != "" {
		if _, ok := in.StringParam("target"); !ok {
			// Target name is the Deployment; ScaledObject name derived later.
			in.Params["target"] = in.Target.Name
		}
	}
	trigger, _ := in.StringParam("trigger")
	target := strings.TrimSpace(in.Target.Name)
	if t, ok := in.StringParam("target"); ok {
		target = t
	}
	if soName := keda.DefaultScaledObjectName(target, trigger); strings.TrimSpace(in.Target.Name) == "" {
		in.Target.Name = soName
	}
	return in
}

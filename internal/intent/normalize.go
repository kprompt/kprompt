package intent

import "strings"

// NormalizeVerb adjusts intent kind using prompt phrasing when the model confuses install vs deploy.
func NormalizeVerb(in Intent, prompt string) Intent {
	p := strings.ToLower(strings.TrimSpace(prompt))
	if strings.HasPrefix(p, "install ") && in.Kind == KindDeploy {
		in.Kind = KindInstall
	}
	return in
}

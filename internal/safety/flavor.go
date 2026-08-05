package safety

import (
	"fmt"
	"hash/fnv"
	"strings"
)

// Wipe-class hard-deny punchlines. Same prompt → same line (demo/README stable);
// different wipe prompts rotate through the pack.
var wipeDenyFlavors = []string{
	"Your cluster lives another day",
	"Nope. That prompt belongs in a horror movie, not a kubeconfig",
	"Plan refused — the etcd babysitter said absolutely not",
	"Denied. Your nodes just exhaled in relief",
	"Hard no. Try naming one Deployment instead of everything",
	"Safety first: mass extinction is not a supported Intent",
	"Blocked. The blast radius was approximately 'career-limiting'",
	"Not today. Keep the wipe fantasies in a scratch kind cluster",
	"Refused. Control plane would like to remain employed",
	"Denied with prejudice — unscoped delete is not a personality",
	"Cluster vetoed that one. Democracy wins",
	"Nice try. Named targets only — chaos needs a ticket",
}

// WipeDenyRemediation is the stable next-step line appended to wipe-class denies (OB-007).
const WipeDenyRemediation = `Next: kprompt "delete deployment <name>" -n <namespace>`

// WipeDenyMessage builds the human TTY/JSON message for a wipe-class prompt deny.
func WipeDenyMessage(prompt string) string {
	return fmt.Sprintf(
		"🚨 Intent: destructive cluster operation\n🛡️ Safe execution: denied\n😅 %s\n\n%s",
		wipeDenyPunchline(prompt),
		WipeDenyRemediation,
	)
}

func wipeDenyPunchline(prompt string) string {
	if len(wipeDenyFlavors) == 0 {
		return "Your cluster lives another day"
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(prompt))))
	return wipeDenyFlavors[int(h.Sum32())%len(wipeDenyFlavors)]
}

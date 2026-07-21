// Package doctor runs local health checks for `kprompt doctor`.
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/team"
	"github.com/kprompt/kprompt/internal/tools"
)

// Severity of a check outcome.
type Severity string

const (
	Pass Severity = "pass"
	Fail Severity = "fail"
	Skip Severity = "skip" // optional / not enrolled
	Warn Severity = "warn" // usable but degraded
)

// Check is one doctor row.
type Check struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Status  Severity `json:"status"`
	Detail  string   `json:"detail"`
	Hint    string   `json:"hint,omitempty"`
	Required bool    `json:"required"`
}

// Report is the full doctor result.
type Report struct {
	Checks  []Check   `json:"checks"`
	OK      bool      `json:"ok"`
	Checked time.Time `json:"checked_at"`
}

// Options configures Run.
type Options struct {
	Context string // kube context override
	// Me probes Team /v1/me when a token is present. nil = use real HTTP client.
	Me func(ctx context.Context, apiURL, token string) (team.MeResponse, error)
	// Detect overrides tools.Detect (tests).
	Detect func(ctx context.Context, opts tools.DetectOptions) (*tools.Registry, error)
}

// Run executes required + optional checks. Does not print secrets.
func Run(ctx context.Context, opts Options) (Report, error) {
	file, err := config.LoadFile()
	if err != nil {
		return Report{}, err
	}
	detect := opts.Detect
	if detect == nil {
		detect = tools.Detect
	}
	me := opts.Me
	if me == nil {
		me = defaultMe
	}

	var checks []Check
	checks = append(checks, checkConfig(file))
	checks = append(checks, checkLLM(file))

	reg, err := detect(ctx, tools.DetectOptions{
		Context: first(opts.Context, file.Context),
		File:    file,
	})
	if err != nil {
		checks = append(checks, Check{
			ID:       "kubernetes",
			Name:     "Kubernetes",
			Status:   Fail,
			Detail:   err.Error(),
			Hint:     "Fix kubeconfig / context, then re-run: kprompt doctor",
			Required: true,
		})
	} else {
		checks = append(checks, checkKubernetes(reg))
		checks = append(checks, checkToolsSummary(reg)...)
	}

	checks = append(checks, checkTeam(ctx, me))
	checks = append(checks, checkPolicyCache())
	checks = append(checks, checkPulledSecrets())

	rep := Report{Checks: checks, Checked: time.Now().UTC()}
	rep.OK = true
	for _, c := range checks {
		if c.Required && c.Status == Fail {
			rep.OK = false
			break
		}
	}
	return rep, nil
}

func checkConfig(file config.File) Check {
	path, err := config.DefaultPath()
	if err != nil {
		return Check{
			ID: "config", Name: "Config", Status: Fail, Required: true,
			Detail: err.Error(), Hint: "Ensure home directory is writable",
		}
	}
	prov := strings.TrimSpace(file.Provider)
	if prov == "" {
		prov = "openai (default)"
	}
	return Check{
		ID: "config", Name: "Config", Status: Pass, Required: true,
		Detail: fmt.Sprintf("%s · provider %s", path, prov),
	}
}

func checkLLM(file config.File) Check {
	r := config.Merge(file, "", "", "", "", false, "")
	preset, ok := llm.LookupPreset(r.Provider)
	if !ok {
		return Check{
			ID: "llm", Name: "LLM provider", Status: Fail, Required: true,
			Detail: fmt.Sprintf("unknown provider %q", r.Provider),
			Hint:   "kprompt config set provider openai",
		}
	}
	key := config.APIKeyFor(r.Provider)
	c := Check{ID: "llm", Name: "LLM provider", Required: true}
	if preset.AllowEmptyKey {
		c.Status = Pass
		c.Detail = fmt.Sprintf("%s · model %s · key optional (local)", r.Provider, r.Model)
		return c
	}
	if key == "" {
		c.Status = Fail
		c.Detail = fmt.Sprintf("%s · model %s · API key unset", r.Provider, r.Model)
		hints := strings.Join(preset.EnvKeys, " or ")
		c.Hint = fmt.Sprintf("export %s=… (or save a key at app.kprompt.ai/secrets and: kprompt secrets pull)", hints)
		return c
	}
	c.Status = Pass
	c.Detail = fmt.Sprintf("%s · model %s · API key set", r.Provider, r.Model)
	return c
}

func checkKubernetes(reg *tools.Registry) Check {
	r, ok := reg.Get(tools.IDKubernetes)
	if !ok {
		return Check{
			ID: "kubernetes", Name: "Kubernetes", Status: Fail, Required: true,
			Detail: "no detect result", Hint: tools.MissingHint(tools.IDKubernetes),
		}
	}
	c := Check{ID: "kubernetes", Name: "Kubernetes", Required: true, Detail: r.Detail, Hint: r.Hint}
	if r.Available() {
		c.Status = Pass
		c.Hint = ""
		return c
	}
	c.Status = Fail
	if c.Hint == "" {
		c.Hint = tools.MissingHint(tools.IDKubernetes)
	}
	return c
}

func checkToolsSummary(reg *tools.Registry) []Check {
	// Helm is the most common optional PATH tool; others are informational.
	out := make([]Check, 0, 2)
	if r, ok := reg.Get(tools.IDHelm); ok {
		c := Check{ID: "helm", Name: "Helm", Required: false, Detail: r.Detail, Hint: r.Hint}
		switch {
		case r.Available():
			c.Status = Pass
			c.Hint = ""
		case r.Status == tools.StatusDisabled:
			c.Status = Skip
		default:
			c.Status = Warn
			if c.Hint == "" {
				c.Hint = "Optional for chart installs. Coming soon: kprompt setup (T-061+)."
			}
		}
		out = append(out, c)
	}
	var avail, missing []string
	for _, r := range reg.All() {
		if r.ID == tools.IDKubernetes || r.ID == tools.IDHelm {
			continue
		}
		if r.Available() {
			avail = append(avail, string(r.ID))
		} else if r.Status != tools.StatusDisabled {
			missing = append(missing, string(r.ID))
		}
	}
	detail := fmt.Sprintf("available: %s", orDash(avail))
	if len(missing) > 0 {
		detail += fmt.Sprintf(" · not detected: %s", strings.Join(missing, ", "))
	}
	out = append(out, Check{
		ID: "integrations", Name: "Integrations", Status: Pass, Required: false,
		Detail: detail,
		Hint:   "See: kprompt tools",
	})
	return out
}

func checkTeam(ctx context.Context, me func(context.Context, string, string) (team.MeResponse, error)) Check {
	c := Check{ID: "team", Name: "Team enrollment", Required: false}
	creds, ok, err := team.LoadCredentials()
	if err != nil {
		c.Status = Warn
		c.Detail = err.Error()
		c.Hint = "Fix ~/.kprompt/credentials.yaml or run: kprompt logout"
		return c
	}
	token := team.ResolveToken(creds)
	if !ok || token == "" {
		c.Status = Skip
		c.Detail = "not enrolled (optional)"
		c.Hint = "kprompt login — approve at https://app.kprompt.ai/connect"
		return c
	}
	apiURL := team.ResolveAPIURL(creds)
	res, err := me(ctx, apiURL, token)
	if err != nil {
		c.Status = Fail
		c.Required = true // token present but broken → treat as required fail
		c.Detail = err.Error()
		c.Hint = "kprompt logout && kprompt login"
		return c
	}
	c.Status = Pass
	c.Detail = fmt.Sprintf("%s · %s (%s)", res.Org.Name, res.Member.Email, res.Member.Role)
	return c
}

func checkPolicyCache() Check {
	c := Check{ID: "policy", Name: "Org policy cache", Required: false}
	pol, ok, err := team.LoadPolicy()
	if err != nil {
		c.Status = Warn
		c.Detail = err.Error()
		return c
	}
	if !ok {
		c.Status = Skip
		c.Detail = "no cached policy"
		c.Hint = "After login: kprompt policy pull"
		return c
	}
	c.Status = Pass
	c.Detail = fmt.Sprintf("org %s · version %d · max_risk %s", pol.OrgID, pol.Version, pol.MaxRisk)
	return c
}

func checkPulledSecrets() Check {
	c := Check{ID: "secrets", Name: "Pulled provider keys", Required: false}
	path, err := config.ProviderSecretsPath()
	if err != nil {
		c.Status = Skip
		c.Detail = "n/a"
		return c
	}
	n, err := readPulledProviderCount(path)
	if err != nil {
		c.Status = Skip
		c.Detail = "no pulled secrets file"
		c.Hint = "Optional: kprompt secrets pull"
		return c
	}
	if n == 0 {
		c.Status = Skip
		c.Detail = "empty secrets cache"
		c.Hint = "Optional: save keys at app.kprompt.ai/secrets then kprompt secrets pull"
		return c
	}
	c.Status = Pass
	c.Detail = fmt.Sprintf("%d provider key(s) cached (values not shown)", n)
	return c
}

func defaultMe(ctx context.Context, apiURL, token string) (team.MeResponse, error) {
	client := team.NewClient(apiURL, token)
	return client.Me(ctx)
}

// FormatText writes a human table to w.
func FormatText(w io.Writer, rep Report) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "CHECK\tSTATUS\tDETAIL")
	for _, c := range rep.Checks {
		detail := sanitize(c.Detail)
		fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Name, strings.ToUpper(string(c.Status)), detail)
		if c.Hint != "" && (c.Status == Fail || c.Status == Warn || c.Status == Skip) {
			fmt.Fprintf(tw, "\t\t→ %s\n", sanitize(c.Hint))
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(w)
	if rep.OK {
		fmt.Fprintln(w, "Overall: OK — ready for prompts (optional checks may still be skipped).")
	} else {
		fmt.Fprintln(w, "Overall: FAIL — fix required checks above, then re-run: kprompt doctor")
	}
	return nil
}

// FormatJSON writes the report as JSON.
func FormatJSON(w io.Writer, rep Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

func first(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func orDash(ss []string) string {
	if len(ss) == 0 {
		return "—"
	}
	return strings.Join(ss, ", ")
}

func sanitize(s string) string {
	return strings.ReplaceAll(s, "\t", " ")
}

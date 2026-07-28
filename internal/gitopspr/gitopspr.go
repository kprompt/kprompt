// Package gitopspr opens or updates a GitHub pull request instead of applying
// a mutating plan to the cluster (T-072).
//
// Local MVP: requires gitops.repo (or KPROMPT_GITOPS_REPO) and an authenticated
// `gh` CLI (or GH_TOKEN / GITHUB_TOKEN). Team org repos (A-060) are not wired yet.
// Never mutates the cluster.
package gitopspr

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/tools/helm"
)

const (
	ModeApply = "apply"
	ModePR    = "pr"

	EnvMode       = "KPROMPT_GITOPS_MODE"
	EnvRepo       = "KPROMPT_GITOPS_REPO"
	EnvPath       = "KPROMPT_GITOPS_PATH"
	EnvBaseBranch = "KPROMPT_GITOPS_BASE_BRANCH"
)

// Settings controls PR-mode apply.
type Settings struct {
	Mode       string // apply | pr
	Repo       string // owner/name
	Path       string // path prefix inside the repo
	BaseBranch string
}

// LoadSettings merges file + env (env wins).
func LoadSettings(file config.File) Settings {
	s := Settings{
		Mode:       firstNonEmpty(os.Getenv(EnvMode), file.GitOps.Mode, ModeApply),
		Repo:       firstNonEmpty(os.Getenv(EnvRepo), file.GitOps.Repo),
		Path:       firstNonEmpty(os.Getenv(EnvPath), file.GitOps.Path, "kprompt"),
		BaseBranch: firstNonEmpty(os.Getenv(EnvBaseBranch), file.GitOps.BaseBranch, "main"),
	}
	s.Mode = strings.ToLower(strings.TrimSpace(s.Mode))
	if s.Mode == "" {
		s.Mode = ModeApply
	}
	s.Repo = strings.TrimSpace(s.Repo)
	s.Path = strings.Trim(strings.TrimSpace(s.Path), "/")
	s.BaseBranch = strings.TrimSpace(s.BaseBranch)
	return s
}

// Enabled reports whether apply should open a PR instead of mutating the cluster.
func (s Settings) Enabled() bool {
	switch s.Mode {
	case ModePR, "gitops", "pull-request", "pull_request":
		return true
	default:
		return false
	}
}

// Target is the human-facing PR destination shown in the plan.
type Target struct {
	Repo       string `json:"repo"`
	Path       string `json:"path"`
	BaseBranch string `json:"baseBranch"`
	Branch     string `json:"branch,omitempty"`
}

// FileChange is one path written into the PR branch.
type FileChange struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Result is the outcome of opening a PR.
type Result struct {
	Type       string       `json:"type"`
	URL        string       `json:"url,omitempty"`
	Repo       string       `json:"repo"`
	BaseBranch string       `json:"baseBranch"`
	Branch     string       `json:"branch"`
	Title      string       `json:"title"`
	Files      []FileChange `json:"files,omitempty"`
	Message    string       `json:"message,omitempty"`
}

// Runner opens PRs. Tests inject a fake.
type Runner interface {
	Open(ctx context.Context, target Target, title, body string, files []FileChange) (Result, error)
}

// Options configures OpenFromPlan.
type Options struct {
	Settings Settings
	Prompt   string
	Now      time.Time
	Runner   Runner // nil = GHRunner
}

// OpenFromPlan builds files from a plan and opens a GitHub PR. Never touches the cluster.
func OpenFromPlan(ctx context.Context, plan planner.ExecutionPlan, opts Options) (Result, error) {
	s := opts.Settings
	if !s.Enabled() {
		return Result{}, fmt.Errorf("gitops PR mode is not enabled (set --gitops or gitops.mode=pr)")
	}
	if err := s.ValidateConnected(); err != nil {
		return Result{}, err
	}
	files, err := FilesFromPlan(ctx, plan, s.Path)
	if err != nil {
		return Result{}, err
	}
	if len(files) == 0 {
		return Result{}, fmt.Errorf("gitops PR mode: plan produced no files to commit")
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	branch := fmt.Sprintf("kprompt/%s-%s", sanitizeFile(string(plan.Intent.Kind)), now.Format("20060102-150405"))
	title := prTitle(plan, opts.Prompt)
	body := prBody(plan, opts.Prompt, files)
	target := Target{
		Repo:       s.Repo,
		Path:       s.Path,
		BaseBranch: s.BaseBranch,
		Branch:     branch,
	}

	runner := opts.Runner
	if runner == nil {
		runner = GHRunner{}
	}
	res, err := runner.Open(ctx, target, title, body, files)
	if err != nil {
		return Result{}, err
	}
	res.Type = "gitops-pr"
	res.Repo = s.Repo
	res.BaseBranch = s.BaseBranch
	res.Branch = branch
	res.Title = title
	res.Files = files
	return res, nil
}

// ValidateConnected returns an honest error when SCM is not configured.
func (s Settings) ValidateConnected() error {
	if strings.TrimSpace(s.Repo) == "" {
		return fmt.Errorf("%s", NotConnectedMessage())
	}
	if !repoPattern.MatchString(s.Repo) {
		return fmt.Errorf("gitops.repo must look like owner/name (got %q)", s.Repo)
	}
	return nil
}

// NotConnectedMessage is the user-facing honesty line for missing SCM.
func NotConnectedMessage() string {
	return "GitOps PR mode requires an SCM repo. Set gitops.repo (or KPROMPT_GITOPS_REPO=owner/name) and authenticate gh (gh auth login) or set GH_TOKEN/GITHUB_TOKEN. Team org repos (A-060) are not wired yet — this is the local CLI path."
}

var repoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// FilesFromPlan extracts YAML files for the PR. MVP: deploy manifests and Helm
// template output. Scale/delete/sync are rejected.
func FilesFromPlan(ctx context.Context, plan planner.ExecutionPlan, pathPrefix string) ([]FileChange, error) {
	pathPrefix = strings.Trim(pathPrefix, "/")
	if pathPrefix == "" {
		pathPrefix = "kprompt"
	}

	switch plan.Intent.Kind {
	case intent.KindDeploy, intent.KindInstall, intent.KindUpgrade, intent.KindPatch:
		// ok
	default:
		return nil, fmt.Errorf(
			"gitops PR mode MVP supports deploy / helm install / helm upgrade plans (got kind=%s). Scale, delete, and live GitOps sync still use cluster apply — omit --gitops for those",
			plan.Intent.Kind,
		)
	}

	var out []FileChange
	repoURL := ""
	for _, a := range plan.Actions {
		if a.Op == planner.OpHelmRepo {
			if u := helm.RepoURLFromCommand(a.Command); u != "" {
				repoURL = u
			}
		}
	}

	for i, a := range plan.Actions {
		switch a.Op {
		case planner.OpHelmRepo, planner.OpHelmRepoUpdate:
			continue
		case planner.OpHelmInstall, planner.OpHelmUpgrade:
			body, err := helmManifestForPR(ctx, a, repoURL)
			if err != nil {
				return nil, err
			}
			name := a.Object.Name
			if name == "" {
				name = fmt.Sprintf("helm-%d", i+1)
			}
			out = append(out, FileChange{
				Path:    path.Join(pathPrefix, sanitizeFile(name)+".yaml"),
				Content: ensureTrailingNewline(body),
			})
		case planner.OpCreate, planner.OpUpdate:
			body := strings.TrimSpace(a.Manifest)
			if body == "" {
				return nil, fmt.Errorf("gitops PR mode: %s %s/%s has no manifest to commit", a.Op, a.Object.Kind, a.Object.Name)
			}
			if strings.Contains(body, "…(preview truncated)") {
				return nil, fmt.Errorf("gitops PR mode: manifest preview is truncated; cannot open a safe PR for %s/%s", a.Object.Kind, a.Object.Name)
			}
			name := a.Object.Name
			if name == "" {
				name = fmt.Sprintf("%s-%d", strings.ToLower(a.Object.Kind), i+1)
			}
			fname := sanitizeFile(strings.ToLower(a.Object.Kind) + "-" + name)
			out = append(out, FileChange{
				Path:    path.Join(pathPrefix, fname+".yaml"),
				Content: ensureTrailingNewline(body),
			})
		default:
			return nil, fmt.Errorf(
				"gitops PR mode does not support op %q yet (cluster apply only). Omit --gitops or use a deploy/helm plan",
				a.Op,
			)
		}
	}
	return out, nil
}

func helmManifestForPR(ctx context.Context, a planner.Action, repoURL string) (string, error) {
	var previewCmd []string
	var err error
	switch a.Op {
	case planner.OpHelmInstall:
		previewCmd, err = helm.PreviewInstallCommand(a.Command, repoURL)
	case planner.OpHelmUpgrade:
		previewCmd, err = helm.PreviewUpgradeCommand(a.Command)
	default:
		return "", fmt.Errorf("not a helm mutate op")
	}
	if err != nil {
		return "", fmt.Errorf("helm preview command: %w", err)
	}
	body, err := helm.RunCapture(ctx, previewCmd)
	if err != nil {
		if m := strings.TrimSpace(a.Manifest); m != "" && !strings.Contains(m, "…(preview truncated)") {
			return m, nil
		}
		return "", fmt.Errorf("helm template for PR: %w", err)
	}
	return body, nil
}

func prTitle(plan planner.ExecutionPlan, prompt string) string {
	if s := strings.TrimSpace(plan.Summary); s != "" {
		return "kprompt: " + truncate(s, 72)
	}
	if p := strings.TrimSpace(prompt); p != "" {
		return "kprompt: " + truncate(p, 72)
	}
	return fmt.Sprintf("kprompt: %s", plan.Intent.Kind)
}

func prBody(plan planner.ExecutionPlan, prompt string, files []FileChange) string {
	var b strings.Builder
	b.WriteString("## kprompt GitOps PR\n\n")
	b.WriteString("This pull request was opened **instead of applying to the cluster**.\n\n")
	if p := strings.TrimSpace(prompt); p != "" {
		fmt.Fprintf(&b, "**Prompt:** %s\n\n", p)
	}
	if s := strings.TrimSpace(plan.Summary); s != "" {
		fmt.Fprintf(&b, "**Plan:** %s\n\n", s)
	}
	b.WriteString("**Files:**\n")
	for _, f := range files {
		fmt.Fprintf(&b, "- `%s`\n", f.Path)
	}
	b.WriteString("\nReview, merge, and let Flux/Argo CD reconcile. Live cluster mutate was skipped.\n")
	return b.String()
}

func sanitizeFile(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "manifest"
	}
	return out
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func ensureTrailingNewline(s string) string {
	return strings.TrimRight(s, "\n") + "\n"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// GHRunner opens PRs via the GitHub CLI (`gh api`).
type GHRunner struct {
	LookPath func(file string) (string, error)
	Command  func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func (r GHRunner) lookPath(file string) (string, error) {
	if r.LookPath != nil {
		return r.LookPath(file)
	}
	return exec.LookPath(file)
}

func (r GHRunner) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if r.Command != nil {
		return r.Command(ctx, name, args...)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = os.Environ()
	return cmd.CombinedOutput()
}

func (r GHRunner) Open(ctx context.Context, target Target, title, body string, files []FileChange) (Result, error) {
	if _, err := r.lookPath("gh"); err != nil {
		return Result{}, fmt.Errorf("GitOps PR mode needs the GitHub CLI (`gh`) on PATH (or Team org repos when A-060 ships). Install: https://cli.github.com/ — also: %w", err)
	}
	if !hasGitHubToken() {
		if out, err := r.run(ctx, "gh", "auth", "status"); err != nil {
			return Result{}, fmt.Errorf("GitOps PR mode: gh is not authenticated (%v). Run: gh auth login — or export GH_TOKEN/GITHUB_TOKEN. Output: %s", err, truncate(string(out), 200))
		}
	}

	baseSHA, err := r.refSHA(ctx, target.Repo, target.BaseBranch)
	if err != nil {
		return Result{}, err
	}
	if err := r.createBranch(ctx, target.Repo, target.Branch, baseSHA); err != nil {
		return Result{}, err
	}
	for _, f := range files {
		if err := r.putFile(ctx, target.Repo, target.Branch, f, title); err != nil {
			return Result{}, err
		}
	}
	url, err := r.createPR(ctx, target.Repo, target.BaseBranch, target.Branch, title, body)
	if err != nil {
		return Result{}, err
	}
	return Result{URL: url, Message: "opened pull request"}, nil
}

func hasGitHubToken() bool {
	return strings.TrimSpace(os.Getenv("GH_TOKEN")) != "" || strings.TrimSpace(os.Getenv("GITHUB_TOKEN")) != ""
}

func (r GHRunner) refSHA(ctx context.Context, repo, branch string) (string, error) {
	out, err := r.run(ctx, "gh", "api", fmt.Sprintf("repos/%s/git/ref/heads/%s", repo, branch), "--jq", ".object.sha")
	if err != nil {
		return "", fmt.Errorf("read base branch %s on %s: %w (%s)", branch, repo, err, truncate(string(out), 200))
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return "", fmt.Errorf("empty SHA for %s@%s", repo, branch)
	}
	return sha, nil
}

func (r GHRunner) createBranch(ctx context.Context, repo, branch, sha string) error {
	out, err := r.run(ctx, "gh", "api",
		fmt.Sprintf("repos/%s/git/refs", repo),
		"--method", "POST",
		"-f", "ref=refs/heads/"+branch,
		"-f", "sha="+sha,
	)
	if err != nil {
		if strings.Contains(string(out), "Reference already exists") {
			return nil
		}
		return fmt.Errorf("create branch %s: %w (%s)", branch, err, truncate(string(out), 240))
	}
	return nil
}

func (r GHRunner) putFile(ctx context.Context, repo, branch string, f FileChange, message string) error {
	content := base64.StdEncoding.EncodeToString([]byte(f.Content))
	args := []string{
		"api", fmt.Sprintf("repos/%s/contents/%s", repo, f.Path),
		"--method", "PUT",
		"-f", "message=" + message,
		"-f", "content=" + content,
		"-f", "branch=" + branch,
	}
	out, err := r.run(ctx, "gh", args...)
	if err == nil {
		return nil
	}
	shaOut, shaErr := r.run(ctx, "gh", "api",
		fmt.Sprintf("repos/%s/contents/%s?ref=%s", repo, f.Path, branch),
		"--jq", ".sha",
	)
	if shaErr != nil {
		return fmt.Errorf("write %s: %w (%s)", f.Path, err, truncate(string(out), 240))
	}
	sha := strings.TrimSpace(string(shaOut))
	args = append(args, "-f", "sha="+sha)
	out, err = r.run(ctx, "gh", args...)
	if err != nil {
		return fmt.Errorf("update %s: %w (%s)", f.Path, err, truncate(string(out), 240))
	}
	return nil
}

func (r GHRunner) createPR(ctx context.Context, repo, base, head, title, body string) (string, error) {
	out, err := r.run(ctx, "gh", "api",
		fmt.Sprintf("repos/%s/pulls", repo),
		"--method", "POST",
		"-f", "title="+title,
		"-f", "head="+head,
		"-f", "base="+base,
		"-f", "body="+body,
		"--jq", ".html_url",
	)
	if err != nil {
		return "", fmt.Errorf("create pull request: %w (%s)", err, truncate(string(out), 240))
	}
	url := strings.TrimSpace(string(out))
	if url == "" {
		return "", fmt.Errorf("create pull request: empty url in response")
	}
	return url, nil
}

// MemRunner records Open calls for tests (no network).
type MemRunner struct {
	LastTitle  string
	LastBody   string
	LastFiles  []FileChange
	LastTarget Target
	URL        string
	Err        error
}

func (m *MemRunner) Open(_ context.Context, target Target, title, body string, files []FileChange) (Result, error) {
	m.LastTarget = target
	m.LastTitle = title
	m.LastBody = body
	m.LastFiles = append([]FileChange(nil), files...)
	if m.Err != nil {
		return Result{}, m.Err
	}
	url := m.URL
	if url == "" {
		url = "https://github.com/" + target.Repo + "/pull/1"
	}
	return Result{URL: url, Message: "opened pull request"}, nil
}

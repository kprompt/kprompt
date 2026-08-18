package intent_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/safety"
)

// Gold extract corpus: heuristic + safety always run in CI; live Extract is opt-in.
//
//	go test ./internal/intent/ -run TestGoldExtract
//	KPROMPT_GOLD_LLM=1 go test ./internal/intent/ -run 'TestGoldExtract/.*/extract' -count=1
//
// JSONL schema (one object per line):
//
//	id, pair, layers[] ("safety"|"normalize"|"extract"), prompt,
//	seed? {kind, target?, params?}, want {kind?, target?, params?, prompt_denied?},
//	want_not? {kinds?[]}, notes?

type goldCase struct {
	ID      string    `json:"id"`
	Pair    string    `json:"pair"`
	Layers  []string  `json:"layers"`
	Prompt  string    `json:"prompt"`
	Seed    *goldSeed `json:"seed,omitempty"`
	Want    goldWant  `json:"want"`
	WantNot *struct {
		Kinds []string `json:"kinds"`
	} `json:"want_not,omitempty"`
	Notes string `json:"notes,omitempty"`
}

type goldSeed struct {
	Kind   string         `json:"kind"`
	Target *intent.Target `json:"target,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

type goldWant struct {
	Kind         string         `json:"kind,omitempty"`
	Target       *intent.Target `json:"target,omitempty"`
	Params       map[string]any `json:"params,omitempty"`
	PromptDenied *bool          `json:"prompt_denied,omitempty"`
}

func TestGoldExtract(t *testing.T) {
	cases := loadGoldExtract(t)
	seen := make(map[string]struct{}, len(cases))
	stats := map[string]*goldPairStat{}

	for _, tc := range cases {
		tc := tc
		if _, dup := seen[tc.ID]; dup {
			t.Fatalf("duplicate gold id %q", tc.ID)
		}
		seen[tc.ID] = struct{}{}
		if tc.Prompt == "" {
			t.Fatalf("%s: empty prompt", tc.ID)
		}
		if len(tc.Layers) == 0 {
			t.Fatalf("%s: layers required", tc.ID)
		}
		pair := strings.TrimSpace(tc.Pair)
		if pair == "" {
			pair = "ungrouped"
		}

		for _, layer := range tc.Layers {
			layer := layer
			name := tc.ID + "/" + layer
			outcome := goldOK
			t.Run(name, func(t *testing.T) {
				defer func() {
					if t.Failed() {
						outcome = goldFail
					}
					st := stats[pair]
					if st == nil {
						st = &goldPairStat{}
						stats[pair] = st
					}
					switch outcome {
					case goldSkip:
						st.skip++
					case goldFail:
						st.fail++
					default:
						st.ok++
					}
				}()
				switch layer {
				case "safety":
					runGoldSafety(t, tc)
				case "normalize":
					runGoldNormalize(t, tc)
				case "extract":
					if os.Getenv("KPROMPT_GOLD_LLM") == "" {
						outcome = goldSkip
						t.Skip("set KPROMPT_GOLD_LLM=1 to run live Extract against a configured provider")
					}
					runGoldExtractLive(t, tc)
				default:
					t.Fatalf("unknown layer %q", layer)
				}
			})
		}
	}

	logGoldPairSummary(t, stats)
}

type goldOutcome int

const (
	goldOK goldOutcome = iota
	goldSkip
	goldFail
)

type goldPairStat struct {
	ok, skip, fail int
}

func logGoldPairSummary(t *testing.T, stats map[string]*goldPairStat) {
	t.Helper()
	pairs := make([]string, 0, len(stats))
	var totalOK, totalSkip, totalFail int
	for p, st := range stats {
		pairs = append(pairs, p)
		totalOK += st.ok
		totalSkip += st.skip
		totalFail += st.fail
	}
	sort.Strings(pairs)
	t.Log("gold pair summary:")
	for _, p := range pairs {
		st := stats[p]
		t.Logf("  %s: ok=%d skip=%d fail=%d", p, st.ok, st.skip, st.fail)
	}
	t.Logf("  TOTAL: ok=%d skip=%d fail=%d", totalOK, totalSkip, totalFail)
}

func runGoldSafety(t *testing.T, tc goldCase) {
	t.Helper()
	if tc.Want.PromptDenied == nil {
		t.Fatal("safety layer requires want.prompt_denied")
	}
	got := safety.CheckPrompt(tc.Prompt)
	if got.Denied != *tc.Want.PromptDenied {
		t.Fatalf("prompt_denied: got %v want %v (msg=%q)", got.Denied, *tc.Want.PromptDenied, got.Message)
	}
}

func runGoldNormalize(t *testing.T, tc goldCase) {
	t.Helper()
	in := seedIntent(tc)
	in.Raw = tc.Prompt
	in = intent.ApplyScope(in, intent.ScopePrefs{})
	in = intent.NormalizeVerb(in, tc.Prompt)
	assertGoldWant(t, in, tc)
}

func runGoldExtractLive(t *testing.T, tc goldCase) {
	t.Helper()
	name := firstNonEmpty(os.Getenv("KPROMPT_GOLD_PROVIDER"), os.Getenv("KPROMPT_PROVIDER"), "ollama")
	key := firstNonEmpty(os.Getenv("KPROMPT_API_KEY"), os.Getenv("OPENAI_API_KEY"), os.Getenv("ANTHROPIC_API_KEY"), os.Getenv("GEMINI_API_KEY"))
	base := os.Getenv("KPROMPT_OPENAI_BASE_URL")
	model := os.Getenv("KPROMPT_MODEL")
	provider, err := llm.New(name, key, base, model)
	if err != nil {
		t.Fatalf("live provider: %v", err)
	}
	in, err := intent.Extract(context.Background(), provider, tc.Prompt)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	in = intent.ApplyScope(in, intent.ScopePrefs{})
	in = intent.NormalizeVerb(in, tc.Prompt)
	assertGoldWant(t, in, tc)
}

func seedIntent(tc goldCase) intent.Intent {
	in := intent.Intent{Kind: intent.KindUnknown, Params: map[string]any{}}
	if tc.Seed == nil {
		return in
	}
	if tc.Seed.Kind != "" {
		in.Kind = intent.NormalizeKind(intent.Kind(tc.Seed.Kind))
	}
	if tc.Seed.Target != nil {
		in.Target = *tc.Seed.Target
	}
	if tc.Seed.Params != nil {
		in.Params = tc.Seed.Params
	}
	return in
}

func assertGoldWant(t *testing.T, got intent.Intent, tc goldCase) {
	t.Helper()
	if tc.Want.Kind != "" && string(got.Kind) != tc.Want.Kind {
		t.Fatalf("kind: got %q want %q", got.Kind, tc.Want.Kind)
	}
	if tc.Want.Target != nil {
		if n := strings.TrimSpace(tc.Want.Target.Name); n != "" && !strings.EqualFold(got.Target.Name, n) {
			t.Fatalf("target.name: got %q want %q", got.Target.Name, n)
		}
		if ns := strings.TrimSpace(tc.Want.Target.Namespace); ns != "" && !strings.EqualFold(got.Target.Namespace, ns) {
			t.Fatalf("target.namespace: got %q want %q", got.Target.Namespace, ns)
		}
		if k := strings.TrimSpace(tc.Want.Target.Kind); k != "" && !strings.EqualFold(got.Target.Kind, k) {
			t.Fatalf("target.kind: got %q want %q", got.Target.Kind, k)
		}
	}
	for key, wantVal := range tc.Want.Params {
		gotVal, ok := got.Params[key]
		if !ok {
			t.Fatalf("params.%s: missing (want %v)", key, wantVal)
		}
		if !paramEqual(gotVal, wantVal) {
			t.Fatalf("params.%s: got %#v want %#v", key, gotVal, wantVal)
		}
	}
	if tc.WantNot != nil {
		for _, bad := range tc.WantNot.Kinds {
			if string(got.Kind) == bad {
				t.Fatalf("kind %q forbidden by want_not", got.Kind)
			}
		}
	}
}

func paramEqual(got, want any) bool {
	switch w := want.(type) {
	case float64:
		switch g := got.(type) {
		case float64:
			return g == w
		case int:
			return float64(g) == w
		case int32:
			return float64(g) == w
		case int64:
			return float64(g) == w
		case json.Number:
			f, err := g.Float64()
			return err == nil && f == w
		}
	case string:
		s, ok := got.(string)
		return ok && s == w
	case bool:
		b, ok := got.(bool)
		return ok && b == w
	}
	gb, _ := json.Marshal(got)
	wb, _ := json.Marshal(want)
	return bytes.Equal(gb, wb)
}

func loadGoldExtract(t *testing.T) []goldCase {
	t.Helper()
	path := filepath.Join("testdata", "gold_extract.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	var out []goldCase
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
			continue
		}
		var tc goldCase
		if err := json.Unmarshal([]byte(line), &tc); err != nil {
			t.Fatalf("%s:%d: %v", path, lineNo, err)
		}
		out = append(out, tc)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	if len(out) == 0 {
		t.Fatalf("%s: no cases", path)
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

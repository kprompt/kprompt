package contexts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/config"
)

// Entry is one kubeconfig context plus optional alias / reachability.
type Entry struct {
	Name       string   `json:"name"`
	Cluster    string   `json:"cluster,omitempty"`
	AuthInfo   string   `json:"user,omitempty"`
	Namespace  string   `json:"namespace,omitempty"`
	Current    bool     `json:"current"`
	Aliases    []string `json:"aliases,omitempty"`
	Reachable  *bool    `json:"reachable,omitempty"`
	ReachError string   `json:"reach_error,omitempty"`
}

// Report is the inventory result.
type Report struct {
	Current   string    `json:"current,omitempty"`
	Items     []Entry   `json:"items"`
	CheckedAt time.Time `json:"checked_at"`
}

// Options configures List.
type Options struct {
	// CheckReachability probes each context's API server (optional; slower).
	CheckReachability bool
	// Timeout per reachability probe.
	Timeout time.Duration
	// Connect overrides cluster.Connect (tests).
	Connect func(contextName string) (*cluster.Clients, error)
	// Now overrides time for tests.
	Now func() time.Time
}

// List returns kubeconfig contexts merged with local aliases.
func List(ctx context.Context, opts Options) (Report, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 3 * time.Second
	}
	if opts.Connect == nil {
		opts.Connect = cluster.Connect
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}

	loading := clientcmd.NewDefaultClientConfigLoadingRules()
	clientCfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loading, &clientcmd.ConfigOverrides{})
	raw, err := clientCfg.RawConfig()
	if err != nil {
		return Report{}, cluster.Friendlier(fmt.Errorf("load kubeconfig: %w", err))
	}

	file, err := config.LoadFile()
	if err != nil {
		return Report{}, err
	}
	aliasByContext := map[string][]string{}
	for alias, target := range file.Aliases {
		t := strings.TrimSpace(target)
		if t == "" {
			continue
		}
		aliasByContext[t] = append(aliasByContext[t], alias)
	}
	for _, aliases := range aliasByContext {
		sort.Strings(aliases)
	}

	names := make([]string, 0, len(raw.Contexts))
	for name := range raw.Contexts {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]Entry, 0, len(names))
	for _, name := range names {
		ctxDef := raw.Contexts[name]
		e := Entry{
			Name:      name,
			Current:   name == raw.CurrentContext,
			Aliases:   append([]string(nil), aliasByContext[name]...),
			Cluster:   ctxDef.Cluster,
			AuthInfo:  ctxDef.AuthInfo,
			Namespace: ctxDef.Namespace,
		}
		if opts.CheckReachability {
			ok, msg := probe(ctx, opts, name)
			e.Reachable = &ok
			if !ok {
				e.ReachError = msg
			}
		}
		items = append(items, e)
	}

	return Report{
		Current:   raw.CurrentContext,
		Items:     items,
		CheckedAt: now().UTC(),
	}, nil
}

func probe(parent context.Context, opts Options, name string) (bool, string) {
	ctx, cancel := context.WithTimeout(parent, opts.Timeout)
	defer cancel()
	done := make(chan struct{})
	var (
		ok  bool
		msg string
	)
	go func() {
		defer close(done)
		clients, err := opts.Connect(name)
		if err != nil {
			msg = err.Error()
			return
		}
		_, err = clients.Clientset.Discovery().ServerVersion()
		if err != nil {
			msg = err.Error()
			return
		}
		ok = true
	}()
	select {
	case <-ctx.Done():
		return false, "timeout reaching API"
	case <-done:
		return ok, msg
	}
}

// FormatText prints a table.
func FormatText(w io.Writer, rep Report) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "CURRENT\tNAME\tALIASES\tCLUSTER\tNAMESPACE\tREACHABLE")
	for _, e := range rep.Items {
		cur := ""
		if e.Current {
			cur = "*"
		}
		aliases := "—"
		if len(e.Aliases) > 0 {
			aliases = strings.Join(e.Aliases, ",")
		}
		ns := e.Namespace
		if ns == "" {
			ns = "—"
		}
		reach := "—"
		if e.Reachable != nil {
			if *e.Reachable {
				reach = "yes"
			} else {
				reach = "no"
				if e.ReachError != "" {
					reach = "no (" + truncate(e.ReachError, 48) + ")"
				}
			}
		}
		clusterName := e.Cluster
		if clusterName == "" {
			clusterName = "—"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", cur, e.Name, aliases, clusterName, ns, reach)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if rep.Current != "" {
		fmt.Fprintf(w, "\nCurrent context: %s\n", rep.Current)
	}
	fmt.Fprintln(w, "Tip: kprompt config alias set <name> <kube-context>")
	return nil
}

// FormatJSON writes the report as JSON.
func FormatJSON(w io.Writer, rep Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

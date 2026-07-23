package contexts

import (
	"context"
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/kprompt/kprompt/internal/config"
)

func TestListMergesAliases(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KUBECONFIG", writeKubeconfig(t, dir))

	if _, err := config.SetAlias("prod", "prod-gke"); err != nil {
		t.Fatal(err)
	}
	if _, err := config.SetAlias("edge", "prod-gke"); err != nil {
		t.Fatal(err)
	}

	rep, err := List(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Current != "kind-dev" {
		t.Fatalf("current=%q", rep.Current)
	}
	var prod *Entry
	for i := range rep.Items {
		if rep.Items[i].Name == "prod-gke" {
			prod = &rep.Items[i]
			break
		}
	}
	if prod == nil {
		t.Fatal("missing prod-gke")
	}
	if len(prod.Aliases) != 2 || prod.Aliases[0] != "edge" || prod.Aliases[1] != "prod" {
		t.Fatalf("aliases=%v", prod.Aliases)
	}
	var kind *Entry
	for i := range rep.Items {
		if rep.Items[i].Name == "kind-dev" {
			kind = &rep.Items[i]
			break
		}
	}
	if kind == nil || !kind.Current {
		t.Fatalf("kind=%v", kind)
	}
}

func TestFormatText(t *testing.T) {
	yes := true
	rep := Report{
		Current: "a",
		Items: []Entry{
			{Name: "a", Current: true, Aliases: []string{"prod"}, Cluster: "c1", Namespace: "default", Reachable: &yes},
		},
	}
	var b strings.Builder
	if err := FormatText(&b, rep); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "prod") || !strings.Contains(out, "*") {
		t.Fatalf("%s", out)
	}
}

func writeKubeconfig(t *testing.T, dir string) string {
	t.Helper()
	cfg := clientcmdapi.NewConfig()
	cfg.CurrentContext = "kind-dev"
	cfg.Clusters["c-dev"] = &clientcmdapi.Cluster{Server: "https://127.0.0.1:1"}
	cfg.Clusters["c-prod"] = &clientcmdapi.Cluster{Server: "https://127.0.0.1:2"}
	cfg.AuthInfos["u"] = &clientcmdapi.AuthInfo{Token: "x"}
	cfg.Contexts["kind-dev"] = &clientcmdapi.Context{Cluster: "c-dev", AuthInfo: "u", Namespace: "default"}
	cfg.Contexts["prod-gke"] = &clientcmdapi.Context{Cluster: "c-prod", AuthInfo: "u"}
	path := dir + "/kubeconfig"
	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		t.Fatal(err)
	}
	return path
}

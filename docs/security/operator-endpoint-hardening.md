# Operator endpoint hardening (SEC-007 Decision A)

Context: security pack **SEC-001…SEC-007**. This guide is the operational follow-up for **SEC-007 Decision A**.

## Why Decision A exists

kprompt trusts **operator-controlled** endpoints configured for the cluster (or laptop kubeconfig):

- LLM provider `base_url` / presets
- Prometheus / Grafana / OpenTelemetry collectors
- Webhook / Slack / Discord notify targets

These are set by operators, not untrusted end users. Automatic private-range / link-local URL blocking would break legitimate self-hosted stacks (in-cluster Ollama, private Prom, etc.).

**Decision A therefore does not add automatic SSRF URL blocking.** Reducing blast radius is an **operator** job: egress allowlists, NetworkPolicy / CNI policy, and honest doctor advisories.

Related product docs: [reality-anchors.md](../reality-anchors.md) · agent chart [Network Policy](../../charts/kprompt-agent/README.md#network-policy).

---

## Chart baseline (opt-in)

`charts/kprompt-agent`, `charts/kprompt-coordinator`, and `charts/kprompt-operator` each ship an **opt-in** egress NetworkPolicy (`networkPolicy.enabled`, default `false`):

- Policy type: **Egress** default-deny once enabled
- Explicit allows (when configured): DNS, kube-apiserver CIDRs; the agent chart also supports LLM / observability / webhook CIDRs + `extraEgress`
- Helm **`values.schema.json`** validates the `networkPolicy` block
- **Strongly recommended for production**, but only after CIDRs match your topology

> Do **not** set `networkPolicy.enabled=true` with empty `kubeAPIServerCIDRs` (and for the agent, empty `llmCIDRs` when you need an LLM). The pod will lose API and/or LLM egress.

---

## Configure with Helm

```bash
helm upgrade --install kprompt-agent ./charts/kprompt-agent \
  --namespace payments --create-namespace \
  --set networkPolicy.enabled=true \
  --set-json 'networkPolicy.kubeAPIServerCIDRs=["10.96.0.1/32"]' \
  --set-json 'networkPolicy.llmCIDRs=["203.0.113.10/32"]' \
  --set networkPolicy.llmPort=443
```

Self-hosted LLM on a non-443 port:

```yaml
networkPolicy:
  enabled: true
  kubeAPIServerCIDRs: ["10.96.0.1/32"]
  llmCIDRs: ["10.0.2.50/32"]
  llmPort: 11434          # Ollama
  # or: llmExtraPorts: [8080]
```

DNS on many clusters uses `kube-system` + `k8s-app: kube-dns`. For NodeLocal DNSCache / custom labels, add `networkPolicy.extraEgress` (see chart README).

---

## Example topologies

### Cloud-hosted LLM (SaaS API)

Allow DNS + API server CIDR + provider egress (often `0.0.0.0/0` on 443 is too wide — prefer the provider’s published ranges or a known egress proxy CIDR).

### Self-hosted stack (in-cluster LLM + Prom)

Allow DNS + API server + Service/pod CIDRs (or `ipBlock`) for the LLM Service and Prometheus / OTel collector. Prefer namespace/`podSelector` rules via `extraEgress` when the CNI supports them.

Minimal illustrative egress policy (adapt labels/CIDRs):

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kprompt-agent-egress
  namespace: payments
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: kprompt-agent
  policyTypes: [Egress]
  egress:
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: kube-system
          podSelector:
            matchLabels:
              k8s-app: kube-dns
      ports:
        - { protocol: UDP, port: 53 }
        - { protocol: TCP, port: 53 }
    - to:
        - ipBlock: { cidr: 10.96.0.1/32 }   # apiserver Service — replace
      ports:
        - { protocol: TCP, port: 443 }
        - { protocol: TCP, port: 6443 }
    - to:
        - ipBlock: { cidr: 10.0.2.50/32 }   # LLM — replace
      ports:
        - { protocol: TCP, port: 11434 }
```

Use Calico / Cilium equivalents if you do not rely on vanilla NetworkPolicy.

---

## Footguns

### `hostNetwork: true`

Pods with `hostNetwork: true` **bypass NetworkPolicy** on most CNIs. Do not run the Observe agent with host networking. Prefer the chart defaults (`hostNetwork` unset/false).

### Empty allowlists

Enabling default-deny without kube-API and LLM CIDRs bricks the agent. Fill CIDRs first; use `helm template` to review the rendered policy.

### DNS rebinding / split-horizon

NetworkPolicy on IP blocks does not stop a DNS name from resolving to an unexpected address after policy evaluation on some stacks. Prefer:

- Stable Service DNS inside the cluster for self-hosted LLM/Prom
- Avoid `dnsPolicy: ClusterFirstWithHostNet` unless you truly need host net (you usually do not)
- Optional `dnsConfig` `nameservers` / `searches` that match your hardened resolver
- Treat DNS-based SSRF (rebinding) as part of the same allowlist story — pin destinations where you can

---

## Doctor advisory

`kprompt doctor` emits **WARN** (non-fatal) when:

- it finds agent / coordinator / operator pods and
  - no selecting **egress** NetworkPolicy, and/or
  - any of those pods with **`hostNetwork: true`**
- configured operator endpoints (`base_url`, `tools.prometheus.url`, `tools.grafana.url`, `tools.otel.endpoint`) resolve to **RFC-1918 / ULA / link-local / loopback** (or cannot be resolved)

```bash
kprompt doctor
kprompt doctor --context staging
```

Advisory only — Decision A still does **not** hard-block private URLs. See [doctor.md](../doctor.md).

---

## Post-apply checklist

- [ ] `networkPolicy.enabled=true` only after CIDRs match topology
- [ ] Agent can still reach apiserver + LLM (`kprompt agent status` / logs)
- [ ] No `hostNetwork: true` on agent pods
- [ ] `kprompt doctor` NetworkPolicy check is PASS or an accepted WARN with a tracked follow-up
- [ ] Notify webhooks / Prom URLs are operator-owned (not end-user input)

---

## See also

- [reality-anchors.md](../reality-anchors.md) — SEC-007 Decision A
- [charts/kprompt-agent/README.md](../../charts/kprompt-agent/README.md) — values and DNS notes
- [doctor.md](../doctor.md) — local checks
- [OWASP SSRF Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html)
- [Kubernetes NetworkPolicy](https://kubernetes.io/docs/concepts/services-networking/network-policies/)

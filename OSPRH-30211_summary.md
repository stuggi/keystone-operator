# OSPRH-30211 — KeystoneAPI Hub/Spoke conversion webhook

Reference implementation of the CRD conversion-webhook Hub/Spoke pattern:
KeystoneAPI is bumped to `v1beta2` as a no-op (schema identical to `v1beta1`),
`v1beta2` becomes the Hub/storage version, `v1beta1` becomes the Spoke, and a
conversion webhook is wired through the generated CRD and bundle.

## API version, storage & Hub decision

- **New version:** `keystone.openstack.org/v1beta2` (KeystoneAPI **only** — the
  other Kinds KeystoneService, KeystoneEndpoint, KeystoneApplicationCredential
  stay v1beta1-only).
- **Hub / storage version:** `v1beta2` — carries `//+kubebuilder:storageversion`
  and implements `conversion.Hub` (`Hub()`). It is a byte-identical copy of the
  v1beta1 schema (no-op bump).
- **Spoke:** `v1beta1` — implements `conversion.Convertible`
  (`ConvertTo`/`ConvertFrom`) as **explicit field-by-field copies** (not unsafe
  casts), so it is the reference template other operators copy.
- No new controller for v1beta2. The controller, defaulting and validating
  webhooks are set up **once**, for the latest version (v1beta2) only.

## Conversion webhook service & namespace templating

- **Service name:** `keystone-operator-webhook-service`, path `/convert`,
  `conversionReviewVersions: [v1]`.
- **Namespace:** `keystone-operator-system`.
- **Templating:** the base patch `config/crd/patches/webhook_in_keystoneapis.yaml`
  uses `name: webhook-service` / `namespace: system`; `config/default`
  (`namePrefix: keystone-operator-`, `namespace: keystone-operator-system`) +
  `config/crd/kustomizeconfig.yaml` (nameReference + namespace transformers)
  rewrite them to the resolved values above.
- **CA injection:** `config/crd/patches/cainjection_in_keystoneapis.yaml` stamps
  `cert-manager.io/inject-ca-from: keystone-operator-system/keystone-operator-serving-cert`
  statically (keeps the OLM bundle free of cert-manager objects; openstack-operator
  rewrites the namespace downstream in OSPRH-30208).

## Files changed

**Modified (tracked):**
- `PROJECT` — registers KeystoneAPI v1beta2 with `conversion: true`, `spoke: [v1beta1]`.
- `cmd/main.go` — defaulting/validating webhook setup migrated to v1beta2.
- `internal/controller/keystoneapi_controller.go` — controller uses v1beta2.
- `internal/keystone/{bootstrap,cronjob,dbsync,deployment,volumes}.go` — helper imports → v1beta2.
- `api/bases/keystone.openstack.org_keystoneapis.yaml`,
  `config/crd/bases/keystone.openstack.org_keystoneapis.yaml` — generated CRD carries both versions.
- `config/crd/kustomization.yaml` — enables the webhook + cainjection CRD patches.
- `config/default/kustomization.yaml` — conversion service name/namespace wiring.
- `config/webhook/manifests.yaml` — v1beta2-only webhook configs.
- `config/manifests/bases/keystone-operator.clusterserviceversion.yaml` — installModes
  restricted to **AllNamespaces only** (operator-sdk requires this once a bundle
  carries a conversion webhook / conversionCRDs; RHOSO deploys operators cluster-wide).
- `config/samples/kustomization.yaml` — adds the v1beta2 sample.
- `api/go.mod` — `github.com/google/gofuzz` promoted to a direct dependency (round-trip test).
- `test/functional/suite_test.go` — registers both KeystoneAPI versions in the scheme
  **before** `testEnv.Start()` (so envtest installs the conversion webhook) and sets up
  the v1beta2 webhook/defaults.

**Deleted:**
- `internal/webhook/v1beta1/keystoneapi_webhook.go` — replaced by the v1beta2 webhook.

**New:**
- `api/v1beta2/` — `groupversion_info.go`, `keystoneapi.go`, `keystoneapi_types.go`,
  `keystoneapi_webhook.go`, `keystoneapi_conversion.go` (`Hub()`),
  `zz_generated.deepcopy.go`.
- `api/v1beta1/keystoneapi_conversion.go` — Spoke `ConvertTo`/`ConvertFrom` (explicit copies).
- `api/v1beta1/keystoneapi_conversion_test.go` — gofuzz round-trip identity unit test.
- `internal/webhook/v1beta2/keystoneapi_webhook.go` — defaulting/validating webhook for v1beta2.
- `config/crd/patches/webhook_in_keystoneapis.yaml`,
  `config/crd/patches/cainjection_in_keystoneapis.yaml` — conversion + CA-injection CRD patches.
- `config/samples/keystone_v1beta2_keystoneapi.yaml` — v1beta2 sample.
- `test/functional/keystoneapi_conversion_test.go` — envtest end-to-end conversion round-trip.

## Test commands

```bash
# from the keystone-operator repo root

# unit: conversion round-trip is the identity (fuzzes every field)
KUBEBUILDER_ASSETS="$(pwd)/$(bin/setup-envtest use 1.33 --bin-dir bin -p path)" \
  go test ./api/v1beta1/...

# functional: conversion webhook end-to-end through envtest (v1beta2 hub read
# direct, v1beta1 spoke read forced through /convert) + full existing suite
KUBEBUILDER_ASSETS="$(pwd)/$(bin/setup-envtest use 1.33 --bin-dir bin -p path)" \
  go test ./test/functional/...

# generated manifests + bundle are clean and carry spec.conversion.webhook
make manifests generate
make bundle
```

## Follow-ups (out of scope for 30211)

- **OSPRH-30210** — openstack-operator must run keystone's webhook **server** and
  stage the serving Service + Certificate the conversion clientConfig points at.
  Today `openstack_controller.go` sets `ENABLE_WEBHOOKS=false` for keystone, and
  that flag does double duty (webhook server/cert **and** validating/defaulting
  registration). It should be split so the server + serving cert + `/convert` are
  activated by "CRD carries conversion" (mandatory everywhere) while the
  validating/defaulting configs stay per-operator (on for infra/baremetal only).

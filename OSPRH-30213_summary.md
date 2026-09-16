# OSPRH-30213 summary

## Files changed

```
test/functional/keystoneapi_ownerref_test.go  (new — 3 envtest specs)
```

## Outcome

Tests prove the existing `ReconcileRbac → serviceaccount.CreateOrPatch →
SetControllerReference` path (lib-common `serviceaccount.go:62`) self-heals a stale
`v1beta1` SA owner ref to `v1beta2` on the next reconcile.  No explicit patch helper
was added — per the OSPRH-30212 spike conclusion, the self-heal path is sufficient.

The canonical implementation lives in lib-common at:
`modules/common/serviceaccount/serviceaccount.go:62`
(`controllerutil.SetControllerReference(h.GetBeforeObject(), sa, h.GetScheme())`)

No interim keystone-specific helper was created; none is needed.

## Test command

```
make test GINKGO_ARGS="--ginkgo.focus=owner-ref convergence"
```

The three specs are in `Describe("KeystoneAPI SA owner-ref convergence across CRD version bumps")`:

1. self-heals a stale v1beta1 SA owner ref to v1beta2 on the next reconcile
2. is idempotent: a second reconcile leaves the v1beta2 owner ref unchanged
3. recreates a deleted SA with a v1beta2 owner ref

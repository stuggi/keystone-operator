# lb-pub-endpt-reuse — keystone-operator rollout (D0 + D1 + D4.3 + DNS annotation)

Rollout of the placement reference implementation
(`nova-operator/lb-pub-endpt-4-placement_summary.md`) to **keystone**, per
`openstack-operator/docs/dev/lb-pub-endpt-reuse/plan-rollout-remaining-operators.md` and
`prompts/5-rollout-remaining-operators.prompt.md`.

`<OVERRIDE_SHAPE>`: **map** — `instance.Spec.Override.Service[endpointType]`
(`map[service.Endpoint]service.RoutedOverrideSpec`), same as placement. Used
`webhook.ValidatePortNameOverrides` + `webhook.DefaultServicePortNames()`, not the
single-field path.

No `go.mod` bump was needed: keystone-operator's `go.mod` already pins the same
`replace` for `lib-common/modules/common` (`v0.0.0-20261008123848-950ea3bae570`) that
placement uses, and that version already has `service.GetVHostServerName`,
`webhook.ValidatePortNameOverrides`/`ValidateServicePortNames`/`DefaultServicePortNames`,
and `service.AnnotationCNAMEsKey`.

## Files changed

- `internal/controller/keystoneapi_controller.go` — Service creation loop (port-name
  split + DNS annotation swap, D0 + DNS step 5); vhost-config build loop (D1).
- `templates/keystoneapi/config/httpd.conf` — httpd vhost template, `ServerAlias` lines.
- `api/v1beta1/keystoneapi_webhook.go` — `KeystoneAPISpecCore` validator (D4.3).
- `test/functional/keystoneapi_controller_test.go` — live-Service object-name/port-name
  assertion, rendered-vhost assertion, DNS annotation swap + regression assertions.
- `test/functional/keystoneapi_webhook_test.go` — validator tests (4 create + 1 update).

## Object name vs. port name (D0)

The Service **object's** name is unchanged: `keystone-public` / `keystone-internal`. The
Service's single **port's** name is now the bare endpoint string, `public` / `internal`.
Keystone's pre-D0 code reused one `endpointName := instance.Name + "-" + endpointTypeStr`
for both the object `Name` and `Port.Name` — split so only `Port.Name` becomes
`endpointTypeStr`; `Name: endpointName` (object name) is untouched.

## Edit points

### D1 — controller (`internal/controller/keystoneapi_controller.go`, `generateServiceConfigMaps`, vhost loop)

```go
defaultName := fmt.Sprintf("%s-%s.%s.svc", instance.Name, endpt.String(), instance.Namespace)
serverName, serverAliases, err := service.GetVHostServerName(instance.Spec.Override.Service[endpt], defaultName)
if err != nil {
    return nil, err
}
endptConfig["ServerName"] = serverName
endptConfig["ServerAliases"] = serverAliases
```

Identical to placement — `GetVHostServerName` returns `(defaultName, nil, nil)` when no
`EndpointURL` override is set (internal, or public without one); no behavior change there.

### D1 — template (`templates/keystoneapi/config/httpd.conf`)

```
  ServerName {{ $vhost.ServerName }}
{{- range $alias := $vhost.ServerAliases }}
  ServerAlias {{ $alias }}
{{- end }}
```

`<VirtualHost *:5000>` unchanged (native port, same for public and internal).

### D0 — controller: decouple object name from port name (service-creation loop)

```go
endpointName := instance.Name + "-" + endpointTypeStr // Service object name — unchanged

svc, err := service.NewService(
    service.GenericService(&service.GenericServiceDetails{
        Name:      endpointName,      // "keystone-public" / "keystone-internal" — unchanged
        ...
        Port: service.GenericServicePort{
            Name:     endpointTypeStr, // "public" / "internal" — D0
            Port:     data.Port,
            Protocol: corev1.ProtocolTCP,
        },
    }),
    ...
)
```

### DNS annotation swap (step 5, same loop as D0, public-and-internal-shared code block)

Replaced the unconditional `AnnotationHostnameKey: svc.GetServiceHostname()` on
`LoadBalancer` Services with:

```go
dnsHostname, dnsAliases, err := service.GetVHostServerName(svcOverride, svc.GetServiceHostname())
...
dnsAnnotations := map[string]string{
    service.AnnotationHostnameKey: dnsHostname,
}
if len(dnsAliases) > 0 {
    dnsAnnotations[service.AnnotationCNAMEsKey] = strings.Join(dnsAliases, ",")
}
svc.AddAnnotation(dnsAnnotations)
```

Reuses the same `GetVHostServerName` helper as D1 (same override, same default-name
semantics) — with no `endpointURL` set it returns the unchanged default with no error;
when `endpointURL` is set, the custom host becomes canonical and the in-cluster `.svc`
name becomes the CNAME alias.

### D4.3 — webhook (`api/v1beta1/keystoneapi_webhook.go`)

```go
func validateServicePortNames(basePath *field.Path, overrides map[service.Endpoint]service.RoutedOverrideSpec) field.ErrorList {
    return common_webhook.ValidatePortNameOverrides(basePath, overrides, common_webhook.DefaultServicePortNames())
}
```

Wired into both `KeystoneAPISpecCore.ValidateCreate` and `.ValidateUpdate` alongside the
existing `service.ValidateRoutedOverrides` call. Reused the file's existing
`common_webhook` import alias (keystone's webhook file already imports
`lib-common/modules/common/webhook` under that alias for deprecated-field validation) —
no new import added. Automatically reachable at the control-plane level: openstack-operator's
`OpenStackControlPlane.ValidateCreateServices`/update path already calls
`r.Spec.Keystone.Template.ValidateCreate`/`ValidateUpdate` (`KeystoneAPISpecCore`), so no
openstack-operator change was needed.

Confirmed field names from the vendored lib-common fork (same as placement):
`OverrideServiceSpec.Ports []OverrideServicePort`, each with `Name string` / `Port int32`.

## Tests added

- `test/functional/keystoneapi_controller_test.go`, `When("Deployment is completed", ...)`
  (default spec, no override): new `It` asserts the live public/internal Services' **object**
  names are unchanged (`keystone-public`/`keystone-internal`) while their **port** names are
  the new bare values (`public`/`internal`) — the operational D0 regression test.
- Same file, `When("A KeystoneAPI is created with service override endpointURL set", ...)`:
  new `It` renders `httpd.conf` and asserts the public vhost has
  `ServerName keystone-openstack.apps-crc.testing` + `ServerAlias keystone-public.<ns>.svc`,
  the internal vhost keeps its default `ServerName`, and `ServerAlias` appears exactly once.
- Same file, `When("A KeystoneAPI is created with service override", ...)` (internal
  `LoadBalancer`, no `endpointURL`): added a regression assertion that
  `dnsmasq.network.openstack.org/cnames` is absent (unchanged default case).
- Same file, new `When("A KeystoneAPI public endpoint is a LoadBalancer with endpointURL
  set", ...)`: asserts `dnsmasq.network.openstack.org/hostname` is the custom host and
  `.../cnames` is the in-cluster `.svc` name.
- `test/functional/keystoneapi_webhook_test.go`: four new `It`s (accept/reject × public/
  internal, on create) plus one new `It` (reject on update) for the port-name validator,
  same structure as placement's.

All 100 specs in `test/functional/` pass (`make test`); `make generate manifests fmt vet`
clean, no CRD/manifest diff (no API type changes).

## Known issue inherited from the reference (not fixed here)

Code review flagged that the DNS hostname/CNAME swap (step 5) is not symmetric on revert:
if `endpointURL` is cleared after having been set, `service.GetVHostServerName` correctly
reverts `AnnotationHostnameKey` to the in-cluster name, but `AnnotationCNAMEsKey` is only
*added* when aliases are non-empty — `svc.AddAnnotation` → lib-common's
`MergeStringMaps(base, extra...)` never deletes a key, so a stale CNAME value lingers on
the live Service forever once set. This is a Service-wrapper/annotation-merge limitation in
lib-common itself (no `RemoveAnnotation` exists), reproduced verbatim from the already-merged
nova-operator/placement implementation — not introduced or made worse by this keystone
change, and not fixable by a one-off workaround here without diverging from the shared
reference pattern. Needs either a lib-common fix (explicit annotation deletion in
`Service.CreateOrPatch`/`AddAnnotation`, or a dedicated removal API) or an explicit
`delete(annotations, AnnotationCNAMEsKey)` call added identically in every operator that
uses this pattern (placement, keystone, and future rollout targets) — tracked as a shared
follow-up, not scoped to this PR.

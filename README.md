# tf-msadmin

[![CI](https://github.com/terraprovider/tf-msadmin/actions/workflows/ci.yml/badge.svg)](https://github.com/terraprovider/tf-msadmin/actions/workflows/ci.yml)

Shared Terraform-Plugin-Framework runtime for the Microsoft admin providers —
[Exchange Online](https://github.com/terraprovider/terraform-provider-exo), Purview and
Teams. Factors out the parts every one of these providers needs so each provider
repo only carries its own resources.

```bash
go get github.com/terraprovider/tf-msadmin
```

## Packages

### `authschema` — one auth surface for every provider

A provider embeds `authschema.Model` in its config struct, merges
`authschema.Attributes()` into its schema, and calls `Model.Config()` to get a
resolved [`authx.Config`](https://github.com/terraprovider/go-exoscc). The surface is
aligned field-for-field with the `hashicorp/azuread` and `azurerm` providers
(same attribute names, `ARM_*`/`AZURE_*` env fallbacks, every OIDC/workload-identity
flavour), so operators configure them all identically.

### `reconcile` — the "configured value wins" rule

The admin APIs are eventually consistent and sometimes normalise written values,
so a read-back right after a create/update can disagree with what was configured
— which Terraform rejects as an inconsistent result. `KeepStr`/`KeepSet`/… keep a
configured (known) value authoritative while letting computed attributes take the
freshly-read value. `ReflectsFields` builds the predicate used to wait out
write-propagation lag.

### `resourcex` — stale-tolerant read-after-write

`LoadUntil` wraps [`consistency`](https://github.com/terraprovider/go-msadmin): it
retries a read until the resource is visible (rides out post-create lag) and,
after an update, until it reflects the written values — returning the last-seen
value rather than dropping the resource when it is present but not yet reflected.

## License

MIT — see [LICENSE](LICENSE). Not affiliated with or endorsed by Microsoft.

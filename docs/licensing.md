# Kit Pro licensing

Kit verifies its PASETO v4.public license offline. Production keys carry the
signing key ID `hk-2026a` and the signed claim `product = "kit"`. Kit Pro
and Layer Pro have independent signing keys, renewals, and release schedules.
Set `HEV_LICENSE` for Kit and `LAYER_LICENSE` for Layer. Each license unlocks
its own product.

Set `HEV_LICENSE` to the issued key or `HEV_LICENSE_REF` to a secret
reference. For persistent host configuration, use `~/.hev/config.toml`:

```toml
[license]
key_ref = "secret://kit-license"
```

References can use `secret://`, `op://`, or `azkv://`. Use `key` for a
literal key and protect the file accordingly. Precedence is `HEV_LICENSE`,
`HEV_LICENSE_REF`, `[license].key`, then `[license].key_ref`.

A verified key is **licensed** through its expiration instant. Non-trial
licenses then enter **grace** for up to 14 days, with Pro access and renewal
warnings. Trials go directly to **floor**. Floor also applies to missing or
invalid licenses. At floor, Kit continues its OSS behavior and refuses new
operations that require Pro. Existing data and running workloads are untouched.

Run `hev license` to refresh and inspect Kit's license, `hev pro` for the
Kit edition and separate Layer gateway information, or `hev s` for a short
host status. The first two support `--json`. The daemon refreshes local
status in the background so receivers can make an entitlement decision
without reading from a vault.

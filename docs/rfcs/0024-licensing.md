# RFC 0024 — Offline Kit Pro licenses

Kit and Layer issue and renew licenses independently. Kit trusts the
`hk-2026a` Ed25519 ceremony key and requires a signed `product = "kit"`
claim. The PASETO v4.public signature authenticates the claims and footer.
The deterministic development key is available only through the development
verifier. Production callers use the ceremony key exclusively.

Kit reads its own environment and TOML config. Secret references use Kit's
provider routing and cooldown controls with a six-hour daemon cache TTL.
The host projection is a private atomic file containing derived status.
It expires locally and its timeline is evaluated at read time. It serves
same-user local feature checks, with the host account as the trust boundary.

Renewal warnings and grace preserve continuity before the OSS floor takes
effect for new Pro operations. License processing never changes existing
workspaces, stored data, or running workloads.

See [Kit Pro licensing](../licensing.md) for configuration and inspection.

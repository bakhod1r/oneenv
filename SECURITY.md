# Security Policy

## Supported versions

| Version | Supported |
| ------- | --------- |
| 1.2.x   | ✅        |
| < 1.2   | ❌        |

Fixes land on the latest minor release. Upgrade before reporting an issue
against an older tag.

## Reporting a vulnerability

**Do not open a public issue.**

Report privately through GitHub's
[private vulnerability reporting](https://github.com/bakhod1r/oneenv/security/advisories/new),
or by email to **bakhodiryashinmansur@gmail.com** with `[oneenv security]` in
the subject.

Please include:

- the affected version,
- a minimal reproduction (struct + `.env` content + options used),
- the impact you believe it has.

You can expect an acknowledgement within 72 hours and an assessment within
7 days. If the report is confirmed, a patch release and a GitHub Security
Advisory follow; you will be credited unless you ask otherwise.

## Scope

`oneenv` reads configuration files and decodes them into structs, so the
security-relevant surface is narrow but real. In scope:

- **Secret leakage** — a `Secret[T]` field, a `,secret` field, or a redacted
  value appearing in an error message, `Marshal` output, `Usage` output, panic
  text, or any `String`/`Format` implementation.
- **Secret corruption** — variable expansion, mutators, or trimming altering
  the literal bytes of a value destined for a secret or `,noexpand` field.
- **Path handling** — `env:"X,file"` reading outside the path it was given, or
  file-mode loading following a link in a way the caller did not ask for.
- **Parser denial of service** — an input that makes the parser allocate or
  loop without bound.
- **`oneenv/watch`** — file descriptor or goroutine leaks on cancellation, or a
  reload path that races the caller's struct.

Out of scope:

- Secrets committed to source control or world-readable `.env` files on disk —
  that is a deployment concern, not a library one.
- Values a caller deliberately prints themselves after decoding.
- Anything requiring an attacker who already controls the process.

## Handling secrets safely

- Mark sensitive fields: `Secret[string]`, or `env:"PASSWORD,secret"`.
- Use `env:"PASSWORD,file"` with Docker/Kubernetes `/run/secrets` rather than
  putting the value in the environment at all.
- Secret and `,noexpand` fields are never expanded, so `pa$$word` decodes
  literally. If you need expansion on a field, don't mark it secret.

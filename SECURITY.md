# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest  | Yes       |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly.

**Do not open a public issue.**

Instead, email security concerns to the maintainers or use [GitHub's private vulnerability reporting](https://github.com/fbsobreira/gotron-mcp/security/advisories/new).

Please include:

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

We will acknowledge receipt within 48 hours and aim to release a fix within 7 days for critical issues.

## Security Design

The GoTRON MCP server is designed with security as a priority:

- **No private key storage** — the server never stores or manages private keys directly
- **Unsigned transactions** — all write tools return unsigned transaction hex; signing is the user's responsibility
- **Keystore opt-in** — signing via local keystore requires the explicit `--keystore` flag
- **Hosted mode isolation** — SSE mode automatically disables all write and sign tools at registration time
- **No secrets by default** — API key is optional and only needed for TronGrid rate limits

## Scope

The following are in scope for security reports:

- Private key exposure or leakage
- Unauthorized transaction signing or broadcasting
- Command injection via tool inputs
- Bypass of hosted mode restrictions (write tools accessible in SSE mode)
- Denial of service via crafted MCP requests

The following are out of scope:

- Vulnerabilities in upstream dependencies (report to the respective project)
- Issues requiring physical access to the machine
- Social engineering

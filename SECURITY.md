# Security Policy

## Reporting a Vulnerability

We take the security of AgentFlow seriously. If you discover a security vulnerability, please report it privately before disclosing it publicly.

**Do NOT report security vulnerabilities via public GitHub Issues.**

Instead, send a detailed report to: **[agentflow@basui01.dev](mailto:agentflow@basui01.dev)**

### What to include

- A clear description of the vulnerability
- Steps to reproduce the issue
- Affected versions
- Any potential impact or exploit scenarios
- Optional: suggested fix or mitigation

### Response Timeline

- **Acknowledgment**: within 48 hours
- **Initial assessment**: within 5 business days
- **Fix release**: depends on severity, typically within 14 days for critical issues

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| 1.x     | :white_check_mark: |
| < 1.0   | :x:                |

## Security Best Practices

When using AgentFlow in production:

1. **Keep dependencies updated**: Run `go mod tidy` and review security advisories regularly
2. **Use environment variables** for API keys and secrets — never hardcode them
3. **Validate all LLM outputs** before executing generated code or commands
4. **Set appropriate timeouts** for all LLM provider calls
5. **Enable audit logging** to track agent decisions and actions
6. **Review provider configurations** — use least-privilege API keys

## Disclosure Policy

When a vulnerability is reported and confirmed:

1. We will work on a fix in a private fork
2. A security advisory will be published on GitHub
3. A patch release will be made available
4. The vulnerability will be publicly disclosed after the fix is released

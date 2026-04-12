# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability in this project, please report it responsibly using [GitHub Security Advisories](https://github.com/JeffResc/authentik-k8s-operator/security/advisories/new).

**Please do not open a public issue for security vulnerabilities.**

### What to include

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

### Response timeline

- **Acknowledgment**: Within 48 hours of report
- **Initial assessment**: Within 1 week
- **Fix or mitigation**: Dependent on severity, targeting within 30 days for critical issues

## What Constitutes a Security Issue

**Security issues** (report privately):
- Authentication or authorization bypasses
- Credential exposure (API tokens, client secrets)
- Kubernetes RBAC escalation
- Container escape or privilege escalation
- Dependency vulnerabilities with a known exploit

**Regular bugs** (open a public issue):
- Reconciliation failures
- Status reporting errors
- Helm chart configuration issues
- Documentation errors

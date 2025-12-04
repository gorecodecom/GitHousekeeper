# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 2.x.x   | :white_check_mark: |
| 1.x.x   | :x:                |

## Reporting a Vulnerability

We take the security of GitHousekeeper seriously. If you believe you have found a security vulnerability, please report it to us as described below.

### How to Report

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, please report them via email to: **security@gorecode.com**

Or use GitHub's private vulnerability reporting feature:
1. Go to the [Security tab](https://github.com/gorecodecom/GitHousekeeper/security)
2. Click "Report a vulnerability"
3. Fill out the form with details

### What to Include

Please include as much of the following information as possible:

- Type of vulnerability (e.g., remote code execution, path traversal, etc.)
- Full paths of source file(s) related to the vulnerability
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact of the issue, including how an attacker might exploit it

### Response Timeline

- **Initial Response**: Within 48 hours
- **Status Update**: Within 7 days
- **Resolution Target**: Within 30 days (depending on complexity)

### What to Expect

1. **Acknowledgment**: We will acknowledge receipt of your report within 48 hours.
2. **Assessment**: We will investigate and assess the vulnerability.
3. **Updates**: We will keep you informed of our progress.
4. **Fix**: Once fixed, we will notify you and discuss disclosure timing.
5. **Credit**: With your permission, we will credit you in the release notes.

## Security Best Practices for Users

When using GitHousekeeper:

1. **Run on trusted networks**: The web interface runs on localhost:8080. Avoid exposing it to untrusted networks.
2. **Review changes before committing**: Always review the changes made by the tool before pushing to remote repositories.
3. **Keep updated**: Use the latest version to benefit from security fixes.
4. **Backup your repositories**: Before running batch operations, ensure you have backups or can restore from remote.

## Scope

The following are in scope for security reports:

- Remote code execution
- Path traversal vulnerabilities
- Arbitrary file read/write
- Command injection
- Cross-site scripting (XSS) in the web interface
- Authentication/authorization bypasses

The following are out of scope:

- Issues requiring physical access to the machine
- Social engineering attacks
- Denial of service attacks
- Issues in third-party dependencies (report these to the dependency maintainers)

Thank you for helping keep GitHousekeeper and our users safe!

# Security Policy

## Reporting a vulnerability

Report suspected vulnerabilities through GitHub's private vulnerability
reporting: open the **Security** tab of this repository and choose
**Report a vulnerability**. This keeps the report private until a fix is ready.

Please do not open a public issue for security reports.

## Scope

funq is a pure-library package with no dependencies outside the Go standard
library. It performs no I/O, no cryptography, and no `unsafe` operations, so
the realistic surface is limited to logic errors (e.g. an out-of-range index
in a sequence operation). Reports of such bugs are still welcome.

## Supported versions

Only the latest `0.x` minor version receives fixes while the API is
pre-1.0.

# Security Policy

## Supported versions

Security fixes are applied to the latest released version of `kc`.

## Reporting a vulnerability

Please do not open public GitHub issues for suspected vulnerabilities.

Instead, report them privately with:

- a clear description of the issue
- reproduction steps or a proof of concept
- impact assessment
- any suggested mitigation

Report issues through a private GitHub security advisory when available, or
contact the maintainer via https://github.com/v-gutierrez.

If the report is valid, we will acknowledge receipt, investigate, and coordinate
disclosure after a fix is available.

## Scope notes

`kc` is designed to work offline and store secrets in the macOS Keychain. Even
so, please report any issue that could affect confidentiality, integrity, or
availability, including CLI output leaks, shell integration problems, Touch ID
authorization issues, export behavior, or documentation that could cause unsafe
usage.

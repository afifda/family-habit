# Security quality policy

Phase 8 treats an exploitable critical or high-severity finding in application dependencies, shipped images, or deployment configuration as a release blocker. A finding may be accepted only when it is demonstrably unreachable in the shipped application, has no vendor fix, and has a documented owner, rationale, and review date.

## Automated checks

Run the complete local quality suite first, then the security scans:

```bash
make check
make security-scan
```

`make scan-dependencies` runs Go's `govulncheck` against reachable code and `npm audit` against the locked frontend dependency tree. Install `govulncheck` using the command printed by the target if it is unavailable.

`make scan-filesystem` uses a pinned Trivy container to check high and critical dependency vulnerabilities, leaked secrets, and deployment misconfiguration. `make scan-images` builds the Compose stack and scans the actual API/migration, frontend, Caddy, and PostgreSQL images. `make sbom` retains CycloneDX inventories for both application images under the gitignored release-artifact directory. Override `TRIVY_IMAGE` explicitly when upgrading the scanner, and record the scanner version with the validation result.

Scans require network access for current vulnerability databases. Never weaken the severity threshold or exclude a vulnerability merely to make a scan pass. Document any justified exception in the Phase validation report.

## HTTP and logging baseline

- The public proxy applies a restrictive same-origin content security policy, clickjacking protection, MIME-sniffing protection, privacy-preserving referrer policy, browser capability restrictions, and removes its server banner.
- API responses additionally use `Cache-Control: no-store` and deny all browser content sources.
- JSON request bodies are limited to 1 MiB and pagination endpoints cap page size.
- Request logs contain method, path without query parameters, response status, duration, and a sanitized request identifier. Cookies, authorization and CSRF headers, request bodies, query strings, child PINs, passwords, and rejection/correction reasons are never logged.
- The API and migration containers run with a read-only root filesystem, all Linux capabilities removed, and no privilege escalation.

Authentication rate limiting remains process-local because the MVP runs one API replica. It must move to shared storage before horizontal scaling.

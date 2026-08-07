# Endpoint Separation and Administrative Login Hardening

## Status and purpose

This document records recommended security changes for separating the public NFC verification service from the administrative and tag-provisioning surfaces. It is a design proposal, not a description of functionality that has already been implemented.

The principal goals are:

- Keep existing NTAG 424 DNA NDEF URLs working.
- Reduce the effect of a vulnerability in the public verification pages on authenticated administrators.
- Apply authentication and authorization to administrative routes by default.
- Harden browser sessions, CORS, CSRF protection, passwords, and login monitoring.
- Preserve the authenticated desktop/mobile tag-provisioning workflow.

## Current layout

The server currently places public and administrative functionality under the same origin and `/verify` path:

| Purpose | Current endpoint | Authentication |
|---|---|---|
| Public SDM/SUN verification | `GET/POST /verify/sun` | Public by design |
| UID originality-signature verification | `POST /verify/api` | Public by design |
| Public model result | `GET /verify/api/linkmodel/{id}` | Public by design |
| Browser admin login page | `GET /verify` | Public login page |
| Browser admin login submission | `POST /verify/admin/login` | Credentials |
| Admin panel | `GET /verify/admin` | Session cookie |
| Administrative APIs | `/verify/api/card*`, `/verify/api/model*`, and login-record APIs | Session cookie or selected app-token routes |
| Provisioning-app login | `POST /verify/api/login` | Credentials and application header |

Most administrative routes are individually protected by `checkAdminVerify`. This is useful, but individually attaching middleware creates a risk that a future route will accidentally be registered without it.

Sharing a URL prefix is not itself a security vulnerability. The material concern is that public pages and the admin application share the same browser origin. An XSS vulnerability in any public same-origin content could cause an authenticated administrator's browser to make administrative requests. Renaming `/verify` to a hidden path would provide obscurity, not isolation.

## Recommended target layout

### Public verification origin

Keep the existing public origin and NDEF URL stable:

```text
https://www.domain.com/verify/sun?d=<SDM_DATA>
```

Suggested public routes:

```text
GET/POST https://www.domain.com/verify/sun
POST     https://www.domain.com/verify/api/signature
GET      https://www.domain.com/verify/model/{id}
```

The existing `/verify/api` and `/verify/api/linkmodel/{id}` paths may initially remain as compatibility aliases. They should expose no administrative functions.

Keeping `/verify/sun` unchanged means existing tags do not need to be rewritten solely for endpoint separation.

### Administrative origin

Move the browser admin UI and all browser-admin APIs together:

```text
https://admin.domain.com/
https://admin.domain.com/login
https://admin.domain.com/api/cards/*
https://admin.domain.com/api/models/*
https://admin.domain.com/api/security/*
https://admin.domain.com/api/sessions/*
```

Moving only the login page is insufficient. If the administrative APIs remain on the public origin, the most important same-origin exposure remains.

### Provisioning origin

Tag writing is a privileged operation because it returns NFC keys and changes physical tags. It can either share the restricted admin origin or use a dedicated origin:

```text
https://provision.domain.com/api/login
https://provision.domain.com/api/cardwrite
https://provision.domain.com/api/cardsearch
https://provision.domain.com/api/modellist
```

A dedicated provisioning origin is preferable when desktop/mobile writers have different authentication, CORS, rate-limit, and network-access requirements from browser administrators.

## Isolation model

Endpoint separation should be enforced at more than one layer:

1. DNS provides separate public, admin, and optionally provisioning hostnames.
2. The reverse proxy routes requests using an exact host allowlist.
3. The public virtual host does not proxy administrative paths.
4. The application registers public and administrative route groups separately.
5. Authentication middleware is attached once to the entire admin group.
6. The admin host is restricted through VPN, IP allowlisting, mutual TLS, or an identity-aware proxy where operationally practical.

The strongest design uses separate application listeners or services. A single Go process behind host-based proxy routing can be an intermediate migration step, provided that the application also validates the expected host and does not expose admin routes through the public listener.

Separate subdomains are separate browser origins but remain the same "site" for `SameSite` cookie rules. Host-only cookies and CSRF protection are therefore still required. For stronger isolation, use an independently controlled administrative domain or private network access.

## Route authorization

Create explicit route groups with default-deny behavior:

```text
public routes
    public verification middleware

admin routes
    browser session authentication
    administrator authorization
    CSRF validation for state-changing requests
    audit logging

provisioning routes
    provisioning-client authentication
    operation-specific authorization
    request replay protection where appropriate
    audit logging
```

Do not use route names, `X-Requested-With`, the `Origin` header, or CORS as authentication. The server must authorize every sensitive operation independently.

Prefer `401 Unauthorized` or `403 Forbidden` for authenticated APIs as appropriate. Returning `404 Not Found` can be retained as a deliberate information-hiding policy, but it must not replace authentication or complicate monitoring to the point that attacks become invisible.

## Administrative session cookies

Use a dedicated cookie that is never sent to the public verification host. A recommended shape is:

```text
Set-Cookie: __Host-AdminSession=<opaque-random-id>;
            Secure;
            HttpOnly;
            SameSite=Strict;
            Path=/
```

Requirements:

- Do not set the `Domain` attribute; this makes the cookie host-only.
- Use the `__Host-` prefix, `Secure`, and `Path=/`.
- Set `HttpOnly` explicitly.
- Prefer `SameSite=Strict` for the admin application. Use `Lax` only if a documented workflow requires cross-site top-level navigation to retain the session.
- Rotate the session identifier immediately after successful login and privilege changes.
- Use an inactivity timeout plus a bounded absolute lifetime; do not rely only on the current seven-day expiry.
- Invalidate the server-side session on logout, password reset, administrator disablement, and suspected compromise.
- Do not place session IDs in URLs, application logs, or client-readable storage.
- Review active sessions and allow administrators to revoke them.

## CSRF protection

All cookie-authenticated state-changing requests must have CSRF protection, including:

```text
card deletion
card-key or card-link changes
model changes and image cleanup
password changes
session deletion
logout
```

Use the framework's maintained CSRF middleware if available. Otherwise use a synchronizer token bound to the authenticated session:

```text
X-CSRF-Token: <unpredictable session-bound token>
```

Reject missing or invalid tokens before executing the handler. Compare tokens in constant time. Also validate `Origin`/`Referer` or Fetch Metadata headers as defense in depth. `SameSite` is useful but should not be the only CSRF control.

The operator-entered `cardFileKey` remains an additional confirmation for card deletion; it is not a general CSRF control and does not protect other admin operations.

## CORS policy

The current implementation reflects the incoming `Origin` after checking an application marker header. Replace this with exact allowlists configured per environment.

Example policy:

```text
Public verification API:
    no CORS unless a browser client genuinely needs it

Admin browser API:
    Access-Control-Allow-Origin: https://admin.domain.com
    Access-Control-Allow-Credentials: true

Provisioning API:
    allow only the exact deployed desktop/mobile origins that require browser CORS
```

Additional requirements:

- Never treat `X-Requested-With` as a secret or credential.
- Do not reflect arbitrary origins.
- Reject `Origin: null` unless there is a documented and secured requirement.
- Allow only required methods and headers.
- Set `Vary: Origin` when responses can differ by allowed origin.
- Do not enable credentialed CORS on public endpoints without a specific need.
- Remember that CORS controls browser response access; it is not server-side authorization.

Native desktop clients are not protected or restricted by browser CORS. They must authenticate normally.

## Login and credential hardening

### Password storage

The current password construction uses a fast HMAC-SHA-256 calculation with a static application salt. Replace it with a password-hashing function designed to resist offline guessing:

- Prefer Argon2id with parameters selected and benchmarked for the deployment.
- bcrypt is an acceptable compatibility alternative.
- Generate a unique random salt for every password and store it with the encoded hash.
- Support hash-version and parameter upgrades on successful login.
- Remove default credentials during initial setup and require an immediate password change.

Existing hashes require a migration plan. One option is to verify the legacy hash once, then replace it with Argon2id after a successful login.

### Authentication controls

- Require MFA for administrative accounts, preferably WebAuthn/passkeys or hardware-backed TOTP where passkeys are unavailable.
- Apply rate limiting by account and source network.
- Add progressive delays or temporary lockout without enabling easy permanent denial of service.
- Return generic login failure messages.
- Record successful and failed logins, MFA failures, lockouts, logout, session revocation, and password changes.
- Alert on repeated failures, unusual source locations, and concurrent anomalous sessions.
- Restrict account creation and privilege assignment to separately authorized administrators.
- Use individual administrator identities rather than shared accounts.

### Sensitive-operation reauthentication

Require recent authentication or MFA confirmation for high-impact operations such as:

- Viewing or exporting NFC keys
- Changing application or master-key configuration
- Deleting cards in bulk
- Changing administrator credentials
- Revoking other administrator sessions

The existing per-card file-key confirmation can remain for individual card deletion.

## Security headers and admin content

Configure the admin origin with at least:

```text
Strict-Transport-Security
Content-Security-Policy with frame-ancestors 'none'
X-Content-Type-Options: nosniff
Referrer-Policy: no-referrer
Cache-Control: no-store for sensitive pages and responses
```

Avoid runtime third-party scripts on the admin panel where possible. If a third-party asset is necessary, pin its version, use Subresource Integrity, and constrain it with CSP. Serve administrative JavaScript and CSS from the admin origin.

Do not include NFC keys, passwords, tokens, session IDs, or full sensitive request bodies in logs, analytics, error pages, or browser caches.

## Public verification hardening

The public `/verify/sun` route must remain reachable, so protect availability rather than requiring authentication:

- Enforce strict input lengths and hexadecimal decoding before cryptographic processing.
- Apply request and concurrency limits.
- Use per-source and global rate limits that account for legitimate NFC traffic patterns.
- Bound handler time and request-body size.
- Keep error responses generic while retaining useful internal metrics.
- Isolate public workload capacity from admin and provisioning capacity.
- Do not send admin cookies from the public host.

## Migration plan

### Phase 1: harden the existing deployment

1. Add explicit cookie attributes and shorten/segment session lifetimes.
2. Add CSRF protection to every cookie-authenticated state-changing endpoint.
3. Replace reflective CORS with environment-specific allowlists.
4. Add rate limiting and structured security audit events.
5. Replace legacy password hashing and remove default credentials.

### Phase 2: introduce the admin origin

1. Create `admin.domain.com` and a separate TLS configuration.
2. Move admin static assets, login, logout, and all browser-admin APIs.
3. Issue a new host-only admin cookie; do not reuse the public-host session cookie.
4. Stop exposing admin routes through the public virtual host.
5. Update hardcoded admin URLs and redirects in Go handlers and browser JavaScript.

### Phase 3: separate provisioning

1. Move writer login, `cardwrite`, `cardsearch`, and required model-list operations to the chosen admin or provisioning origin.
2. Update mobile and desktop endpoint configuration.
3. Use scoped, short-lived provisioning tokens instead of a general admin session where practical.
4. Audit every response that exports `appmasterkey`, `metakey`, or per-tag `fkey`.

### Phase 4: strengthen network and service isolation

1. Put the admin/provisioning origin behind VPN, mTLS, IP restrictions, or an identity-aware proxy.
2. Use separate listeners or services for public and privileged traffic.
3. Apply independent resource limits, monitoring, and deployment permissions.

## Compatibility considerations

- Existing NDEF URLs remain valid if `/verify/sun` is retained.
- Tags need rewriting only if the public SUN URL itself changes.
- Mobile and desktop writers must be updated if provisioning APIs move.
- Browser admin JavaScript currently uses absolute paths under `/verify/api`; these must be updated or made configurable.
- Redirects and static-asset URLs in the server templates must be updated for the admin origin.
- Tests should assert that public hosts cannot reach admin handlers, not only that unauthenticated handlers return an error.
- CORS tests should cover allowed origins, denied origins, credentials, preflight requests, and missing `Origin` headers.

## Acceptance checklist

- [ ] Existing tags still resolve `/verify/sun` successfully.
- [ ] The public virtual host has no routable admin or provisioning endpoints.
- [ ] Admin cookies are host-only, `Secure`, `HttpOnly`, and explicitly `SameSite` protected.
- [ ] Session identifiers rotate after login.
- [ ] All cookie-authenticated mutations validate CSRF tokens.
- [ ] CORS uses exact configured origins and never reflects arbitrary origins.
- [ ] Administrative routes inherit authentication and authorization from a route group.
- [ ] Provisioning tokens are scoped to required operations.
- [ ] Passwords use Argon2id or an approved equivalent with unique salts.
- [ ] MFA is enabled for administrators.
- [ ] Login and sensitive operations produce security audit events.
- [ ] Sensitive responses use `Cache-Control: no-store` and are not logged.
- [ ] Admin and public services have independent rate/resource limits.
- [ ] Backup, restore, key rotation, and administrator lockout recovery have been tested.

## References

- [OWASP Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)
- [OWASP Cross-Site Request Forgery Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Cross-Site_Request_Forgery_Prevention_Cheat_Sheet.html)
- [OWASP Password Storage Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)
- [OWASP REST Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/REST_Security_Cheat_Sheet.html)
- [OWASP HTTP Headers Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/HTTP_Headers_Cheat_Sheet.html)
- [NIST SP 800-63B, Authentication and Lifecycle Management](https://pages.nist.gov/800-63-4/sp800-63b.html)

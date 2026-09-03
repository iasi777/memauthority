# Transport and authentication contract — 1.3.2

## stdio

`serve` defaults to stdio. OAuth configuration is ignored for stdio. A valid committed clean Vault and an external state directory are still required for managed admission.

## HTTP exposure

HTTP is selected explicitly with `--transport http`. The default listen address is loopback (`127.0.0.1:8000`). Without OAuth, HTTP listen is restricted to loopback (`localhost`, IPv4 loopback or IPv6 loopback). Non-loopback HTTP requires OAuth.

Write-enabled HTTP requires OAuth. HTTP uses defensive non-zero read-header/read/write/idle timeouts and a bounded maximum header size.

Exact Host and Origin allowlists may be configured. Their deployment-specific values are not compatibility constants.

## Embedded OAuth

For HTTP OAuth, the public issuer/base URL, client ID, exact redirect URI set, username and password file are required. OAuth state is stored outside Vault Authority. A confidential-client secret is optional; when configured, metadata supports both `client_secret_post` and `client_secret_basic`. Without a client secret, the public-client method is `none`.

Authorization code flow requires PKCE S256. Supported scopes are `memory` and `offline_access`, and `memory` is required. Refresh tokens rotate; replay of a rotated refresh token revokes the token family. Access/refresh token raw values are not stored in the OAuth database.

OAuth client IDs, secrets, usernames/passwords, redirect URIs, issuer domains and production resource URLs are deployment configuration, not fixed public contract constant values.

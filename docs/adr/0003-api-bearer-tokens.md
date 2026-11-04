# The API uses bearer tokens and bypasses cross-origin protection

`/api/v1` authenticates with personal tokens sent as `Authorization: Bearer ...`, not session cookies, and is exempted from `http.NewCrossOriginProtection`. The browser extension runs on a different origin, and cross-site request forgery depends on browsers attaching cookies automatically, which they don't do for a bearer token. The bypass looks like a security hole but is deliberate: it must only ever cover routes that ignore cookies, and CORS limits which origins may call them. Tokens are stored only as hashes.

## Considered Options

- **Session cookies for the extension:** would need cross-site cookies, and would bring back the forgery risk the protection exists to stop.

## Consequences

- API handlers must never accept session authentication.
- Allowed origins are configurable, because an extension's ID differs between builds.

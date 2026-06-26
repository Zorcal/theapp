# auth

Magic-link authentication and JWT/refresh-token issuance.

## Design decisions and known tradeoffs

**Access tokens cannot be revoked before expiry.** JWTs are stateless; closing this gap requires a denylist checked on every authenticated request — a DB round-trip on the hot path. At a 15-minute TTL the exposure window is small enough that the cost isn't justified. Shorten `AccessTokenTTL` if tighter guarantees are needed.

**Refresh token reuse is detected at the concurrent-request level only.** If two requests race to rotate the same token, the second is rejected. There is no token-family chain, so a stolen token rotated silently by an attacker before the legitimate user refreshes cannot be detected. Full family tracking (linking each token to its parent and invalidating the whole family on reuse) would catch this, but introduces false positives when a network failure causes the client to retry with the superseded token.

**Rate limiting does not apply to new accounts.** The cooldown window is checked only when a user already exists. A new email address bypasses it entirely, allowing unbounded account creation and email delivery. Rate limiting by IP or globally belongs at the API gateway or infrastructure layer, not in core business logic.

**HS256 (symmetric) JWT signing.** The same key signs and verifies tokens, so anything that can verify can also mint. Keep one service (e.g. this one) as the only process that ever parses a raw client Bearer token, and have everything else behind it in the private network trust an internal signal instead of reverifying the JWT — mTLS service identity, or a trusted `x-user-id` set by the verifying service/gateway. Switching to RS256/ES256 only earns its keep once a second binary needs to independently verify client-presented tokens — e.g. a service with a smaller trust footprint than the issuer (more externally exposed, different team, weaker audit trail). Distributing only the public key then means compromising that second verifier can't be used to forge tokens for the rest of the system.

**Rate-limit check includes consumed and expired tokens.** `LatestMagicLinkTokenCreatedAt` returns the `created_at` of the most recent token regardless of whether it was consumed or has expired. Consuming a link and immediately requesting a new one still triggers the cooldown. This is intentional: it prevents clients from link-farming by rapidly consuming and re-requesting, and the 1-minute window matches the expected re-delivery delay for transient email failures.

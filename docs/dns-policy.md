# DNS Policy

The server supports optional reverse-DNS policy checks for incoming clients.

## What It Can Do

- Allow clients only when PTR result matches allow suffixes.
- Deny clients when PTR result matches deny suffixes.
- Resolve PTR records through custom nameservers.
- Cache reverse lookup results to reduce DNS load.

## Configuration Fields

- `policy.dns.enableReverseLookup`: turns feature on/off.
- `policy.dns.allowSuffixes`: whitelist suffixes (for example `corp.example.com`).
- `policy.dns.denySuffixes`: blacklist suffixes.
- `policy.dns.cacheTTLSeconds`: cache lifetime for lookup results.
- `policy.dns.lookupTimeoutMillis`: timeout per reverse query.
- `policy.dns.allowOnLookupError`: decide fail-open or fail-closed on DNS failures.
- `policy.dns.resolverNameservers`: custom recursive resolvers.

## Evaluation Order

1. Reverse lookup is executed (or loaded from cache).
2. Deny suffixes are checked first.
3. If `allowSuffixes` is empty and deny check passed, request is allowed.
4. If `allowSuffixes` exists, at least one suffix must match.

## Recommendation

For internet-wide public NTP, leave reverse DNS policy disabled unless you have a controlled fleet.

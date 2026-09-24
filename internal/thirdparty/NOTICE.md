# Third-party code

The directories below contain source derived from official Go subrepositories
and are kept internal to preserve Go 1.24 compatibility while retaining current
IDNA security fixes.

- internal/thirdparty/xnet/idna
  - Source: golang.org/x/net/idna
  - Upstream version: v0.55.0
  - License: BSD-3-Clause (see internal/thirdparty/xnet/LICENSE)

- internal/thirdparty/xtext/transform
- internal/thirdparty/xtext/unicode/norm
- internal/thirdparty/xtext/unicode/bidi
- internal/thirdparty/xtext/secure/bidirule
  - Source: golang.org/x/text
  - Upstream version: v0.39.0
  - License: BSD-3-Clause (see internal/thirdparty/xtext/LICENSE)

Local changes are limited to import-path rewrites into this module's internal
namespace. The copied code itself builds and tests with Go 1.24 even though the
upstream module-level go directives now require Go 1.25.

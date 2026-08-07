# Feature Specification: Identity, Credentials & Access Control

Kind: journey
Status: shipped
Constitution: v1.0.0

This document was reconstructed from the implementation at commit `23a167c`
(`v0.4.0-3-g23a167c`); every requirement below cites the code it was read from, and where the
code does something a reader would not expect it is written down as found rather than smoothed
into a requirement that was met.

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Bootstrap an instance and become its administrator (Priority: P1)

A person deploys ContactsHQ on their own hardware, opens the API, and creates the first account.
That account is granted the administrator role. From that moment public sign-up is closed unless
the operator has explicitly opened it, so the deployment is not left handing out accounts to
anyone who can reach the port.

**Why this priority**: nothing else in the system can be reached without this step, and no other
code path ever assigns the administrator role. If bootstrap fails, the only recovery is editing
the database by hand.

**Independent Test**: point a fresh, empty database at the server, `POST /api/v1/auth/register`
once, and confirm the response carries `"role": "admin"`; repeat the same call with a second
address and confirm `403`.

**Acceptance Scenarios**:

1. **Given** an empty database and `auth.allow_registration` unset (default `false`), **When** a
   client posts an email and an 8+ character password to `/api/v1/auth/register`, **Then** the
   response is `201` and the created user's `role` is `admin`
   (`internal/service/auth.go:112-129`, `internal/handler/registration_policy_test.go:93`).
2. **Given** an instance that already has one account and registration closed, **When** another
   client posts to `/api/v1/auth/register`, **Then** the response is `403` with body
   `{"error": "registration is closed"}` (`internal/handler/auth_handler.go:68-70`).
3. **Given** the operator sets `CHQ_AUTH_ALLOW_REGISTRATION=true`, **When** a second client
   registers, **Then** the response is `201` and the role is `user`
   (`internal/service/auth_registration_policy_test.go:39`).
4. **Given** any instance, **When** a client with no token requests `GET /api/v1/auth/config`,
   **Then** it receives `200` and `{"registration_open": <bool>}`, where the value is `true` on an
   empty instance even with registration closed (`internal/handler/handler.go:91`,
   `internal/service/auth.go:90-99`).
5. **Given** a new account of any kind, **When** it is created, **Then** an address book named
   "Contacts" is created for it in the same call (`internal/service/auth.go:146-155`).

---

### User Story 2 - Sign in and stay signed in (Priority: P1)

A user signs in with email and password and receives an access token and a refresh token. The web
UI attaches the access token to every API call and silently exchanges the refresh token when the
access token expires, so a working session survives an hour-long access-token lifetime without a
re-prompt.

**Why this priority**: it is the only way into the API for a human. Every other story in this spec
assumes a bearer token exists.

**Independent Test**: log in, wait past the access-token TTL (or forge an expired one), issue any
protected request through the SPA, and confirm one refresh call is made and the original request
is replayed.

**Acceptance Scenarios**:

1. **Given** correct credentials, **When** `POST /api/v1/auth/login` is called, **Then** the
   response contains `access_token`, `refresh_token` and `expires_at`
   (`internal/service/auth.go:234-279`).
2. **Given** a wrong password or an unknown email, **When** login is called, **Then** the response
   is `401` with `{"error": "invalid credentials"}` — the same body in both cases
   (`internal/handler/auth_handler.go:101-103`).
3. **Given** a valid refresh token, **When** `POST /api/v1/auth/refresh` is called, **Then** a new
   pair is issued and the claims are rebuilt from the *current* stored user, so a role change
   reaches the next access token (`internal/service/auth.go:184-199`).
4. **Given** a refresh token presented as an API bearer token, **When** any protected route is
   called, **Then** the response is `401`: the `typ` claim must be `access`
   (`internal/service/auth.go:201-232`, tests `internal/service/auth_test.go:85` and `:100`).
5. **Given** concurrent API calls that all receive `401`, **When** the SPA handles them, **Then**
   exactly one refresh request is issued and the queued calls are replayed
   (`web/src/api/client.ts:17-79`, `web/src/api/client.spec.ts:52`).
6. **Given** a rejected refresh token, **When** the SPA handles it, **Then** both tokens are
   cleared from local storage and the browser is sent to `/app/login`
   (`web/src/api/client.ts:23-27`).

---

### User Story 3 - Administer the people on the instance (Priority: P2)

The administrator lists the accounts on the instance, adds a colleague even though public sign-up
is closed, promotes or demotes someone, and removes an account.

**Why this priority**: it is how a multi-person instance is run, but a single-user deployment never
needs it.

**Independent Test**: sign in as the bootstrap account, call the four `/api/v1/admin/*` routes,
then repeat each with a non-admin token and confirm `403`.

**Acceptance Scenarios**:

1. **Given** an administrator's access token, **When** `GET /api/v1/admin/users` is called,
   **Then** a paginated list is returned with `users`, `total`, `limit`, `offset`
   (`internal/handler/admin_handler.go:18-33`).
2. **Given** an instance with registration closed, **When** the administrator posts to
   `/api/v1/admin/users`, **Then** the account is created with role `user` and public
   `/auth/register` remains closed — the admin route is deliberately wired to the
   policy-bypassing create path, not back through registration
   (`internal/handler/handler.go:250`, `internal/handler/auth_handler.go:43-45`,
   `internal/handler/registration_policy_test.go:142`).
3. **Given** a non-admin access token, **When** any `/api/v1/admin/*` route is called, **Then** the
   response is `403` with `{"error": "admin access required"}`
   (`internal/handler/middleware/admin.go`, `internal/handler/registration_policy_test.go:169`).
4. **Given** an administrator, **When** `PUT /api/v1/admin/users/:id/role` is called with anything
   other than `"user"` or `"admin"`, **Then** the response is `400`
   (`internal/handler/admin_handler.go:47-49`).
5. **Given** an administrator deletes a user, **When** the row is removed, **Then** every table
   keyed on that user is cascaded by the database
   (`internal/repository/bun_user.go:48-51`, `ON DELETE CASCADE` in `migrations/001_init.up.sql`
   and twelve later migrations; SQLite enforcement relies on the `foreign_keys` pragma set in
   `internal/repository/db.go:33`).

---

### User Story 4 - Manage my own account (Priority: P2)

A signed-in user reads their profile, changes their display name or email, and changes their
password by supplying the old one. A password change takes effect for CardDAV clients immediately,
not after a cache expires.

**Why this priority**: routine self-service; the instance functions without it, but a password
that cannot be changed from the UI is a support burden.

**Independent Test**: change the password through `PUT /api/v1/users/me/password`, then attempt a
CardDAV request with the old password in the same second and confirm `401`.

**Acceptance Scenarios**:

1. **Given** a valid token, **When** `GET /api/v1/users/me` is called, **Then** the profile is
   returned and never includes the password hash (`json:"-"` on
   `internal/domain/user.go:14`).
2. **Given** an email already used by another account, **When** `PUT /api/v1/users/me` tries to
   take it, **Then** the response is `409` (`internal/service/user.go:73-82`).
3. **Given** a wrong current password, **When** `PUT /api/v1/users/me/password` is called, **Then**
   the response is `401` and nothing is written (`internal/service/user.go:105-107`).
4. **Given** a new password shorter than 8 characters, **When** the change is submitted, **Then**
   the response is `400` (`internal/handler/user_handler.go:73-75`).
5. **Given** a successful password change, **When** a CardDAV client retries with the old password,
   **Then** it is rejected immediately because the cached verdicts for that account were dropped
   (`internal/service/user.go:120`, `cmd/server/main.go:296`,
   `internal/service/user_set_password_test.go:99`).
6. **Given** a valid token, **When** `DELETE /api/v1/users/me` is called, **Then** the account is
   deleted (`internal/handler/user_handler.go:88-96`). No SPA screen calls this route —
   `web/src/api/users.ts` exposes only get/update/change-password.

---

### User Story 5 - Give a phone its own credential (Priority: P2)

A user creates an app-specific password for a CardDAV client, sees the token once, uses it on the
device, watches "last used" update, and revokes it when the device is lost — which cuts the device
off on its next request.

**Why this priority**: the supported way to attach a mobile client without putting the account
password on the device. CardDAV is usable without it (the main password works), so it is not P1.

**Independent Test**: create an app password, authenticate a CardDAV request with it, delete it,
and confirm the same request now returns `401` without waiting.

**Acceptance Scenarios**:

1. **Given** a signed-in user, **When** `POST /api/v1/app-passwords` is called with a label,
   **Then** `201` is returned with a 64-character hex token generated from 32 random bytes
   (`internal/service/app_password.go:58,127-133`).
2. **Given** the token was shown once, **When** the list is fetched again, **Then** no endpoint
   ever returns it: only the argon2id hash is stored and it is excluded from JSON
   (`internal/domain/app_password.go:15`).
3. **Given** an account that already holds 20 app passwords, **When** another is requested,
   **Then** the response is `409` naming the limit
   (`internal/service/app_password.go:24,54`, `internal/handler/app_password_handler.go:36-41`,
   `internal/service/app_password_limit_test.go:82`).
4. **Given** an app password belonging to another user, **When** its owner-scoped delete is
   attempted, **Then** the response is `404` (`internal/service/app_password.go:86-93`).
5. **Given** a successful delete, **When** a device retries with the deleted token, **Then** it is
   rejected immediately: deletion drops the cached verdicts for that account
   (`internal/service/app_password.go:98-105`,
   `internal/service/app_password_limit_test.go:122`).
6. **Given** a successful CardDAV authentication with an app password, **When** the list is
   refreshed, **Then** `last_used_at` reflects that use
   (`internal/carddav/server.go:244`, `internal/repository/bun_app_password.go:56-63`).

---

### User Story 6 - Authenticate a CardDAV client cheaply and safely (Priority: P1)

A phone syncs contacts over `/dav` using HTTP Basic authentication. It sends dozens of requests per
session; the server must not pay a 64 MiB argon2id hash for each of them, must not be knocked over
by an attacker firing the same requests, and must not lock the owner's phone out as a side effect
of that defence.

**Why this priority**: `/dav` is the one authenticated surface reachable without ever visiting the
web UI, and it has no route-level rate limiter of its own.

**Independent Test**: drive `/dav` with a wrong password from one address until it is blocked, and
confirm a correct credential from another address is unaffected.

**Acceptance Scenarios**:

1. **Given** a request to `/dav` with no `Basic` credentials, **When** it arrives, **Then** the
   response is `401` with `WWW-Authenticate: Basic realm="ContactsHQ CardDAV"`
   (`internal/carddav/server.go:71-77`).
2. **Given** valid credentials, **When** the same credentials are presented again within five
   minutes, **Then** the verdict is served from cache with no password hashing
   (`internal/carddav/authcache.go:18-22`, `internal/carddav/server.go:160-167`).
3. **Given** an account whose main password does not match, **When** app passwords exist, **Then**
   they are tried most-recently-used first so the common case hashes once
   (`internal/carddav/server.go:221-249`,
   `internal/carddav/app_password_loop_test.go:86`).
4. **Given** 10 failed attempts from one source address within five minutes, **When** the next
   attempt arrives, **Then** the response is `429` with `Retry-After: 300` and no hashing occurs
   (`internal/carddav/throttle.go:26-31`, `internal/carddav/server.go:94-98`,
   `internal/carddav/auth_throttle_http_test.go:85`).
5. **Given** a blocked source address, **When** a *different* address presents correct credentials,
   **Then** it is served: the counter is per address and there is deliberately no per-account
   counter (`internal/carddav/throttle.go:21-25`,
   `internal/carddav/auth_throttle_http_test.go:104`).
6. **Given** a client already holding a cached positive verdict, **When** its address happens to be
   blocked, **Then** it still works — the cache is consulted before the throttle
   (`internal/carddav/server.go:153-167`,
   `internal/carddav/auth_throttle_http_test.go:121`).
7. **Given** many simultaneous credential checks, **When** they run, **Then** at most four hash at
   once, bounding peak hashing memory at 4 × 64 MiB
   (`internal/carddav/throttle.go:27`, `internal/carddav/server.go:193-198`,
   `internal/carddav/argon2_semaphore_test.go:50`).
8. **Given** `server.trusted_proxies` names the reverse proxy, **When** requests arrive through it,
   **Then** each forwarded client gets its own failure bucket; **and given** the peer is not a
   configured proxy, **Then** `X-Forwarded-For` is ignored entirely
   (`internal/carddav/throttle.go:166-196`,
   `internal/carddav/auth_throttle_http_test.go:136` and `:152`).

---

### User Story 7 - Recover a forgotten password from the host (Priority: P3)

The only administrator forgets the password. With shell access to the host (or `docker exec`), the
operator runs `contactshq set-password <email>`, types a new password twice without it appearing on
screen or in argv, and is told plainly what the command does *not* do.

**Why this priority**: rare, but it is the sole recovery path — there is no password-reset email
flow anywhere in the codebase.

**Independent Test**: run the command against a test database from a non-terminal stdin without
`--stdin` and confirm exit code 2 and a message explaining the flag.

**Acceptance Scenarios**:

1. **Given** an interactive terminal, **When** `set-password you@example.com` runs, **Then** the
   password is prompted twice with echo disabled and mismatches are rejected
   (`cmd/server/cli.go:230-256`).
2. **Given** a pipe, **When** `--stdin` is passed, **Then** the first line of stdin is used; without
   the flag the command refuses rather than reading a non-terminal
   (`cmd/server/cli.go:214-234`, `cmd/server/cli_test.go:66`).
3. **Given** a password written as a command-line argument, **When** the command runs, **Then** it
   is a usage error: the command accepts exactly one positional, the email
   (`cmd/server/cli.go:155-163`, `cmd/server/cli_test.go:54`). The prohibition itself is stated by
   **008 FR-060**, which owns `cmd/server/cli.go`; this scenario records how it is observed here.
4. **Given** a new password shorter than 8 characters, **When** it is entered, **Then** the command
   exits without writing, and it never creates an account and never changes a role
   (`internal/service/user.go:124-155`, `internal/service/user_set_password_test.go:69`).
5. **Given** an unknown email, **When** the command runs, **Then** the exit code distinguishes that
   case from a short password, an unreachable database and an unmigrated schema. The code values
   are defined by **008 FR-059** (`cmd/server/cli.go:21-29`); what this spec requires is that the
   four situations remain distinguishable (SC-009).
6. **Given** success, **When** the command finishes, **Then** it prints that existing sessions stay
   signed in and that a running server keeps cached CardDAV verdicts for up to five minutes
   (`cmd/server/cli.go:200-211`, `cmd/server/cli_test.go:104`).

---

### Edge Cases

- **What happens when an account somehow holds more app passwords than the creation cap allows?**
  Verification still tries every stored password: the loop is deliberately uncapped, because
  truncating it would silently revoke whichever passwords sorted last
  (`internal/carddav/server.go:215-220`, `internal/carddav/app_password_loop_test.go:52`).
- **What happens when the same account is attacked from many addresses?** Nothing: there is no
  per-account lockout, by design. The CardDAV login is the email and is usually public, so a
  counter keyed on it would be a ready-made way to lock the owner's phone out
  (`internal/carddav/throttle.go:21-25`).
- **What happens when a client with correct credentials is behind a blocked address and holds no
  cached verdict?** It receives `429`, not `401`. The throttle is consulted before any verification
  and does not know the credential is good (`internal/carddav/server.go:94-98`); only a cached
  positive short-circuits it (FR-035).
- **How does the system handle a tampered client-side role?** The SPA decodes the JWT locally only
  to hide admin navigation (`web/src/stores/auth.ts:20-27`, `web/src/router/index.ts:166`);
  `isAdmin` there is advisory and enforcement is entirely server-side (FR-015).

## Requirements *(mandatory)*

> **FR numbers are stable identifiers.** Four requirements (FR-045, FR-046, FR-048, FR-049) were
> withdrawn when ownership was settled with spec 008; their numbers are retained as labelled
> cross-references rather than reused, so an external citation never silently points at a different
> rule.

### Functional Requirements

**Account creation and the bootstrap rule**

- **FR-001**: Creating the very first account MUST always be permitted, regardless of the sign-up
  policy, so a fresh deployment can be bootstrapped without database access.
  (`internal/service/auth.go:112-119`)
- **FR-002**: The first account created on an instance MUST be given the `admin` role; no other
  code path assigns it. (`internal/service/auth.go:126-129`)
- **FR-003**: Once an account exists, `POST /api/v1/auth/register` MUST return `403` unless
  `auth.allow_registration` is enabled; the setting defaults to `false`.
  (`internal/service/auth.go:117`, `internal/config/config.go:183`,
  `internal/handler/auth_handler.go:68-70`)
- **FR-004**: Registration MUST reject a missing email or password with `400`, a password shorter
  than 8 characters with `400`, and an email already in use with `409`.
  (`internal/handler/auth_handler.go:55-67`)
- **FR-005**: `POST /api/v1/admin/users` MUST create accounts through the policy-bypassing path so
  an administrator can add people on an instance closed to public sign-up; it MUST NOT be routed
  back through the registration policy. (`internal/handler/handler.go:250`,
  `internal/handler/auth_handler.go:43-45`)
- **FR-006**: `GET /api/v1/auth/config` MUST be reachable without a token and MUST report whether
  the public sign-up path would currently accept an account, returning `true` on an instance with
  no accounts even when the policy is closed. (`internal/handler/handler.go:91`,
  `internal/service/auth.go:90-99`)
- **FR-007**: Creating an account MUST also create that user's "Contacts" address book.
  (`internal/service/auth.go:146-155`)

**Sign-in, tokens and the API barrier**

- **FR-008**: `POST /api/v1/auth/login` MUST return an access token, a refresh token and the access
  token's expiry, and MUST answer both an unknown email and a wrong password with the same `401`
  body. (`internal/service/auth.go:168-182`, `internal/handler/auth_handler.go:99-107`)
- **FR-009**: Tokens MUST be HS256 JWTs carrying `user_id`, `email`, `role` and a `typ` claim, and
  validation MUST reject any other signing method. (`internal/service/auth.go:212-215,238-268`)
- **FR-010**: The `typ` claim MUST prevent a refresh token being used as an API bearer token and an
  access token being used to mint a new pair. (`internal/service/auth.go:201-232`)
- **FR-011**: Access and refresh lifetimes MUST be configurable, defaulting to 1 hour and 168 hours.
  (`internal/config/config.go:181-182`)
- **FR-012**: There MUST be no token revocation list; the token lifetime is the revocation window,
  and instance-wide invalidation is achieved by rotating the signing secret.
  (`internal/config/config.go:177-180`)
- **FR-013**: Refresh MUST rebuild claims from the stored user, so a role change or a deletion takes
  effect at the next refresh; a refresh for a deleted user MUST fail.
  (`internal/service/auth.go:184-199`)
- **FR-014**: Every route under `/api/v1` except `/auth/register`, `/auth/login`, `/auth/refresh`,
  `/auth/config` and the Google OAuth callback MUST require a valid `Authorization: Bearer` access
  token and answer `401` otherwise. (`internal/handler/handler.go:84-100`,
  `internal/handler/middleware/auth.go`)
- **FR-015**: `/api/v1/admin/*` MUST additionally require the `admin` role and answer `403`
  otherwise. (`internal/handler/handler.go:246`, `internal/handler/middleware/admin.go`)
- **FR-016**: The server MUST refuse to start without a signing secret of at least 32 characters
  that is not one of the known placeholder values. (`internal/config/config.go:362-381`)

**Password storage**

- **FR-017**: Passwords MUST be stored as argon2id hashes with 64 MiB memory, 1 iteration, 4 lanes,
  a 16-byte random salt and a 32-byte key, encoded as
  `$argon2id$v=19$m=…,t=…,p=…$salt$hash`. (`internal/service/auth.go:283-304`)
- **FR-018**: Verification MUST use a constant-time comparison.
  (`internal/service/auth.go:338`, `internal/carddav/server.go:310-317`)
- **FR-019**: A hash written by the API MUST be accepted by the CardDAV verifier and vice versa,
  since the two implementations are independent. (`internal/carddav/server.go:251-318`,
  `internal/service/user_set_password_test.go:128`)
- **FR-020**: No API response may include a password hash. (`internal/domain/user.go:14`,
  `internal/domain/app_password.go:15`)

**Self-service account management**

- **FR-021**: `GET /api/v1/users/me` MUST return the caller's profile.
  (`internal/handler/user_handler.go:18-30`)
- **FR-022**: `PUT /api/v1/users/me` MUST update display name and/or email, and MUST return `409`
  when the requested email belongs to another account. (`internal/service/user.go:64-94`)
- **FR-023**: `PUT /api/v1/users/me/password` MUST require the current password, MUST enforce the
  8-character minimum, and MUST answer `401` when the current password is wrong.
  (`internal/handler/user_handler.go:61-86`, `internal/service/user.go:96-107`)
- **FR-024**: A successful password change MUST invalidate that account's cached CardDAV
  authentication verdicts immediately. (`internal/service/user.go:120`, `cmd/server/main.go:296`)
- **FR-025**: `DELETE /api/v1/users/me` MUST delete the caller's account, and the database MUST
  cascade every dependent row. (`internal/handler/user_handler.go:88-96`,
  `internal/repository/bun_user.go:48-51`, `ON DELETE CASCADE` across `migrations/`)

**App passwords as a second credential class**

- **FR-026**: `POST /api/v1/app-passwords` MUST require a label and MUST return a freshly generated
  256-bit token exactly once, in the creation response only.
  (`internal/handler/app_password_handler.go:23-51`, `internal/service/app_password.go:49-80`)
- **FR-027**: An app password MUST be stored only as an argon2id hash.
  (`internal/service/app_password.go:63-73`)
- **FR-028**: An account MUST hold at most 20 app passwords; exceeding it MUST return `409`.
  (`internal/service/app_password.go:19-24,54`)
- **FR-029**: Listing MUST expose id, label, `last_used_at` and `created_at` and never the token.
  (`internal/handler/app_password_handler.go:53-62`, `internal/domain/app_password.go`)
- **FR-030**: Deletion MUST be scoped to the owner (`404` otherwise) and MUST invalidate that
  account's cached CardDAV verdicts. (`internal/service/app_password.go:86-106`)
- **FR-031**: A successful authentication with an app password MUST record `last_used_at`.
  (`internal/carddav/server.go:244`, `internal/service/app_password.go:119`)

**Credential verification at the CardDAV boundary**

- **FR-032**: `/dav` MUST require HTTP Basic credentials and MUST answer `401` with a
  `WWW-Authenticate` challenge when they are missing or malformed.
  (`internal/carddav/server.go:71-103`)
- **FR-033**: Verification MUST try the account's main password first and fall back to its app
  passwords, ordered most-recently-used first. (`internal/carddav/server.go:190-249`)
- **FR-034**: Verdicts MUST be cached for 5 minutes when positive and 30 seconds when negative,
  keyed on the email plus a SHA-256 of the presented password so no password is held in memory as
  plaintext; the cache MUST be bounded at 1024 entries.
  (`internal/carddav/authcache.go:18-22,46-49,76-83`)
- **FR-035**: A cached positive MUST be answered before the failure throttle is consulted, so a
  working client is never locked out by another client's failures.
  (`internal/carddav/server.go:153-167`)
- **FR-036**: 10 failed attempts from one client address within a 5-minute sliding window MUST block
  that address for 5 minutes, answered `429` with `Retry-After`; a success MUST clear the counter.
  (`internal/carddav/throttle.go:26-31,78-117`, `internal/carddav/server.go:94-98`)
- **FR-037**: At most 4 password verifications may run concurrently on the CardDAV path, bounding
  peak hashing memory; a cancelled request MUST NOT take a slot.
  (`internal/carddav/throttle.go:27`, `internal/carddav/server.go:193-198`)
- **FR-038**: The CardDAV failure counter MUST be keyed on the client address as resolved by the
  trusted-proxy rule of FR-044, so behind a configured proxy each forwarded client gets its own
  bucket. (`internal/carddav/throttle.go:166-196`, `cmd/server/main.go:288-291`)
- **FR-039**: There MUST NOT be a failure counter keyed on the account, because the CardDAV login is
  public and such a counter would be a denial-of-service against the owner.
  (`internal/carddav/throttle.go:21-25`)
- **FR-040**: The credential invalidation callbacks MUST be wired from the user service and the
  app-password service to the CardDAV verdict cache in the composition root.
  (`cmd/server/main.go:293-297`, `internal/service/user.go:20-45`)

**Credential-cost rate limiting**

- **FR-041**: `POST /auth/register` and `POST /auth/login` MUST share one bucket of 10 requests per
  minute per client, because each costs a 64 MiB argon2id operation.
  (`internal/handler/handler.go:85-87`, `internal/handler/middleware/ratelimit.go:11-14`)
- **FR-042**: `POST /auth/refresh` MUST have its own, more generous bucket of 60 per minute, because
  it only verifies an HMAC. (`internal/handler/handler.go:88`,
  `internal/handler/middleware/ratelimit.go:16-18`)
- **FR-043**: Exceeding a bucket MUST return `429` with
  `{"error": "too many attempts, try again later"}`.
  (`internal/handler/middleware/ratelimit.go:44-48`)
- **FR-044**: `X-Forwarded-For` MUST be believed only when the request arrived through a peer
  matching `server.trusted_proxies`; otherwise the direct peer's address is the key. This is the
  single trust rule for both per-client buckets in this domain — the credential-cost rate limiter
  and the CardDAV failure throttle of FR-038. (`cmd/server/main.go:225-235`,
  `internal/handler/middleware/ratelimit.go:41`,
  `internal/handler/middleware/ratelimit_test.go:91,123`)

**Password recovery from the host**

- **FR-045**: *Withdrawn — cross-reference only.* "A secret MUST NEVER be accepted as a
  command-line argument; prompt twice without echo, or read stdin behind an explicit `--stdin`" is
  stated normatively by **008 FR-060**, which owns `cmd/server/cli.go` and binds every subcommand,
  and by Constitution Principle III. This spec observes it in User Story 7 scenario 3 and measures
  its consequence in SC-008.
- **FR-046**: *Withdrawn — cross-reference only.* Interleaved flag parsing (`parseInterleaved`
  rather than `fs.Parse`) is a property of the shared parser at `cmd/server/cli.go:69`, used by
  every subcommand; it is stated by **008 FR-061**.
- **FR-047**: `contactshq set-password` MUST enforce the same 8-character minimum as the HTTP API,
  MUST NOT create an account, and MUST NOT change a role — recovering access must not be a way to
  grant yourself one. (`internal/service/user.go:124-155`)
- **FR-048**: *Withdrawn — cross-reference only.* The exit-code table is declared once as package
  constants at `cmd/server/cli.go:21-29` and shared by every subcommand; it is stated by
  **008 FR-059**. This spec keeps only the outcome it depends on, SC-009.
- **FR-049**: *Withdrawn — cross-reference only.* "A subcommand MUST NOT run migrations and MUST
  refuse to operate on a database with no schema" is a property of the shared helper
  `openCLIDatabase` (`cmd/server/cli.go:108-136`); it is stated by **008 FR-062**.
- **FR-050**: On success `set-password` MUST state that existing sessions remain valid and that a
  running server keeps cached CardDAV verdicts. (`cmd/server/cli.go:198-211`)

**Web client behaviour**

- **FR-051**: The SPA MUST attach the access token to API calls and, on a `401`, MUST attempt
  exactly one refresh for concurrent failures before replaying them.
  (`web/src/api/client.ts:9-79`)
- **FR-052**: When no refresh token exists or the refresh is rejected, the SPA MUST clear both
  tokens and navigate to the login screen. (`web/src/api/client.ts:23-27,68-73`)
- **FR-053**: The admin screens MUST allow listing users, creating a user, changing a role and
  deleting a user, and MUST be hidden from non-admins client-side while the server enforces the
  barrier. (`web/src/views/admin/`, `web/src/api/admin.ts`, `web/src/router/index.ts:166`)
- **FR-054**: The app-password screen MUST show the generated token once with an explicit
  "copy it now" warning and MUST warn on deletion that devices lose access immediately.
  (`web/src/views/settings/AppPasswordsView.vue:30-75`)

### Key Entities

- **User** — id (UUID generated in Go), unique email, argon2id password hash (never serialised),
  display name, role (`user` | `admin`), created/updated timestamps. Owns everything else in the
  system through `ON DELETE CASCADE`. (`internal/domain/user.go`, `migrations/001_init.up.sql`)
- **App password** — id, owning user, label, argon2id hash of a 256-bit token, `last_used_at`,
  `created_at`. Up to 20 per user; deleted with the user.
  (`internal/domain/app_password.go`, `migrations/017_app_passwords.up.sql`)
- **Token pair** — access token, refresh token, access expiry (Unix seconds). Not persisted
  anywhere; the server keeps no record of issued tokens. (`internal/service/auth.go:58-62`)
- **Claims** — `user_id`, `email`, `role`, `typ`, plus standard `sub`, `iat`, `exp`. The `role`
  claim is what the admin barrier reads. (`internal/service/auth.go:64-70`)
- **Auth verdict (in-memory)** — ok flag, user id, user email, expiry; keyed by
  `email + ":" + sha256(password)`. Process-local, capped at 1024 entries, never persisted.
  (`internal/carddav/authcache.go:24-49`)
- **Failure record (in-memory)** — count, first-failure time, blocked-until, keyed by client
  address. Process-local, capped at 1024 addresses. (`internal/carddav/throttle.go:34-38`)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A person with a fresh deployment and no database access can obtain a working
  administrator account in exactly one API call, with no configuration change.
- **SC-002**: On an instance that already has an account, an anonymous sign-up attempt fails 100% of
  the time unless the operator has explicitly enabled public registration; the administrator's
  ability to add people is unaffected in both states.
- **SC-003**: A CardDAV sync session pays at most one password-hashing operation per credential per
  5 minutes, instead of one per request — the difference between one 64 MiB hash and hundreds for a
  single phone sync.
- **SC-004**: Peak memory attributable to CardDAV password hashing never exceeds 256 MiB
  (4 concurrent × 64 MiB) regardless of request volume or number of source addresses.
- **SC-005**: A single source address can make at most 10 failed CardDAV authentication attempts per
  5 minutes before being shut out for 5 minutes, while a legitimate client from another address —
  or one holding a cached verdict — is never affected.
- **SC-006**: A single client can make at most 10 login-or-register attempts and 60 refresh attempts
  per minute; behind a configured trusted proxy those budgets are per end client rather than shared.
- **SC-007**: Revoking an app password or changing a password through the web UI stops CardDAV
  access on the very next request (0 seconds), rather than after the 5-minute cache lifetime.
- **SC-008**: A recovered password never appears in `ps`, `/proc/<pid>/cmdline`, shell history or
  `docker inspect`: the recovery command has no argument that could carry it.
- **SC-009**: An operator scripting recovery can distinguish "wrong email", "password too short",
  "database down" and "schema missing" from the exit code alone, without parsing text.
- **SC-010**: A signed-in user is not asked to re-authenticate for up to the refresh lifetime
  (7 days by default) despite a 1-hour access token, and a burst of simultaneous expired-token
  requests causes exactly one re-authentication round trip.
- **SC-011**: A server started with no signing secret, a placeholder secret, or one shorter than 32
  characters fails to start rather than serving forgeable tokens.

## Assumptions

- **Single-instance deployment.** Rate-limit buckets, the CardDAV verdict cache and the failure
  throttle are all process-local, so the stated limits hold per process. Running two instances
  behind a load balancer multiplies every budget and is not a supported configuration.
- **A self-hosted instance's first user is its owner.** This is what makes "the first account
  becomes the administrator" acceptable, including the unlikely race in which two simultaneous first
  registrations both succeed.
- **Transport security is the reverse proxy's job.** Basic authentication over `/dav` and bearer
  tokens both assume TLS terminates in front of the application; nothing in this domain enforces it
  (see `docs/reverse-proxy.md`).
- **`server.trusted_proxies` is configured when a proxy is in front.** Left empty behind a proxy,
  every client shares one rate-limit bucket and one CardDAV failure bucket — the limits are chosen
  to stay clear of normal use even then (`internal/handler/middleware/ratelimit.go:29-35`).
- **The token TTLs are the security boundary.** Because there is no revocation list, any incident
  response that must invalidate sessions is "rotate `CHQ_AUTH_JWT_SECRET` and restart".
- **Account recovery requires host access.** There is no email delivery anywhere in the codebase, so
  no self-service password reset is possible by design.

## Status

Shipped at `v0.4.0-3-g23a167c` (`23a167c`). Every requirement above except the four withdrawn
cross-reference slots describes behaviour present in that build; the gaps between requirement and
enforcement are listed under Known Divergences rather than left implied.

## Code Paths

This spec owns every decision about *who may act*: account creation and the bootstrap rule, sign-in
and token lifetime, password hashing, the role barrier, self-service profile and password
management, app passwords, credential verification at the CardDAV boundary, the two
credential-cost rate-limit tiers, and the user-visible behaviour of `set-password`.

It does **not** own CardDAV protocol semantics, resource routing, CTag or sync-collection (004 —
this spec owns only "is this request authenticated"); credentials used to reach outbound providers
including Google OAuth (006); the shared expensive-operation rate limiter, one bucket across six
routes (008); or the CLI dispatch contract as a class (008).

- `internal/carddav/authcache.go`
- `internal/carddav/authcache_test.go`
- `internal/carddav/authcache_invalidate_test.go`
- `internal/carddav/throttle.go`
- `internal/carddav/throttle_test.go`
- `internal/carddav/auth_throttle_http_test.go`
- `internal/carddav/app_password_loop_test.go`
- `internal/carddav/argon2_semaphore_test.go`
- `internal/domain/user.go`
- `internal/domain/app_password.go`
- `internal/handler/auth_handler.go`
- `internal/handler/user_handler.go`
- `internal/handler/admin_handler.go`
- `internal/handler/app_password_handler.go`
- `internal/handler/registration_policy_test.go`
- `internal/handler/middleware/auth.go`
- `internal/handler/middleware/admin.go`
- `internal/handler/middleware/ratelimit.go`
- `internal/handler/middleware/ratelimit_test.go`
- `internal/repository/bun_user.go`
- `internal/repository/bun_app_password.go`
- `internal/service/auth.go`
- `internal/service/auth_test.go`
- `internal/service/auth_registration_policy_test.go`
- `internal/service/user.go`
- `internal/service/user_set_password_test.go`
- `internal/service/app_password.go`
- `internal/service/app_password_limit_test.go`
- `migrations/017_app_passwords.up.sql`
- `migrations/017_app_passwords.down.sql`
- `web/src/api/client.ts`
- `web/src/api/client.spec.ts`
- `web/src/api/auth.ts`
- `web/src/api/users.ts`
- `web/src/api/admin.ts`
- `web/src/api/app-passwords.ts`
- `web/src/stores/auth.ts`
- `web/src/views/LoginView.vue`
- `web/src/views/admin/`
- `web/src/views/settings/ProfileView.vue`
- `web/src/views/settings/PasswordView.vue`
- `web/src/views/settings/AppPasswordsView.vue`

## References

Paths this spec touches but does not own:

- `internal/carddav/server.go` — holds `authenticate`, `verifyCredentials`, `verifyAppPassword` and
  `VerifyArgon2id` alongside spec 004's `ServeHTTP`; see Known Divergences.
- `cmd/server/cli.go`
- `cmd/server/cli_test.go`
- `cmd/server/main.go`
- `internal/handler/handler.go`
- `internal/config/config.go`
- `internal/repository/interfaces.go`
- `migrations/001_init.up.sql`
- `migrations/001_init.down.sql`
- `web/src/router/index.ts`

Neighbouring specs:

- **Spec 004 (CardDAV protocol)** consumes the authenticated identity this spec places on the
  request context (`WithUserID` / `WithUserEmail`, `internal/carddav/server.go:105-107`) and owns
  everything after that point.
- **Spec 006 (outbound provider credentials, Google OAuth)** owns `/api/v1/credentials`,
  `/api/v1/auth/google/*` and the provider connection vault. The only overlap is that the Google
  OAuth *callback* is registered inside the same public `/auth` group
  (`internal/handler/handler.go:93-97`).
- **Spec 008 (runtime configuration and delivery)** owns the shared expensive-operation limiter
  (`ExpensiveOpRateLimit`), the configuration surface, and the CLI dispatch contract as a class —
  including FR-059 (exit codes), FR-060 (no secret in argv), FR-061 (interleaved flag parsing) and
  FR-062 (a subcommand never migrates). This spec's withdrawn FR-045/046/048/049 point there.

## Enforced By

**Bootstrap and the registration policy**

- `TestRegister_FirstUserBecomesAdmin`, `TestRegister_SubsequentUsersAreNotAdmin`,
  `TestRegister_AdminRoleReachesTheAccessToken` (`internal/service/auth_test.go:194,206,224`) —
  FR-002.
- `TestRegister_FirstAccountIsAllowedWhenRegistrationIsClosed`,
  `TestRegister_SecondAccountIsRefusedWhenRegistrationIsClosed`,
  `TestRegister_SecondAccountIsAllowedWhenRegistrationIsOpen`,
  `TestRegisterBypassPolicy_WorksWhenRegistrationIsClosed`,
  `TestRegisterBypassPolicy_StillRejectsDuplicateEmail`, `TestRegistrationOpen`
  (`internal/service/auth_registration_policy_test.go:11,25,39,58,76,90`) — FR-001, FR-003, FR-005,
  FR-006.
- `TestRegistrationPolicy_ClosedAfterBootstrap`, `TestRegistrationPolicy_OpenWhenConfigured`,
  `TestRegistrationPolicy_ConfigIsPublic`, `TestRegistrationPolicy_AdminCanCreateUsersWhileClosed`,
  `TestRegistrationPolicy_NonAdminCannotUseAdminCreate`
  (`internal/handler/registration_policy_test.go:93,105,116,142,169`) — FR-003, FR-005, FR-006,
  FR-015, SC-001, SC-002.

**Tokens and the signing secret**

- `TestTokenPairCarriesTokenType`, `TestRefreshTokenRejectedAsAccessToken`,
  `TestAccessTokenCannotRefresh`, `TestRefreshTokenIssuesNewPair`,
  `TestValidateTokenRejectsForeignSecret` (`internal/service/auth_test.go:59,85,100,117,136`) —
  FR-008, FR-009, FR-010, FR-013.
- `TestDefaults_TokenLifetimes` (`internal/config/config_test.go:166`) — FR-011.
- `TestAuthConfigValidate`, `TestConfigValidateRejectsDefaultSecret`
  (`internal/config/config_test.go:10,46`) — FR-016, SC-011. *(File owned by spec 008.)*

**Passwords and self-service**

- `TestSetPassword_ReplacesTheHashAndKeepsEverythingElse`, `TestSetPassword_UnknownEmailIsAnError`,
  `TestSetPassword_RejectsAShortPassword`, `TestSetPassword_InvokesTheCredentialInvalidatorOnce`,
  `TestChangePassword_InvokesTheCredentialInvalidator`,
  `TestSetPassword_WithoutAnInvalidatorDoesNotPanic`
  (`internal/service/user_set_password_test.go:33,56,69,83,99,115`) — FR-023, FR-024, FR-040,
  FR-047, SC-007.
- `TestSetPassword_HashIsAcceptedByBothVerifiers`
  (`internal/service/user_set_password_test.go:128`) — FR-019. This single test is the only thing
  preventing the two argon2id verifiers from drifting apart.

**App passwords**

- `TestAppPassword_CreateIsCappedPerUser`, `TestAppPassword_DeletingFreesASlot`,
  `TestAppPassword_DeleteInvalidatesTheCredentialCache`,
  `TestAppPassword_FailedDeleteDoesNotInvalidate`, `TestAppPassword_CapIsPerUser`
  (`internal/service/app_password_limit_test.go:82,99,122,139,177`) — FR-028, FR-030, SC-007.

**Credential verification at the CardDAV boundary**

- `TestAuthCacheHitAndMiss`, `TestAuthCachePositiveExpires`, `TestAuthCacheNegativeExpiresSooner`,
  `TestAuthCacheKeyIsolatesPasswordAndUser`, `TestAuthCacheBounded`
  (`internal/carddav/authcache_test.go:14,31,51,68,81`) — FR-034, SC-003.
- `TestAuthCache_InvalidateEmailDropsOnlyThatAccount`,
  `TestAuthCache_InvalidateEmailIgnoresBlankAndUnknown`,
  `TestAuthCache_InvalidateEmailDoesNotMatchOnPrefixAlone`, `TestServer_InvalidateUser`
  (`internal/carddav/authcache_invalidate_test.go:5,26,41,60`) — FR-024, FR-030, FR-040.
- `TestAuthThrottle_BlocksAfterLimitAndReleasesLater`, `TestAuthThrottle_SuccessResetsCounter`,
  `TestAuthThrottle_WindowSlides`, `TestAuthThrottle_MapIsBounded`, `TestClientIP`
  (`internal/carddav/throttle_test.go:10,44,61,76,103`) — FR-036, FR-038.
- `TestDavAuth_ThrottleStopsHashingAfterTheLimit`, `TestDavAuth_ThrottleIsPerAddress`,
  `TestDavAuth_LegitimateClientIsNeverThrottled`, `TestDavAuth_TrustedProxySeparatesClients`,
  `TestDavAuth_UntrustedForwardedHeaderCannotMintBuckets`
  (`internal/carddav/auth_throttle_http_test.go:85,104,121,136,152`) — FR-035, FR-036, FR-038,
  FR-039, FR-044, SC-005.
- `TestVerifyCredentials_ConcurrencyIsCapped`,
  `TestVerifyCredentials_CancelledContextDoesNotTakeASlot`
  (`internal/carddav/argon2_semaphore_test.go:50,97`) — FR-037, SC-004.
- `TestVerifyAppPassword_AccountOverTheCapStillAuthenticates`,
  `TestVerifyAppPassword_MostRecentlyUsedIsTriedFirst`
  (`internal/carddav/app_password_loop_test.go:52,86`) — FR-033.
- `TestAuthRequired`, `TestWrongPasswordRejected`, `TestAppPasswordAuthenticates`
  (`internal/carddav/carddav_e2e_test.go:125,136,272`) — FR-031, FR-032, FR-033.
  *(File owned by spec 004.)*

**Credential-cost rate limiting**

- `TestRateLimiterBlocksAfterMax`, `TestRateLimiterInstanceSharesBucket`,
  `TestRateLimiterConstants`, `TestRateLimiter_KeysPerForwardedClientWhenProxyTrusted`,
  `TestRateLimiter_IgnoresForwardedHeaderFromUntrustedPeer`
  (`internal/handler/middleware/ratelimit_test.go:23,42,66,91,123`) — FR-041, FR-042, FR-043,
  FR-044, SC-006.

**Password recovery from the host**

- `TestSetPassword_RefusesAPasswordArgument`, `TestSetPassword_RequiresAnEmail`,
  `TestSetPassword_NonTerminalWithoutStdinFlagIsRefused`, `TestSetPassword_EmptyStdinIsRefused`
  (`cmd/server/cli_test.go:54,59,66,72`) — SC-008 here; the rule itself is 008 FR-060.
  *(File owned by spec 008.)*
- `TestSetPassword_WarningsAreSpelledOut` (`cmd/server/cli_test.go:104`) — FR-050.

**Web client**

- `web/src/api/client.spec.ts` — `attaches the stored access token` (:42),
  `refreshes once and replays the concurrent requests that hit 401` (:52),
  `clears the session and redirects when the refresh token is rejected` (:77),
  `redirects immediately when there is no refresh token at all` (:91),
  `replays once and gives up, rather than looping on an endpoint that always 401s` (:100) —
  FR-051, FR-052, SC-010.

## Known Divergences

1. **There is no token revocation list.** A stolen or leaked access token stays valid for its full
   TTL; the lifetime *is* the revocation window. Changing a password, deleting an app password, or
   even deleting the user does **not** invalidate an outstanding access token, because `JWTAuth`
   never touches the database (`internal/handler/middleware/auth.go:22-30`). The documented
   instance-wide remedy is rotating `CHQ_AUTH_JWT_SECRET` and restarting
   (`internal/config/config.go:177-180`, `cmd/server/cli.go:200-211`). A *refresh* does re-read the
   user, so a deleted account cannot mint a new pair (`internal/service/auth.go:190-196`).
2. **A demoted administrator keeps administrator rights until their access token expires.**
   `AdminOnly` reads the `role` claim, not the stored row
   (`internal/handler/middleware/admin.go:9`), which is the FR-015 barrier operating on stale data.
3. **Changing an email does not invalidate cached CardDAV verdicts.** `UpdateProfile` never calls
   the credential invalidator (`internal/service/user.go:64-94`), while the cache is keyed on
   `email + SHA-256(password)` (`internal/carddav/authcache.go:46-49`). The *old* address plus the
   unchanged password therefore keeps opening `/dav` for up to five minutes. Password change and
   app-password deletion both invalidate (FR-024, FR-030); this path is the inconsistency, and
   whether it should be closed is an open question.
4. **`set-password` cannot invalidate a running server's cache**, because it is a separate process
   holding its own state. The command says so; restarting the server is the stated remedy
   (`cmd/server/cli.go:206-210`).
5. **The `set-password` epilogue quotes stale TTL defaults** — "default 24h" for access and "720h"
   for refresh (`cmd/server/cli.go:204-206`) — while the shipped defaults have been `1h` and `168h`
   since v0.4.0 (`internal/config/config.go:181-182`, `CHANGELOG.md` 0.4.0 "Breaking"). FR-050 is
   satisfied in substance and wrong in its numbers, which are what an operator will act on.
6. **Nothing prevents an instance from losing its last administrator.** `PUT /admin/users/:id/role`
   and `DELETE /admin/users/:id` have no "last admin" guard and no self-protection, and
   `set-password` deliberately never touches roles (`internal/handler/admin_handler.go:39-66`,
   `internal/service/user.go:124-128`). No CLI subcommand can grant the admin role back; recovery
   would mean editing the database. Whether this should be preventable is an open question.
7. **Two registrations racing for the very first account could both come out as administrators.**
   The count and the insert are not one transaction, so FR-002 holds only under the assumption that
   a self-hosted instance's first user is its owner; the code documents this as acceptable
   (`internal/service/auth.go:160-166`).
8. **Login leaks account existence by timing.** An unknown email returns before any hashing
   (`internal/service/auth.go:173-175`) while a known email pays a 64 MiB argon2id hash. FR-008's
   identical response bodies are real; the latency is not identical, and nothing tests or enforces
   that it should be.
9. **The API login path has no argon2 concurrency cap.** FR-037's 4-slot semaphore protects `/dav`
   only (`internal/carddav/server.go:58`); `/auth/login` and `/auth/register` are bounded solely by
   the 10-per-minute rate limit, so the memory ceiling on that path is 10 × 64 MiB per limiter
   window per client address. SC-004 is therefore a statement about CardDAV, not about the process.
10. **Rate-limit and throttle state is per process and in memory.** The Fiber limiter's counters and
    the CardDAV failure map live in the process (`internal/handler/middleware/ratelimit.go:37-49`,
    `internal/carddav/throttle.go:40-51`); a restart clears them and two instances do not share
    them. The verdict cache (1024 entries) and the failure map (1024 addresses) both drop
    *everything* when full rather than growing (`internal/carddav/authcache.go:76-83`,
    `internal/carddav/throttle.go:98-105`), so a flood of distinct keys can evict a legitimate
    client's cached verdict and reset an attacker's counter in the same moment.
11. **`GET /auth/config` is unauthenticated and unmetered.** It is registered before the JWT barrier
    and carries no rate limiter (`internal/handler/handler.go:91`), and each call runs a user count
    query when registration is closed (`internal/service/auth.go:90-99`).
12. **The SPA never calls `/auth/config` and has no sign-up form.** `web/src/views/LoginView.vue`
    offers only email and password, and no file under `web/src/` references `auth/config` or
    `registration_open`. FR-006 is implemented and tested, but the behaviour it enables — hiding a
    form that would `403` — is not observable in the shipped product; account creation happens
    through the API or the admin screens. Whether the endpoint should stay is an open question.
13. **Email addresses are not validated for shape.** Registration checks only that the field is
    non-empty and the password is at least 8 characters
    (`internal/handler/auth_handler.go:55-61`); no format, MX or confirmation check exists anywhere.
14. **Password strength is a length check only** — 8 characters, shared between the HTTP handler and
    the CLI through `service.MinPasswordLen` (`internal/service/user.go:13-15`). No breach list, no
    composition rules, no re-use check.
15. **There are two independent argon2id verifiers**, because `internal/service` cannot import
    `internal/carddav`. They parse the encoded hash differently — the CardDAV one derives the key
    length from the stored hash, the service one uses a constant — and a hash accepted by only one
    would let a user log in but not sync. FR-019 is held together by exactly one test,
    `TestSetPassword_HashIsAcceptedByBothVerifiers`
    (`internal/carddav/server.go:251-259`, `internal/service/user_set_password_test.go:128`).
16. **`DELETE /api/v1/users/me` has no confirmation step and no UI.** It deletes on the first call
    (`internal/handler/user_handler.go:88-96`), so FR-025 ships as an API-only capability that no
    shipped screen exercises.
17. **Several requirements have no enforcer at all.** Stated louder than the rest, per Constitution
    Principle VI: FR-021, FR-022 and FR-025 (profile read, the `409` on a taken email, self-delete
    and its cascade) have no test in `internal/service` or `internal/handler`; FR-026 and FR-029
    (app-password handler response shapes) are covered only at the service layer; FR-007 (the
    "Contacts" address book created on registration) is exercised through a mock that counts calls
    but no test asserts the count (`internal/service/auth_test.go:153-170`); FR-017 and FR-018 have
    no test that pins the argon2 parameters or the constant-time comparison, only the cross-verifier
    test of FR-019; FR-014 has no direct test of `JWTAuth` — it is exercised incidentally through
    the registration-policy suite; FR-053 and FR-054 have no component tests, only
    `web/src/router/routes.spec.ts` asserting that a route exists for every view file.
18. **The credential-verification code lives in a file this spec does not own.** `authenticate`,
    `verifyCredentials`, `verifyAppPassword` and `VerifyArgon2id` are at
    `internal/carddav/server.go:153-328`, alongside `ServeHTTP` and `serveSyncExtensions`, which
    belong to spec 004. Ownership is therefore split by function rather than by file, and
    `internal/carddav/server.go` is listed under References here and owned by 004. Splitting the
    file into `internal/carddav/auth.go` would make the ownership map match the code; until then,
    every citation above points at the file as it actually is.

## Amendments

| Date | Tag | Change | Issue/PR |
|------|-----|--------|----------|
| 2026-08-07 | v0.4.0-3-g23a167c | Initial spec, reconstructed from the implementation at `23a167c`. | — |
| 2026-08-07 | v0.4.0-3-g23a167c | Conformed to the house template; withdrew FR-045, FR-046, FR-048 and FR-049 in favour of spec 008 (FR-060, FR-061, FR-059, FR-062) and left their numbers as labelled cross-references; merged the trusted-proxy trust rule into a single statement at FR-044; folded Scope Note into Code Paths, Dependencies into References and Open Questions into Known Divergences. | — |

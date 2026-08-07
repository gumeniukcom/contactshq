# Deliberately unowned paths

Constitution Principle VII requires that exactly one spec own each path. This file is the
other half of that rule: the short, argued list of paths that no spec owns **on purpose**.

An entry here is a decision, not a backlog item. Anything missing from both the specs'
`## Code Paths` sections and this file is a gap, and the coverage gate should say so.

Keep this list small. A long one means the specs stopped describing the system.

| Path | Why no spec owns it |
|---|---|
| `internal/service/testutil_test.go` | Package-scoped test doubles (`mockContactRepo`, `mockAddressBookRepo` and friends) shared by the auth tests (001), contact tests (002), backup and restore tests (005), sync tests (006) and duplicate/merge tests (007). No spec's requirements become false if it changes — all five suites simply stop compiling. Assigning it to whichever domain happens to hold the most mocks would be arbitrary and would make that spec's ownership claim mean something different from every other claim in the tree. |
| `internal/web/static/spa/**` except `.gitkeep` | Vite build output, gitignored and untracked. Produced by `make build` from `web/src`, which is owned file by file, so ownership of the source is ownership of these. `.gitkeep` itself **is** owned (008) because `go:embed all:static/spa` fails to compile without it. |

## Not on this list

Two categories are often mistaken for exemptions and are not:

- **`.down.sql` migration halves.** Every one is claimed, as the second line of the pair whose
  `.up.sql` its owning spec claims. No code applies them (`MigrateFS` globs `*.up.sql` only), but
  claiming them keeps a future spec from adopting the down half of a migration another spec
  introduced.
- **Paths a spec claims but that do not exist yet** (`internal/speckit/`, `specs/README.md`).
  Those are forward claims recorded in `000-speckit-adoption`, and they are listed there under
  "not built at all" rather than hidden here.

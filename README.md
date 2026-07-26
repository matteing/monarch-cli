<h1 align="center">monarch-cli</h1>

<p align="center">
  <strong>A joyful CLI and MCP for Monarch Money</strong>
</p>

> [!IMPORTANT]
> I built this for myself! It is not affiliated with or endorsed by Monarch.

## Why?

Monarch recently shut down their MCP solution. Other solutions are written in interpreted languages. I wanted a self-contained, single binary solution. And I like learning and building things.

## What it does

- Lists accounts, transactions, categories, and budgets.
- Fetches transaction details, cashflow summaries, and a financial overview.
- Provides responsive terminal tables and a keyboard-driven transaction pager.
- Emits machine-readable JSON for scripts and pipes.
- Serves the same read-only operations as MCP tools over stdio.
- Stores sessions in the operating system keyring—never a plaintext config file.

Write operations and arbitrary GraphQL are deliberately out of scope.

## Install

### Tell an agent

Give your coding agent the following instructions:

> Install Monarch CLI from `github.com/matteing/monarch-cli`. Inspect the
> repository before running anything, use mise to download the pinned Go
> toolchain, run the full audit, build and install the CLI, then configure its
> local stdio MCP server. Discover the available commands with `monarch --help`
> and the relevant subcommand help, inspect the MCP tools, and write your own
> local skill for using the read-only interface safely. Do not ask me for
> credentials or place them in commands—stop and ask me to run
> `monarch auth login` in a real terminal. Finish by running `monarch doctor`
> without printing my financial data.

### Install manually

Install [mise](https://mise.jdx.dev/), review the source checkout, then build it
with the pinned Go toolchain:

```sh
git clone https://github.com/matteing/monarch-cli.git
cd monarch-cli
mise install
mise run audit
mise run install
# Ensure the printed install directory is on PATH, then:
monarch --help
```

The repository pins the supported Go toolchain in `mise.toml`; a separate Go
installation is not required. On Unix-like systems, `mise run install` writes
to `~/.local/bin` by default. On Windows it uses
`%LOCALAPPDATA%\monarch-cli\bin`. Set `MONARCH_INSTALL_DIR` to choose another
user-owned directory. The selected directory must be on `PATH`: add
`$HOME/.local/bin` to your shell profile on Unix-like systems, or add the
Windows directory to your user `PATH`, when it is not already present.

Release builds report their tag through `monarch --version`. Source builds
report `dev-<commit>` and append `-dirty` for local changes (or `dev-dirty`
before a checkout has its first commit), so update verification is meaningful.

## Sign in

```sh
monarch auth login
monarch auth status
monarch doctor
```

Your password and MFA code are not stored anywhere. Monarch CLI keeps only the
resulting session credential in your OS keyring. Cancellation is honored until
the keyring commit begins. Native keyring writes cannot be canceled once they
start, so the CLI waits for that commit and reports its result.

If password login is blocked by CAPTCHA, you can import an existing browser
session instead:

```sh
monarch auth login --method browser-session
```

Copy the `Cookie` request header from an authenticated request in a signed-in
`app.monarch.com` tab. The prompt is hidden, and only the cookies required for
authentication are retained in the OS keyring. There is no plaintext fallback.
Saved records are strict and minimal; if an older record contains extra fields
or cookies, replace it explicitly with `monarch auth login --force`.

Profiles are just named keyring slots. Ignore them if you use one Monarch
account. Names are 1–64 characters, start with a letter or digit, and otherwise
use only letters, digits, dots, underscores, or hyphens. If you use more than
one account or household:

```sh
monarch --profile household auth login
monarch --profile household accounts list
monarch --profile household auth logout
```

## Use the CLI

```text
monarch accounts list
monarch transactions list [filters]
monarch transactions get TRANSACTION_ID
monarch categories list
monarch budgets get
monarch cashflow summary
monarch overview
```

Human-readable tables are the default. In a terminal, `transactions list`
opens a responsive pager and groups transactions by month:

- `up` / `down` scroll the current page.
- `left` / `right` load the previous or next API page.
- `q`, `esc`, or `ctrl+c` close the pager.

Visited pages are cached. Use `--group none` for one continuous table,
`--limit` to set a page size, or `--cursor` to continue from an earlier result.

For automation, use JSON:

```sh
monarch transactions list \
  --start-date 2026-07-01 \
  --end-date 2026-07-31 \
  --search coffee \
  --output json
```

JSON and piped table output return one bounded page. The JSON response includes
an opaque `next_cursor` when another page is available. Reuse a cursor only with
the same normalized filters and ordering that produced it. Amounts are exact
decimal strings rather than JSON numbers.

## Use the MCP server

Sign in once from a real terminal, then configure your MCP host with the
absolute path to the binary:

```json
{
  "mcpServers": {
    "monarch": {
      "command": "/absolute/path/to/monarch",
      "args": ["--profile", "default", "mcp"]
    }
  }
}
```

The stdio server exposes:

```text
monarch_accounts_list
monarch_transactions_list
monarch_transaction_get
monarch_categories_list
monarch_budgets_get
monarch_cashflow_summary
monarch_financial_overview
```

Each tool has explicit input/output JSON Schema and read-only annotations. MCP
stdout is reserved for JSON-RPC; diagnostics go to stderr. Stdio input is
bounded to 512 KiB per newline-delimited message and 16 MiB per server process;
clients with unusually long sessions should reconnect before the session
budget is exhausted.

## Configuration

Optional non-secret configuration lives at
`$XDG_CONFIG_HOME/monarch-cli/config.json` on Unix-like systems, or the
platform-equivalent user config directory:

```json
{
  "profile": "default",
  "output": "table",
  "timeout": "30s",
  "log_level": "info",
  "log_format": "text"
}
```

Environment overrides are `MONARCH_PROFILE`, `MONARCH_OUTPUT`,
`MONARCH_TIMEOUT`, `MONARCH_LOG_LEVEL`, and `MONARCH_LOG_FORMAT`. Flags take
final precedence. None of these settings accepts credentials.

Stable exit codes are `2` for invalid input or a missing resource, `3` for
authentication, `4` for a rate limit, `5` for upstream unavailability, and `6`
for a keyring failure. Canceled commands use `130`; unexpected errors use `1`.

## Develop

mise manages the Go version and every project task:

```sh
mise install
mise run                 # list tasks
mise run fmt             # format source
mise run check           # format, module, test, vet, staticcheck, and build gate
mise run audit           # full gate: check, race, shuffle, coverage, vulnerabilities
mise run test:coverage   # enforce the 65% statement coverage floor
mise run test:race       # race-enabled tests
mise run vuln            # scan reachable code with pinned govulncheck
mise run release:check   # validate release configuration
mise run release:snapshot # cross-build and package release artifacts locally
```

An opt-in live smoke test starts the built MCP server and calls all seven tools
without printing your financial data:

```sh
mise run test:live
```

It requires a valid session in the selected profile. The normal test suite is
offline: it does not touch a real Monarch account or keyring.

Tagged releases are built for macOS, Linux, and Windows on `amd64` and `arm64`,
with checksums. Push a tag such as `v0.1.0` to start the release workflow.

## License

[MIT](LICENSE). Unrelated to my job btw <3

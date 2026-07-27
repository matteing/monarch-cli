<h1 align="center">monarch-cli</h1>

<p align="center">
  <strong>💖 A joyful CLI and MCP for Monarch Money 🌟</strong>
</p>

> [!IMPORTANT]
> I built this for myself! It is not affiliated with or endorsed by Monarch.

## Why?

Monarch recently shut down their MCP solution. Other solutions are written in interpreted languages. I wanted a self-contained, single binary solution. And I like learning and building things.

## Features

- Lists accounts, transactions, categories, and budgets, and lets MCP clients request account refreshes.
- Fetches transaction details, cashflow summaries, and a financial overview.
- Nice TUI for browsing your transactions.
- Structured output mode for all your agents <3
- All features are available as MCP tools over stdio.
- Stores sessions in the operating system keychain for safety.

Account refresh is the only write operation and is available through MCP.

## Install

Tell Codex or another agent to set this up:

> Install Monarch CLI from `github.com/matteing/monarch-cli`. Review the source,
> download the archive matching this system from the latest stable GitHub
> release, verify it against `checksums.txt`, and install the `monarch` binary on
> `PATH`.
>
> Then configure stdio MCP, discover commands
> with `monarch --help`, and inspect the MCP tools.
>
> Create a local skill named
> **Monarch CLI** for general usage.
> Never handle my credentials; ask me to run `monarch auth login`.

### Build from source

Requires Go 1.25 or newer:

```sh
git clone https://github.com/matteing/monarch-cli.git
cd monarch-cli
go test ./...
go vet ./...
go install ./cmd/monarch
monarch --help
```

## Sign in

```sh
monarch auth login
```

Your password and MFA code are not stored anywhere. Monarch CLI keeps only the
resulting session credential in your OS keyring.

### Multiple accounts

We support profiles, which are just different keyring slots. Names are 1–64 characters, start with a letter or digit, and otherwise
use only letters, digits, dots, underscores, or hyphens. If you use more than
one account or household:

```sh
monarch --profile household auth login
monarch --profile household accounts list
monarch --profile household auth logout
```

## Use the CLI

```text
# some sample commands...
monarch accounts list
monarch transactions list [filters]
monarch transactions get TRANSACTION_ID
monarch categories list
monarch budgets get
monarch cashflow summary
monarch overview
```

Human-readable tables are the default.

For agents, use JSON:

```sh
monarch transactions list \
  --start-date 2026-07-01 \
  --end-date 2026-07-31 \
  --search coffee \
  --output json
```

- JSON and piped table output return one bounded page.
- The JSON response includes
  an opaque `next_cursor` when another page is available.
- Reuse a cursor only with
  the same normalized filters and ordering that produced it.
- Amounts are exact
  decimal strings rather than JSON numbers.

## Use the MCP server

**You must login using a terminal app before setting this up**.

After signing in, in your `mcp.json`:

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

Afterwards, ask it what it can do! We expose _just about everything_ that the CLI can do.

## Configuration

Some non-secret config lives at
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

### Contracts

Stable exit codes are `2` for invalid input or a missing resource, `3` for
authentication, `4` for a rate limit, `5` for upstream unavailability, and `6`
for a keyring failure. Canceled commands use `130`; unexpected errors use `1`.

## Develop

Development uses [mise](https://mise.jdx.dev/) to pin tool versions and provide
task aliases. It is a contributor convenience, not an installation requirement:

```sh
mise install
mise run                 # list tasks
mise run fmt             # format source
mise run check           # format, module, test, vet, staticcheck, and build gate
mise run audit           # full gate: check, race, shuffle, coverage, vulnerabilities
mise run test:coverage   # enforce the statement coverage floor
mise run test:race       # race-enabled tests
mise run vuln            # scan reachable code with pinned govulncheck
mise run release:check   # validate release configuration
mise run release:snapshot # cross-build and package release artifacts locally
```

An opt-in live smoke test starts the built MCP server and calls the seven read tools
without printing your financial data:

```sh
mise run test:live
```

It requires a valid session in the selected profile.

Tagged releases are built for macOS, Linux, and Windows on `amd64` and `arm64`,
with checksums. Push a tag such as `v0.1.0` to start the release workflow.

## License

[MIT](LICENSE). Unrelated to my job btw <3

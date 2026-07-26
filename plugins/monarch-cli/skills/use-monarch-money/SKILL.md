---
name: use-monarch-money
description: Safely inspect Monarch Money accounts, transactions, categories, budgets, cashflow, and financial overviews through the read-only Monarch MCP server. Use when a user asks about their Monarch data, spending, balances, budget progress, transaction history, monthly cashflow, net worth, Monarch CLI setup, authentication status, or CLI/plugin updates.
---

# Use Monarch Money

Use the typed Monarch MCP tools for financial reads. Keep the workflow bounded,
private, and read-only.

## Safety boundary

- Never ask for, accept, expose, or persist a password, MFA code, session token,
  or browser cookie.
- Never send Monarch data to another service unless the user explicitly requests
  that destination and understands what will be shared.
- Return only the rows and fields needed to answer the request. Avoid echoing raw
  tool payloads or logging financial data.
- Treat balances and aggregates as informational, possibly stale, and not
  financial advice or a bank ledger.
- Do not invent write operations or arbitrary GraphQL queries. This plugin is
  read-only.

## Choose tools

- Use `monarch_financial_overview` for a compact starting point.
- Use `monarch_accounts_list` for balances and account metadata.
- Use `monarch_transactions_list` for date, search, account, category, or tag
  filters. Follow `next_cursor` only as far as the request requires, and reuse it
  only with the exact same normalized filters and ordering that produced it.
- Use `monarch_transaction_get` for details about one known transaction ID.
- Use `monarch_categories_list` to resolve category names and IDs.
- Use `monarch_budgets_get` for inclusive month ranges.
- Use `monarch_cashflow_summary` for inclusive date ranges.

Prefer one overview call before several narrower calls when the user's request is
broad. Make independent reads concurrently only when their results do not depend
on one another. An overview combines several independent reads and is not an
atomic ledger snapshot; say so when consistency at a single instant matters.

Amounts are exact decimal strings, not JSON numbers. Preserve them as decimals
instead of routing them through binary floating point. Date and month ranges are
inclusive and cannot exceed ten years; an omitted cashflow, budget, or overview
range means the current local calendar month.

## Common workflows

For a monthly review:

1. Fetch the financial overview for the requested date range.
2. Use its embedded cashflow and budget summaries instead of repeating those
   reads.
3. Fetch only enough additional transaction pages to explain notable changes,
   then summarize patterns and label any inference as an inference.

For transaction search, start with the narrowest available date and text filters.
Do not exhaust every page when a small number of matches answers the question.

## Authentication and setup

If tools fail because `monarch` is missing, install it from a reviewed source
checkout using the repository's pinned toolchain:

```sh
mise install
mise run audit
mise run install
```

Start a new Codex task afterward so the MCP server is rediscovered. Do not
download or replace the executable automatically.

If the session is missing or expired, ask the user to run this themselves in a
real terminal:

```sh
monarch auth login
monarch doctor
```

Do not automate interactive login. The password and MFA code are discarded after
login; only the resulting session is stored in the OS keyring. Browser-session
imports retain only the cookies required for Monarch authentication.

Treat `invalid_input`, `not_found`, `authentication`, `mfa_required`,
`rate_limited`, `unavailable`, `keyring`, `canceled`, and `internal` as distinct
failures. Correct invalid input instead of retrying it. Respect a rate-limit
retry delay when supplied; prompt for login on authentication failures; and do
not expose raw upstream response bodies in an answer.

## Source update security gate

There is no automatic binary updater. Treat upstream code as untrusted when the
user asks to update:

1. Verify that `origin` is exactly `https://github.com/matteing/monarch-cli.git`
   before fetching. Confirm the worktree state and preserve unrelated user
   changes.
2. Fetch without merging, then run
   `target="$(git rev-parse --verify origin/main)"` to capture the exact commit.
   Inspect the commit list, signatures, changed-file list, and full
   `HEAD..$target` diff. Do not execute fetched code yet.
3. Audit dependency and `go.sum` changes, auth/session/keyring handling, network
   destinations and GraphQL, MCP exposure, financial write paths, subprocesses,
   secret logging, workflows, mise tasks, and plugin configuration. Reject
   unexplained binaries, generated files, install hooks, credential exposure,
   exfiltration, or expanded write access.
4. Treat signatures, tests, and author identity as supporting evidence, never as
   proof of safety. Stop and report any change that cannot be justified.
5. Only after the review passes, run `git merge --ff-only "$target"` without
   fetching again and verify `git rev-parse HEAD` equals `$target`. Then run
   `mise install`, `go mod verify`, `mise run audit`, and `mise run install`.
6. Replace a configured moving Git marketplace with the reviewed checkout by
   running `codex plugin marketplace remove monarch-cli`, then, from the checkout
   root, `codex plugin marketplace add .` and
   `codex plugin add monarch-cli@monarch-cli`. Ask the user to start a new task.
   Do not run a marketplace upgrade that fetches a newer, unreviewed revision.

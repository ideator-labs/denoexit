# AGENTS.md

Handoff for a coding agent that starts with no context.

## What this is

denoexit is a Go CLI for teams leaving Deno Deploy for Cloudflare Workers. It runs one
Deno app two ways and compares them:

1. `deno run` on the original entry file.
2. `deno bundle` of the same entry, loaded into the real `workerd` binary behind a
   small shim that maps `Deno.serve` to the Worker `fetch` handler and `Deno.env` to
   text bindings.

It replays a list of HTTP requests against both and reports, per request, whether the
status and body match. When workerd fails, it reports the JavaScript error workerd logged.
It also lists every `Deno.*` call found in the source.

The claim: an app that passes under Deno can fail on workerd at boot or on specific
requests, and denoexit shows which ones and why before you cut over.

## Status

Real:
- The workerd binary (npm `workerd` 1.20261024.1, reports `workerd 2026-10-24`), pinned in `package.json`.
- Deno 2.9.8 for `deno run` and `deno bundle` (tested version, not pinned by the repo).
- Every error string in the reports comes from workerd's own log. Nothing is simulated.

Not built:
- No `wrangler dev` or Miniflare. No KV, D1, Durable Objects, cron or bindings beyond text vars.
- No cloud account and no deploy. Everything runs on 127.0.0.1.
- No model calls.

`bash demo.sh` passes 5/5 on a fresh clone. `bash demo.sh --break` fails checks 2, 3 and 4.

## Data shape

Input, `denoexit.json` in the app directory:

```json
{ "entry": "main.ts",
  "env": { "NAME": "value" },
  "requests": [{ "name": "GET /x", "method": "GET", "path": "/x", "body": "", "headers": {} }] }
```

Output, the `Report` struct in `main.go`, also written by `--json`:

- `apis[]`: `{api, where, shimmed, hint}` from the regex scan.
- `deno_boot`, `workerd_boot`: `{ok, error}`. A boot fails when the process exits before it listens.
- `results[]`: `{name, deno:{status, body, error}, workerd:{status, body, error}, same}`.
- `ok`: both booted and every result is `same`. Exit code 0 when ok, 1 when not, 2 on usage or setup errors.

## Repo map

- `main.go`. Types, flag parsing, loading `denoexit.json`, exit codes.
- `scan.go`. Regex scan for `Deno.<name>` and the table of shimmed and missing APIs with hints.
- `run.go`. Starts deno and workerd on free ports, writes the capnp config, replays requests, pulls errors from logs.
- `report.go`. Text report.
- `internal/shim/`. `shim.js` (the fake `Deno` global) and `entry.js` (the Worker entry), embedded with `go:embed`.
- `examples/before`, `examples/halfway`, `examples/after`. One app in three migration states, same `denoexit.json`.
- `demo.sh`. Builds, runs the three examples, runs the 5 checks with `jq`.
- `package.json`, `package-lock.json`. Pin the workerd npm package.

## Setup, run, verify (fresh clone)

```bash
git clone https://github.com/ideator-labs/denoexit && cd denoexit
# prerequisites: go 1.24+, deno 2.x, node + npm, jq
npm ci --ignore-scripts      # demo.sh also does this if node_modules is missing
bash demo.sh                 # expect "5 passed, 0 failed", exit 0, about 10 s
bash demo.sh --break         # expect checks 2, 3, 4 FAIL, exit 1
bash demo.sh                 # passes again
```

`--ignore-scripts` is fine. The binary comes from the `@cloudflare/workerd-linux-64`
optional dependency, not the postinstall script.

## Design decisions

- Plain workerd, not wrangler. It is the same runtime Cloudflare runs, with no login and
  no extra layers. The cost is that wrangler-only behavior (bindings, `nodejs_compat`
  defaults) is not covered.
- The shim passes only the request to the handler. Real Workers have no `info`
  argument, so hiding that would make the check lie.
- Compare status and body only. Headers such as `date` and `content-length` differ
  between runtimes for harmless reasons.
- The Go binary resolves the platform workerd binary under `node_modules/@cloudflare/`.
  The `.bin/workerd` file is a Node wrapper. Killing it left the real binary running and
  hung the tool, so children run in their own process group and are killed as a group.
- Uppercase `Deno.X` names that are not in the table are treated as types and skipped.

## Known limits

- The scan is a regex. It misses aliased access such as `const { env } = Deno`.
- Requests run one at a time. No concurrency or isolate-reuse checks.
- Error attribution reads the log written within 150 ms of each response.
- `import.meta.url` is `undefined` for these modules in workerd (measured), so
  `new URL("./file", import.meta.url)` fails with `Invalid URL string` before any file read.
  A wrangler build may name modules differently.
- `new Function` at module scope worked in this workerd build. Inside a handler it fails
  with `EvalError: Code generation from strings disallowed for this context` (measured).
- Linux x64 is the only tested platform.

## Next steps

1. Run under `wrangler dev` as a second target. Done when `--target wrangler` replays
   `examples/after` 7/7 and `examples/halfway` shows the same 4 DIFFs.
2. Shim `Deno.openKv` on a local Durable Object. Done when a new `examples/kv` app with
   `get`, `set` and `atomic().check()` matches 100% on both runtimes.
3. Map `Deno.cron` to a `scheduled` handler and trigger it once. Done when a cron example
   reports the same side effect on both runtimes.
4. Replace the regex scan with a TypeScript AST pass (`deno` or `ts-morph`). Done when
   `const { env } = Deno; env.get("X")` is reported as `Deno.env`.
5. Record requests from a running Deploy app (access log to `denoexit.json`). Done when a
   converter turns a sample log into a spec that `demo.sh` can run.
6. Package as a GitHub Action. Done when a workflow in this repo runs denoexit on
   `examples/after` and fails on `examples/halfway`.

## Working rules

- `bash demo.sh` is the source of truth. Keep it green and keep `--break` failing.
- Every error string in docs must come from a real run. Do not invent workerd output.
- Keep Go stdlib only. Short sentences in docs, no long dashes.
- Commit as `danielgerlag <daniel@gerlag.ca>`.

## Context and sources

- Deno is joining Cloudflare. Deno Deploy runs six more months, then shuts down (Oct 9, 2026): https://deno.com/blog/cloudflare
- Cloudflare's side, workerd and celld merge: https://blog.cloudflare.com/deno-joins-cloudflare/
- A real 100k-line migration and what broke on workerd (global-scope random values, eval, 128 MB limit, CDN-Loop): https://uriv.me/blog/migrating-from-deno-deploy-to-cloudflare-workers
- A typical hand-port tutorial (serve to fetch, KV, cron): https://foundermag.co/tutorials/deno-deploy-migrate-to-cloudflare-workers/
- workerd: https://github.com/cloudflare/workerd
- Handlers and the global-scope rule: https://developers.cloudflare.com/workers/runtime-apis/handlers/

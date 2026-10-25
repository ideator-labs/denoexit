# denoexit

Deno Deploy shuts down in about six months. Most teams are moving to Cloudflare Workers.
Your app passes its tests under `deno run`. That does not mean it boots on workerd.

denoexit runs your Deno Deploy app twice. Once under `deno run`, once bundled on the real
`workerd` binary with a small `Deno.serve` and `Deno.env` shim. It replays the same
requests against both and shows every request that answers differently, with the
workerd error that caused it. It also lists every `Deno.*` call in your source.

```
bash demo.sh          # 5/5 PASS expected
bash demo.sh --break  # second runtime becomes deno, checks 2-4 fail
```

You need Go 1.24+, Deno 2.x (tested 2.9.8), Node/npm (for the pinned `workerd` npm
package) and `jq`.

## What the demo shows

Three versions of one small Deploy-style app in `examples/`:

- `before` is a normal Deno Deploy app. workerd refuses to boot it, because it calls
  `crypto.randomUUID()` at module scope.
- `halfway` fixes that. workerd boots it, but 4 of 7 requests fail: `info.remoteAddr`
  (the handler gets no second argument), `new Function` inside a handler,
  and reading a file next to the module.
- `after` is ported. 7 of 7 requests match.

The static scan finds 1 missing API in `halfway`. The replay finds 4 broken requests.
That gap is the point of the tool.

## Run it on your app

Add a `denoexit.json` next to your entry file:

```json
{
  "entry": "main.ts",
  "env": { "GREETING": "hello" },
  "requests": [{ "name": "GET /hello", "path": "/hello" }]
}
```

Then `go build -o bin/denoexit . && ./bin/denoexit path/to/app`. Exit 0 means every
request matched. Exit 1 means workerd did not boot or a request differed.

## Limits

This is a proof of concept. It only shims `Deno.serve` and `Deno.env`. No KV, cron,
Durable Objects or bindings beyond text vars. It uses plain workerd, not `wrangler dev`.
Static scanning is a regex. See [AGENTS.md](AGENTS.md) for status, design and next steps.

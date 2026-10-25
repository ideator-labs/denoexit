import { env } from "cloudflare:workers";

let handler = null;

function text(k) {
  return typeof env[k] === "string" ? env[k] : undefined;
}

globalThis.Deno = {
  serve(a, b) {
    if (typeof a === "function") handler = a;
    else if (typeof b === "function") handler = b;
    else if (a && typeof a.handler === "function") handler = a.handler;
    return { finished: new Promise(() => {}), shutdown: async () => {}, ref() {}, unref() {} };
  },
  env: {
    get: text,
    has: (k) => text(k) !== undefined,
    toObject: () => Object.fromEntries(Object.keys(env).filter((k) => text(k) !== undefined).map((k) => [k, env[k]])),
  },
};

export function getHandler() {
  return handler;
}

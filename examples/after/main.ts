// Ported: runs the same under Deno and workerd.
import { ABOUT } from "./about.ts";

let instanceId: string | undefined;
const emailPattern = /^[^@\s]+@[^@\s]+\.[a-z]+$/;

Deno.serve(async (req, info?: Deno.ServeHandlerInfo) => {
  instanceId ??= crypto.randomUUID();
  const url = new URL(req.url);
  switch (url.pathname) {
    case "/hello":
      return Response.json({ message: Deno.env.get("GREETING") ?? "unset", instance: instanceId.length === 36 });
    case "/whoami": {
      const ip = req.headers.get("cf-connecting-ip") ?? req.headers.get("x-forwarded-for") ?? info?.remoteAddr.hostname;
      return Response.json({ ip });
    }
    case "/validate":
      return Response.json({ valid: emailPattern.test(url.searchParams.get("email") ?? "") });
    case "/about":
      return new Response(ABOUT);
    case "/echo": {
      const { n } = await req.json();
      return Response.json({ n: n + 1 });
    }
    default:
      return new Response("not found", { status: 404 });
  }
});

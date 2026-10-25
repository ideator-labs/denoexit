// A typical Deno Deploy app. It runs fine under `deno run`.
const instanceId = crypto.randomUUID();
const emailPattern = "^[^@\\s]+@[^@\\s]+\\.[a-z]+$";

Deno.serve(async (req, info) => {
  const url = new URL(req.url);
  switch (url.pathname) {
    case "/hello":
      return Response.json({ message: Deno.env.get("GREETING") ?? "unset", instance: instanceId.length === 36 });
    case "/whoami":
      return Response.json({ ip: info.remoteAddr.hostname });
    case "/validate": {
      const test = new Function("s", `return new RegExp(${JSON.stringify(emailPattern)}).test(s)`);
      return Response.json({ valid: test(url.searchParams.get("email") ?? "") });
    }
    case "/about":
      return new Response(Deno.readTextFileSync(new URL("./about.txt", import.meta.url)));
    case "/echo": {
      const { n } = await req.json();
      return Response.json({ n: n + 1 });
    }
    default:
      return new Response("not found", { status: 404 });
  }
});

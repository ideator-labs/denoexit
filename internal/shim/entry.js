import { getHandler } from "./shim.js";
import "./app.js";

export default {
  fetch(request) {
    const handler = getHandler();
    if (!handler) return new Response("denoexit: the app never called Deno.serve", { status: 599 });
    return handler(request);
  },
};

import "./app.css";
import { mount } from "svelte";
import App from "./App.svelte";
import { client } from "./lib/api/client.gen";

// Point the generated client at the API base path. In dev, Vite proxies
// /api -> http://localhost:8080 (see vite.config.ts); in production the UI is
// served from the same origin as the API.
client.setConfig({ baseUrl: "/api/v1" });

const app = mount(App, {
  target: document.getElementById("app")!,
});

export default app;

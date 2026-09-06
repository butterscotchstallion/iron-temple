import "./app.css";
import { mount } from "svelte";
import App from "./App.svelte";

// Where the API lives is no longer configured here: every generated call goes
// through src/lib/apiFetch.ts, which owns the base path (and the offline case)
// for the whole app.

const app = mount(App, {
  target: document.getElementById("app")!,
});

export default app;

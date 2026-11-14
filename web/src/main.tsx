import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
// Inter loads from JS (as in groot): an @import inside the shell CSS would be
// inlined by Tailwind before Vite could emit the font files.
import "@fontsource-variable/inter";
import "./styles.css";
import { initTheme } from "@groot/shell/lib/theme";
import { ShellQueryProvider, createShellQueryClient } from "@groot/shell/lib/query-client";
import App from "./App";

initTheme();

// After a deploy, a tab still running the old bundle asks for chunks that no
// longer exist. Reload once to pick up the new build instead of breaking.
window.addEventListener("vite:preloadError", (event) => {
  try {
    if (sessionStorage.getItem("chunk-reload") === "1") return;
    sessionStorage.setItem("chunk-reload", "1");
  } catch {
    return;
  }
  event.preventDefault();
  window.location.reload();
});
window.setTimeout(() => {
  try {
    sessionStorage.removeItem("chunk-reload");
  } catch {
    // Storage blocked: nothing to clear.
  }
}, 10_000);

const queryClient = createShellQueryClient({ queries: { staleTime: 15_000 } });

const rootElement = document.getElementById("root");
if (!rootElement) throw new Error("Root element not found");

createRoot(rootElement).render(
  <StrictMode>
    <ShellQueryProvider client={queryClient}>
      <App />
    </ShellQueryProvider>
  </StrictMode>,
);

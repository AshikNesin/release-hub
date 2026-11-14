import { useEffect, type ReactNode } from "react";
import { QueryClient, QueryClientProvider, type DefaultOptions } from "@tanstack/react-query";
import { useAuthStore } from "@groot/shell/store/auth";

/**
 * The shell's React Query defaults. Queries stay fresh for 5 minutes, live in
 * the cache for 30, refetch on window focus and reconnect, and retry once.
 * Mutations never retry: a repeated POST can create a duplicate.
 */
export const SHELL_QUERY_DEFAULTS = {
  queries: {
    staleTime: 1000 * 60 * 5,
    gcTime: 1000 * 60 * 30,
    refetchOnWindowFocus: true,
    refetchOnReconnect: true,
    retry: 1,
  },
  mutations: {
    retry: false,
  },
} as const satisfies DefaultOptions;

/**
 * A `QueryClient` with the shell's defaults. `overrides` are merged per
 * section, so `{ queries: { retry: false } }` keeps every other default.
 */
export function createShellQueryClient(overrides: DefaultOptions = {}): QueryClient {
  return new QueryClient({
    defaultOptions: {
      ...overrides,
      queries: { ...SHELL_QUERY_DEFAULTS.queries, ...overrides.queries },
      mutations: { ...SHELL_QUERY_DEFAULTS.mutations, ...overrides.mutations },
    },
  });
}

/**
 * Forget everything the previous session fetched. Query keys say nothing
 * about the user, so data cached for one account must not be served to the
 * next. Inactive queries and the mutation cache are dropped; active queries
 * are reset (their data goes at once, and any request still in flight for the
 * old session is cancelled) and refetch as the new session.
 */
function resetForNewSession(client: QueryClient): void {
  client.getMutationCache().clear();
  client.removeQueries({ type: "inactive" });
  void client.resetQueries({ type: "active" });
}

/**
 * Reset `client` whenever the auth store's `generation` changes: every
 * sign-in, sign-out and session-invalid 401. Returns the unsubscribe function.
 * `ShellQueryProvider` calls this; call it yourself only for a client you
 * provide some other way.
 */
export function bindQueryClientToSession(client: QueryClient): () => void {
  let generation = useAuthStore.getState().generation;
  return useAuthStore.subscribe((state) => {
    if (state.generation === generation) return;
    generation = state.generation;
    resetForNewSession(client);
  });
}

/**
 * `QueryClientProvider` scoped to the session: the cache is reset on every
 * sign-in, sign-out and session-invalid 401, so one account never sees what
 * another fetched. Mount it at the root with a client from
 * `createShellQueryClient()`.
 */
export function ShellQueryProvider({
  client,
  children,
}: {
  client: QueryClient;
  children: ReactNode;
}) {
  useEffect(() => bindQueryClientToSession(client), [client]);
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

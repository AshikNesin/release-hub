// RELEASE-HUB OVERRIDE of @groot/shell/store/auth (skipped by
// scripts/sync-groot.sh). Same public surface as groot's store so the vendored
// Layout, ProtectedRoute, query-client and useSignOut work unchanged, minus
// what release-hub does not have: emails, passkeys and multiple accounts.
// The release hub has one admin, signed in with a password (Go: srv/api_v1.go).
import { create } from "zustand";
import { ApiError, apiClient } from "@groot/shell/lib/api";
import { describeError } from "@groot/shell/lib/feedback";

/** The signed-in user (`GET /auth/me`): always the release hub admin. */
export interface SessionUser {
  id: number;
  email: string;
  name: string | null;
}

export type AuthStatus =
  | { status: "unknown" }
  | { status: "signedOut" }
  | { status: "signedIn"; user: SessionUser };

interface AuthState {
  user: SessionUser | null;
  isAuthenticated: boolean;
  hasCheckedAuth: boolean;
  /** Bumped on every sign-in and sign-out (the query cache resets on it). */
  generation: number;
  login: (password: string) => Promise<SessionUser>;
  /** First run: set the admin password and sign in. 403 once it is set. */
  setup: (password: string) => Promise<SessionUser>;
  logout: () => Promise<void>;
  checkAuth: () => Promise<void>;
  clearAuth: () => void;
}

/** Sign-in failure wording: a 401 always means the password was wrong. */
export function loginErrorDescription(error: unknown, fallback: string): string {
  if (error instanceof ApiError && error.status === 401) return "Wrong password.";
  return describeError(error, fallback);
}

export function selectAuthStatus(state: Pick<AuthState, "user" | "hasCheckedAuth">): AuthStatus {
  if (state.user) return { status: "signedIn", user: state.user };
  return state.hasCheckedAuth ? { status: "signedOut" } : { status: "unknown" };
}

let generationChangedAt = 0;

export const useAuthStore = create<AuthState>((set, get) => {
  const change = (user: SessionUser | null) => {
    generationChangedAt = performance.now();
    set((s) => ({
      user,
      isAuthenticated: user !== null,
      hasCheckedAuth: true,
      generation: s.generation + 1,
    }));
  };
  const signIn = async (run: () => Promise<SessionUser>) => {
    const user = await run();
    change(user);
    return user;
  };

  return {
    user: null,
    isAuthenticated: false,
    hasCheckedAuth: false,
    generation: 0,

    login: (password) =>
      signIn(async () => {
        const { user } = await apiClient.post<{ user: SessionUser }>("/auth/login", { password });
        return user;
      }),

    setup: (password) =>
      signIn(async () => {
        const { user } = await apiClient.post<{ user: SessionUser }>("/auth/setup", { password });
        return user;
      }),

    logout: async () => {
      try {
        await apiClient.post("/auth/logout");
      } catch {
        // The local session ends either way.
      }
      change(null);
    },

    checkAuth: async () => {
      const startedAt = get().generation;
      let user: SessionUser | null = null;
      try {
        user = await apiClient.get<SessionUser>("/auth/me");
      } catch {
        // No session cookie, or an expired one.
      }
      if (get().generation !== startedAt) return;
      change(user);
    },

    clearAuth: () => change(null),
  };
});

apiClient.onUnauthorized(({ sentAt }) => {
  if (useAuthStore.getState().user !== null && sentAt >= generationChangedAt) {
    useAuthStore.getState().clearAuth();
  }
});

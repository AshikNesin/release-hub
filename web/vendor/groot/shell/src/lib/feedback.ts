import { useRef, useState, type ReactNode } from "react";
import { useQueryClient, type QueryKey } from "@tanstack/react-query";
import { toast } from "sonner";
import { useConfirm } from "@groot/ui/primitives";
import type { ConfirmOptions } from "@groot/ui/primitives/confirm";
import { ApiError, NetworkError } from "@groot/shell/lib/api";

/**
 * Mutation feedback: how the shell words a failure and runs a user action
 * ("run, toast, invalidate"). Every page that changes data goes through
 * `useAction`, so the same failure reads the same everywhere.
 */

/** Shown when a request never reached the server. */
export const OFFLINE_MESSAGE = "Could not reach the server. Check your connection and try again.";

/**
 * The one wording table for WebAuthn (passkey) failures, keyed by the
 * `DOMException` name the browser raises (`@simplewebauthn/browser` keeps it
 * as the error's `name`).
 */
export const WEBAUTHN_MESSAGES: Readonly<Record<string, string>> = {
  NotAllowedError: "The passkey request was cancelled or timed out. Please try again.",
  NotSupportedError: "Passkeys are not supported in this browser.",
  InvalidStateError: "This passkey is already registered to your account.",
  SecurityError: "Passkeys are not available on this address. Open the app on its own domain.",
};

/** `fetch` rejection messages across browsers, for a `TypeError` that did not come from `apiClient`. */
const FETCH_FAILURE = /failed to fetch|networkerror|load failed|network request failed/i;

/**
 * The `WEBAUTHN_MESSAGES` row for a browser WebAuthn `DOMException`, or for
 * `@simplewebauthn/browser`'s rethrow of one; undefined for anything else.
 */
export function describeWebAuthnError(error: unknown): string | undefined {
  if (typeof error !== "object" || error === null) return undefined;
  const { name, code } = error as { name?: unknown; code?: unknown };
  if (typeof name !== "string" || !Object.hasOwn(WEBAUTHN_MESSAGES, name)) return undefined;
  const fromBrowser = typeof DOMException !== "undefined" && error instanceof DOMException;
  // @simplewebauthn/browser's WebAuthnError carries an `ERROR_*` code.
  const fromLibrary = typeof code === "string" && code.startsWith("ERROR_");
  return fromBrowser || fromLibrary ? WEBAUTHN_MESSAGES[name] : undefined;
}

/**
 * The sentence a person sees for `error`:
 *
 * - an `ApiError` whose message came from the server's error envelope: that
 *   message (the server words its refusals for people); any other `ApiError`
 *   (a bare status line such as "Bad Gateway", or no data): `fallback`
 * - a request that never reached the server (`NetworkError`, or a `fetch`
 *   `TypeError`): `OFFLINE_MESSAGE`
 * - a WebAuthn failure: its row in `WEBAUTHN_MESSAGES`
 * - a plain `Error` (`new Error("Invalid JSON format")`), which is how an
 *   action's own code refuses: its message
 * - anything else (`SyntaxError`, a programming `TypeError`, a string):
 *   `fallback`
 */
export function describeError(error: unknown, fallback: string): string {
  if (error instanceof ApiError)
    return error.fromServer && error.message ? error.message : fallback;
  if (error instanceof NetworkError) return OFFLINE_MESSAGE;
  if (error instanceof TypeError) {
    return FETCH_FAILURE.test(error.message) ? OFFLINE_MESSAGE : fallback;
  }
  const webAuthn = describeWebAuthnError(error);
  if (webAuthn) return webAuthn;
  const ownError =
    error instanceof Error &&
    Object.getPrototypeOf(error) === Error.prototype &&
    error.name === "Error";
  return ownError && error.message ? error.message : fallback;
}

/** A toast: a title alone, or a title with a description. */
export type ToastContent = string | { title: string; description?: ReactNode };

/** Show a success toast. */
export function toastSuccess(content: ToastContent): void {
  if (typeof content === "string") toast.success(content);
  else toast.success(content.title, { description: content.description });
}

/** Log `error` and show it as an error toast, worded by `describeError`. */
export function toastError(
  error: unknown,
  { title, fallback }: { title: string; fallback: string },
) {
  console.error(error);
  toast.error(title, { description: describeError(error, fallback) });
}

/** What `useAction` does around the action. */
export interface ActionOptions<TVars, TData> {
  /** Ask first. Return `null` from the function form to skip the question for these vars. */
  confirm?: ConfirmOptions | ((vars: TVars) => ConfirmOptions | null);
  /** The success toast; none when omitted. */
  success?: ToastContent | ((data: TData, vars: TVars) => ToastContent);
  /** The error toast's title. Defaults to "Something went wrong". */
  errorTitle?: string;
  /** The error toast's description when `describeError` has nothing better. */
  fallback: string | ((vars: TVars) => string);
  /** Query keys to invalidate after success; awaited, so `isPending` lasts until they refetch. */
  invalidate?: QueryKey[] | ((data: TData, vars: TVars) => QueryKey[]);
  /** Runs last, after the toast and the invalidation (close a dialog, reset a form). */
  onSuccess?: (data: TData, vars: TVars) => void | Promise<void>;
}

/**
 * How a run ended: `done` with the action's result, or not done because the
 * person declined the confirmation (`cancelled`), a run of this action was
 * already under way (`busy`), or the action threw (`failed`, already toasted).
 */
export type ActionResult<TData> =
  | { ok: true; data: TData }
  | { ok: false; reason: "cancelled" | "busy" }
  | { ok: false; reason: "failed"; error: unknown };

export interface Action<TVars, TData> {
  /** Confirm, run, toast and invalidate. Never rejects. */
  run: (vars: TVars) => Promise<ActionResult<TData>>;
  /** True from the confirmed start of a run until its invalidations settle. */
  isPending: boolean;
}

/**
 * Run a user action with the shell's feedback: an optional confirmation, one
 * run at a time (a second click while one is under way, or while its
 * confirmation is open, is ignored), a success toast, an error toast worded
 * by `describeError`, and query invalidation. Needs `GrootUIProvider` (for
 * `useConfirm` and the toaster) and a `QueryClientProvider` above it.
 *
 * ```ts
 * const remove = useAction((id: number) => apiClient.delete(`/todos/${id}`), {
 *   confirm: { title: "Delete todo?", destructive: true },
 *   success: "Todo deleted",
 *   errorTitle: "Delete failed",
 *   fallback: "Unable to delete todo",
 *   invalidate: [["todos"]],
 * });
 * <Button onClick={() => void remove.run(todo.id)} disabled={remove.isPending} />
 * ```
 */
export function useAction<TVars = void, TData = unknown>(
  action: (vars: TVars) => Promise<TData>,
  options: ActionOptions<TVars, TData>,
): Action<TVars, TData> {
  const queryClient = useQueryClient();
  const confirm = useConfirm();
  const busy = useRef(false);
  const [isPending, setPending] = useState(false);

  const run = async (vars: TVars): Promise<ActionResult<TData>> => {
    if (busy.current) return { ok: false, reason: "busy" };
    busy.current = true;
    try {
      const question =
        typeof options.confirm === "function" ? options.confirm(vars) : options.confirm;
      if (question && !(await confirm(question))) return { ok: false, reason: "cancelled" };

      setPending(true);
      let data: TData;
      try {
        data = await action(vars);
      } catch (error) {
        toastError(error, {
          title: options.errorTitle ?? "Something went wrong",
          fallback:
            typeof options.fallback === "function" ? options.fallback(vars) : options.fallback,
        });
        return { ok: false, reason: "failed", error };
      }

      if (options.success) {
        toastSuccess(
          typeof options.success === "function" ? options.success(data, vars) : options.success,
        );
      }
      const keys =
        typeof options.invalidate === "function"
          ? options.invalidate(data, vars)
          : (options.invalidate ?? []);
      await Promise.all(keys.map((queryKey) => queryClient.invalidateQueries({ queryKey })));
      await options.onSuccess?.(data, vars);
      return { ok: true, data };
    } finally {
      busy.current = false;
      setPending(false);
    }
  };

  return { run, isPending };
}

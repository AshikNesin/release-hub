import { useMutation } from "@tanstack/react-query";
import { toast } from "sonner";
import { describeError, toastSuccess } from "@groot/shell/lib/feedback";

type ToastContent = string | { title: string; description?: string };

interface ToastMutationConfig<TData, TVars> {
  /** Shown on success. Pass a function for a result-dependent message. */
  success?: ToastContent | ((data: TData, vars: TVars) => ToastContent);
  /**
   * Shown on error (the error is always `console.error`-logged too). The
   * description is the fallback for `describeError`, so a server's message or
   * the action's own `Error` wins over it.
   */
  error?: ToastContent;
  /** Runs after a successful mutation, e.g. invalidate queries or reset UI. */
  onSuccess?: (data: TData, vars: TVars) => void | Promise<void>;
}

/**
 * @deprecated Use `useAction` from `@groot/shell/lib/feedback`, which also
 * owns confirmation, double-submit and invalidation. Kept for existing callers.
 *
 * `useMutation` + toast wiring in one call. Collapses the repeated
 *   try { await fn(); toast.success(...) } catch (e) { console.error(e); toast.error(...) }
 * boilerplate. `mutateAsync` still rejects on error, so callers awaiting it
 * can run post-success code in a `try` with an empty catch.
 *
 * The `onSuccess` callback is awaited, so a returned invalidation Promise
 * keeps `isPending`/`mutateAsync` in sync with the refetch.
 */
export function useToastMutation<TData, TVars>(
  mutationFn: (vars: TVars) => Promise<TData>,
  config: ToastMutationConfig<TData, TVars> = {},
) {
  return useMutation<TData, Error, TVars>({
    mutationFn,
    onSuccess: async (data, vars) => {
      if (config.success) {
        const content =
          typeof config.success === "function" ? config.success(data, vars) : config.success;
        toastSuccess(content);
      }
      await config.onSuccess?.(data, vars);
    },
    onError: (error) => {
      console.error(error);
      const content = config.error;
      if (typeof content === "string") toast.error(describeError(error, content));
      else if (content) {
        toast.error(content.title, {
          description: describeError(error, content.description ?? "") || undefined,
        });
      }
    },
  });
}

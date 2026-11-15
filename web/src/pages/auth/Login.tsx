import { useState } from "react";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { ArrowRight, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { z } from "zod";
import { Button } from "@groot/ui/button";
import { Form, FormField } from "@groot/ui/form";
import { Input } from "@groot/ui/input";
import { LoadingSpinner } from "@groot/ui/loading-spinner";
import { loginErrorDescription, selectAuthStatus, useAuthStore } from "@groot/shell/store/auth";
import { AuthFrame } from "./AuthFrame";
import { useSetupStatus } from "./useSetupStatus";

const loginSchema = z.object({ password: z.string().min(1, "Enter the admin password") });

/** Where to go after signing in: the protected page that sent us here, else /apps. */
function returnPath(state: unknown): string {
  const from = (state as { from?: { pathname?: unknown; search?: unknown } })?.from;
  if (!from || typeof from.pathname !== "string" || !from.pathname.startsWith("/")) return "/apps";
  if (from.pathname.startsWith("//") || from.pathname === "/login") return "/apps";
  return `${from.pathname}${typeof from.search === "string" ? from.search : ""}`;
}

export function Login() {
  const login = useAuthStore((s) => s.login);
  const status = useAuthStore((s) => selectAuthStatus(s).status);
  const needsSetup = useSetupStatus();
  const navigate = useNavigate();
  const location = useLocation();
  const [busy, setBusy] = useState(false);

  if (needsSetup === undefined) {
    return (
      <div className="flex min-h-dvh items-center justify-center bg-muted/40">
        <LoadingSpinner size="md" />
      </div>
    );
  }
  if (needsSetup) return <Navigate to="/setup" replace />;
  if (status === "signedIn") return <Navigate to={returnPath(location.state)} replace />;

  const onSubmit = async ({ password }: z.infer<typeof loginSchema>) => {
    if (busy) return;
    setBusy(true);
    try {
      await login(password);
      navigate(returnPath(location.state), { replace: true });
    } catch (error) {
      toast.error("Sign-in failed", {
        description: loginErrorDescription(error, "Could not sign in"),
      });
    } finally {
      setBusy(false);
    }
  };

  return (
    <AuthFrame title="Welcome back" subtitle="Sign in with the release hub's admin password.">
      <Form
        schema={loginSchema}
        defaultValues={{ password: "" }}
        onSubmit={onSubmit}
        className="mt-6 space-y-4"
      >
        <FormField name="password" label="Admin password">
          <Input type="password" autoComplete="current-password" placeholder="••••••••" autoFocus />
        </FormField>
        <Button type="submit" size="lg" className="w-full" disabled={busy} aria-busy={busy}>
          {busy ? "Signing in…" : "Sign in"}
          {busy ? <Loader2 className="size-4 animate-spin" /> : <ArrowRight className="size-4" />}
        </Button>
      </Form>
    </AuthFrame>
  );
}

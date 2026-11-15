import { Navigate, useNavigate } from "react-router-dom";
import { ArrowRight } from "lucide-react";
import { toast } from "sonner";
import { z } from "zod";
import { Button } from "@groot/ui/button";
import { Form, FormField } from "@groot/ui/form";
import { Input } from "@groot/ui/input";
import { LoadingSpinner } from "@groot/ui/loading-spinner";
import { ApiError } from "@groot/shell/lib/api";
import { loginErrorDescription, useAuthStore } from "@groot/shell/store/auth";
import { AuthFrame } from "./AuthFrame";
import { useSetupStatus } from "./useSetupStatus";

// Mirrors the server rule (srv/api_v1.go handleV1Setup); the confirmation never leaves the browser.
const setupSchema = z
  .object({
    password: z.string().min(8, "Use at least 8 characters"),
    confirmPassword: z.string(),
  })
  .refine((v) => v.password === v.confirmPassword, {
    message: "Passwords do not match",
    path: ["confirmPassword"],
  });

/** First run at /setup: set the admin password and sign in. Closed once it is set. */
export function Setup() {
  const needsSetup = useSetupStatus();
  const setup = useAuthStore((s) => s.setup);
  const navigate = useNavigate();

  if (needsSetup === undefined) {
    return (
      <div className="flex min-h-dvh items-center justify-center bg-muted/40">
        <LoadingSpinner size="md" />
      </div>
    );
  }
  if (!needsSetup) return <Navigate to="/login" replace />;

  const onSubmit = async ({ password }: z.infer<typeof setupSchema>) => {
    try {
      await setup(password);
      toast.success("Release hub ready", { description: "Register your first app to get started." });
      navigate("/apps", { replace: true });
    } catch (error) {
      if (error instanceof ApiError && error.status === 403) {
        toast.error("Setup already complete", { description: "Sign in with the admin password." });
        navigate("/login", { replace: true });
        return;
      }
      toast.error("Setup failed", {
        description: loginErrorDescription(error, "Could not save the password"),
      });
    }
  };

  return (
    <AuthFrame
      title="Set the admin password"
      subtitle="This release hub has no password yet. The one you choose here signs you in, and this page closes once it is set."
    >
      <Form
        schema={setupSchema}
        defaultValues={{ password: "", confirmPassword: "" }}
        onSubmit={onSubmit}
        className="mt-6 space-y-4"
      >
        <FormField name="password" label="Password">
          <Input type="password" autoComplete="new-password" placeholder="••••••••" autoFocus />
        </FormField>
        <FormField name="confirmPassword" label="Confirm password">
          <Input type="password" autoComplete="new-password" placeholder="••••••••" />
        </FormField>
        <Button type="submit" size="lg" className="w-full">
          Save and sign in
          <ArrowRight className="size-4" />
        </Button>
      </Form>
    </AuthFrame>
  );
}

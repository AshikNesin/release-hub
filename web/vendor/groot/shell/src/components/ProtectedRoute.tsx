import { Navigate, useLocation } from "react-router-dom";
import type { ReactNode } from "react";
import { LoadingSpinner } from "@groot/ui/loading-spinner";
import { selectAuthStatus, useAuthStore } from "@groot/shell/store/auth";

interface ProtectedRouteProps {
  children: ReactNode;
}

export function ProtectedRoute({ children }: ProtectedRouteProps) {
  const status = useAuthStore((state) => selectAuthStatus(state).status);
  const location = useLocation();

  if (status === "unknown") {
    return (
      <div className="flex min-h-dvh items-center justify-center bg-muted/40">
        <LoadingSpinner size="md" />
      </div>
    );
  }

  if (status === "signedOut") {
    // Remember where the visitor was headed so login can send them back.
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  return <>{children}</>;
}

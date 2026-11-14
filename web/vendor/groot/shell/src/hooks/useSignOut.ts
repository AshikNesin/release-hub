import { useNavigate } from "react-router-dom";
import { toast } from "sonner";
import { useAuthStore } from "@groot/shell/store/auth";

/** The one sign-out action every shell surface uses: end the session, say so, go to /login. */
export function useSignOut(): () => void {
  const logout = useAuthStore((state) => state.logout);
  const navigate = useNavigate();
  return () => {
    void logout();
    toast.success("Signed out", { description: "You have been logged out safely." });
    navigate("/login");
  };
}

import { useSyncExternalStore, type CSSProperties } from "react";
import { Toaster as Sonner, type ToasterProps } from "sonner";
import {
  CircleCheckIcon,
  InfoIcon,
  TriangleAlertIcon,
  OctagonXIcon,
  Loader2Icon,
} from "lucide-react";

/**
 * The shell themes the app with a `dark` class on <html>; sonner needs the
 * theme passed in, or its rich colors stay light on a dark page. Read the
 * class directly (this package cannot import the shell's theme hook).
 */
function subscribeToRootClass(onChange: () => void) {
  const observer = new MutationObserver(onChange);
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
  return () => observer.disconnect();
}

function useRootTheme(): "light" | "dark" {
  return useSyncExternalStore(
    subscribeToRootClass,
    () => (document.documentElement.classList.contains("dark") ? "dark" : "light"),
    () => "light",
  );
}

const Toaster = ({ ...props }: ToasterProps) => (
  <Sonner
    richColors
    theme={useRootTheme()}
    className="toaster group"
    icons={{
      success: <CircleCheckIcon className="size-4" />,
      info: <InfoIcon className="size-4" />,
      warning: <TriangleAlertIcon className="size-4" />,
      error: <OctagonXIcon className="size-4" />,
      loading: <Loader2Icon className="size-4 animate-spin" />,
    }}
    style={
      {
        "--normal-bg": "var(--popover)",
        "--normal-text": "var(--popover-foreground)",
        "--normal-border": "var(--border)",
        "--border-radius": "var(--radius-xl)",
      } as CSSProperties
    }
    toastOptions={{
      classNames: {
        toast: "cn-toast",
      },
    }}
    {...props}
  />
);

export { Toaster };

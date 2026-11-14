import type { ReactNode } from "react";
import { Navigation, ShieldCheck, Upload } from "lucide-react";
import { displayClass, h1Class } from "@groot/ui/typography";
import { useShellConfig } from "@groot/shell/lib/shell-config";

const FEATURES = [
  {
    icon: Upload,
    title: "One upload endpoint",
    desc: "CI posts the build once; the hub stores it and pushes .aab files to Play.",
  },
  {
    icon: Navigation,
    title: "Every channel",
    desc: "Direct APKs with an update manifest, plus Play internal, closed, open and production.",
  },
  {
    icon: ShieldCheck,
    title: "Keys that survive laptops",
    desc: "Signing keystores generated per app and encrypted at rest.",
  },
];

/** Groot's sign-in layout: form on the left, brand panel on the right (lg+). */
export function AuthFrame({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle: string;
  children: ReactNode;
}) {
  const { icon: BrandMark, label } = useShellConfig().brand;
  return (
    <div className="flex min-h-dvh bg-background">
      <div className="flex w-full flex-col justify-center px-6 py-12 sm:px-12 lg:w-1/2 lg:px-16">
        <div className="mx-auto w-full max-w-sm">
          <div className="mb-8 flex items-center gap-2">
            <span className="flex size-8 items-center justify-center rounded-xl bg-primary text-primary-foreground">
              <BrandMark className="size-5" />
            </span>
            <span className="font-heading text-base font-semibold">{label}</span>
          </div>
          <h1 className={h1Class}>{title}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{subtitle}</p>
          {children}
        </div>
      </div>

      <div className="relative hidden overflow-hidden bg-primary lg:flex lg:w-1/2 lg:flex-col lg:justify-center lg:px-16 dark:bg-card">
        <div
          className="absolute inset-0 opacity-[0.07]"
          style={{
            backgroundImage: "radial-gradient(circle at 1px 1px, currentColor 1px, transparent 0)",
            backgroundSize: "32px 32px",
          }}
        />
        <div className="relative max-w-md text-primary-foreground dark:text-foreground">
          <h2 className={displayClass}>Ship every build from one place.</h2>
          <p className="mt-3 text-sm text-primary-foreground/70 dark:text-foreground/70">
            Versioning, artifact storage, channels and the update manifest for your apps — so build
            environments only ever talk to one endpoint.
          </p>
          <ul className="mt-8 space-y-4">
            {FEATURES.map((f) => (
              <li key={f.title} className="flex items-start gap-3">
                <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-primary-foreground/10 dark:bg-foreground/10">
                  <f.icon className="size-4 text-primary-foreground dark:text-foreground" />
                </span>
                <div>
                  <p className="text-sm font-medium text-primary-foreground dark:text-foreground">
                    {f.title}
                  </p>
                  <p className="text-sm text-primary-foreground/60 dark:text-foreground/60">
                    {f.desc}
                  </p>
                </div>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </div>
  );
}

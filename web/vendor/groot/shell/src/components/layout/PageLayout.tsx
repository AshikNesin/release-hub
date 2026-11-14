import type { ReactNode } from "react";
import { h1Class } from "@groot/ui/typography";
import { cn } from "@groot/ui/lib/utils";
import { PageContainer } from "./PageContainer";

interface PageLayoutProps {
  children: ReactNode;
  title: string;
  description?: ReactNode;
  actions?: ReactNode;
  /** Rendered above the title (e.g. a `Breadcrumb`). */
  breadcrumb?: ReactNode;
  /** Rendered inline after the title (e.g. a `StatusBadge`). */
  titleAdornment?: ReactNode;
  className?: string;
  maxWidth?: "full" | "7xl" | "6xl" | "5xl" | "4xl";
}

/**
 * A routed page: max-width container, header (title, description, actions)
 * and content.
 *
 * Every routed page renders through this so titles, max-widths and
 * header-to-content spacing stay identical across the app; it owns the one
 * sanctioned `<h1>`. Detail pages that need a breadcrumb trail or a status
 * pill beside the title use the `breadcrumb` / `titleAdornment` slots.
 *
 * @example
 * ```tsx
 * <PageLayout
 *   title="Dashboard"
 *   description="Overview of your finances"
 *   actions={<Button>Action</Button>}
 * >
 *   <Card>Content here</Card>
 * </PageLayout>
 * ```
 */
export function PageLayout({
  children,
  title,
  description,
  actions,
  breadcrumb,
  titleAdornment,
  className,
  maxWidth = "5xl",
}: PageLayoutProps) {
  return (
    <PageContainer maxWidth={maxWidth} className={className}>
      {breadcrumb && <div className="mb-4">{breadcrumb}</div>}
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2.5">
            <h1 className={cn("truncate", h1Class)}>{title}</h1>
            {titleAdornment}
          </div>
          {description && <div className="mt-1 text-sm text-muted-foreground">{description}</div>}
        </div>
        {actions && <div className="flex flex-shrink-0 items-center gap-2">{actions}</div>}
      </div>
      <div className="mt-8 space-y-6">{children}</div>
    </PageContainer>
  );
}

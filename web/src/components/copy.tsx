import { useEffect, useRef, useState } from "react";
import { Check, Copy } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@groot/ui/button";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@groot/ui/input-group";
import { cn } from "@groot/ui/lib/utils";

async function writeClipboard(text: string) {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    // Non-secure context (plain-http self-hosting): fall back to a hidden textarea.
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    document.execCommand("copy");
    ta.remove();
  }
}

/** Copy `text`, flipping the icon to a check for a moment. */
function useCopy(text: string, what?: string) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<number>(undefined);
  useEffect(() => () => window.clearTimeout(timer.current), []);
  const copy = async () => {
    await writeClipboard(text);
    setCopied(true);
    if (what) toast.success(`${what} copied`);
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setCopied(false), 1400);
  };
  return { copied, copy };
}

/** Icon-only copy button. */
export function CopyButton({
  text,
  label = "Copy",
  what,
  className,
}: {
  text: string;
  label?: string;
  what?: string;
  className?: string;
}) {
  const { copied, copy } = useCopy(text, what);
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon-sm"
      aria-label={label}
      title={label}
      onClick={() => void copy()}
      className={cn("text-muted-foreground hover:text-foreground", className)}
    >
      {copied ? <Check className="text-success" /> : <Copy />}
    </Button>
  );
}

/** A read-only, monospaced value with a copy button: URLs, commands, emails. */
export function CopyField({
  value,
  what,
  className,
}: {
  value: string;
  what?: string;
  className?: string;
}) {
  const { copied, copy } = useCopy(value, what);
  return (
    <InputGroup className={className}>
      <InputGroupInput
        readOnly
        value={value}
        aria-label={what ?? "Value"}
        onFocus={(e) => e.currentTarget.select()}
        className="font-mono text-xs"
      />
      <InputGroupAddon align="inline-end">
        <InputGroupButton
          size="icon-xs"
          aria-label={`Copy ${what ?? "value"}`}
          onClick={() => void copy()}
        >
          {copied ? <Check className="text-success" /> : <Copy />}
        </InputGroupButton>
      </InputGroupAddon>
    </InputGroup>
  );
}

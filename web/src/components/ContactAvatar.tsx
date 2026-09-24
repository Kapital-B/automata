import { cn } from "@/lib/utils";
import { initials } from "@/lib/contacts";

export function ContactAvatar({
  name,
  email,
  size = "md",
  className,
}: {
  name: string;
  email?: string;
  size?: "md" | "lg";
  className?: string;
}) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "inline-flex shrink-0 select-none items-center justify-center rounded-full bg-secondary font-medium text-secondary-foreground",
        size === "md" ? "h-9 w-9 text-xs" : "h-14 w-14 text-lg",
        className,
      )}
    >
      {initials(name, email)}
    </span>
  );
}

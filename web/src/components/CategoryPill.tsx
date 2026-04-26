import { cn } from "@/lib/utils";

const styles: Record<string, string> = {
  important: "bg-[hsl(184_55%_22%/0.08)] text-[hsl(184_55%_22%)] border-[hsl(184_55%_22%/0.18)]",
  finance: "bg-[hsl(152_45%_38%/0.10)] text-[hsl(152_45%_28%)] border-[hsl(152_45%_38%/0.20)]",
  newsletter: "bg-[hsl(220_10%_42%/0.08)] text-[hsl(220_10%_30%)] border-[hsl(220_10%_42%/0.18)]",
  personal: "bg-[hsl(28_70%_52%/0.10)] text-[hsl(28_70%_38%)] border-[hsl(28_70%_52%/0.22)]",
  spam: "bg-[hsl(358_70%_48%/0.08)] text-[hsl(358_60%_42%)] border-[hsl(358_70%_48%/0.20)]",
  uncategorized: "bg-muted text-muted-foreground border-border border-dashed",
  other: "bg-muted text-muted-foreground border-border",
};

export function CategoryPill({ category, className }: { category: string; className?: string }) {
  const normalized = (category || "uncategorized").toLowerCase();
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] font-medium uppercase tracking-wider",
        styles[normalized] ?? styles.other,
        className
      )}
    >
      {normalized}
    </span>
  );
}

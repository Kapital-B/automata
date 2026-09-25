import { Skeleton } from "@/components/ui/skeleton";

/** Placeholder rows shaped like a list page's rows, so nothing jumps on load. */
export function ListSkeleton({ label, rows = 6 }: { label: string; rows?: number }) {
  return (
    <ul aria-label={label} className="surface-card divide-y divide-border/70">
      {Array.from({ length: rows }, (_, i) => (
        <li key={i} className="flex items-center gap-3 px-4 py-3">
          <Skeleton className="h-9 w-9 rounded-md" />
          <div className="space-y-1.5">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-3 w-56" />
          </div>
        </li>
      ))}
    </ul>
  );
}

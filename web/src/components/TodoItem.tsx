import { Link } from "react-router-dom";
import { Check, ListTodo } from "lucide-react";
import { Button } from "@/components/ui/button";
import { dueLabel } from "@/lib/todos";
import { cn } from "@/lib/utils";

/**
 * One to-do from mail, rendered the same on Home, a project and an issue. The
 * text opens the message it came from when it is yours; a teammate's mail
 * stays private, so theirs is plain text. Whose it is, where it sits and when
 * it is due read underneath; Done closes it.
 */
export function TodoItem({
  text,
  href,
  owner,
  label,
  projectLabel,
  issueTitle,
  issueHref,
  dueAt,
  pending,
  onDone,
}: {
  text: string;
  /** Opens the message. Absent for a teammate's to-do, whose mail is private. */
  href?: string;
  /** Whose to-do it is, on shared lists. */
  owner?: string;
  /** Leading word in the meta line, e.g. "To-do" where rows of other kinds sit alongside. */
  label?: string;
  projectLabel?: string;
  issueTitle?: string;
  issueHref?: string;
  dueAt?: string;
  pending?: boolean;
  onDone?: () => void;
}) {
  const due = dueLabel(dueAt);
  const meta = [
    label && <span key="label">{label}</span>,
    owner && (
      <span key="owner" className="truncate font-medium text-foreground/80">
        {owner}
      </span>
    ),
    projectLabel && (
      <span key="project" className="truncate">
        {projectLabel}
      </span>
    ),
    issueTitle &&
      (issueHref ? (
        <Link key="issue" to={issueHref} className="truncate font-medium text-primary hover:underline">
          On: {issueTitle}
        </Link>
      ) : (
        <span key="issue" className="truncate">
          On: {issueTitle}
        </span>
      )),
    due && (
      <span key="due" className={cn(due.overdue && "font-medium text-destructive")}>
        {due.text}
      </span>
    ),
  ].filter(Boolean);

  return (
    <li className="flex items-start gap-3 px-4 py-3">
      <span
        aria-hidden="true"
        className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-border bg-secondary"
      >
        <ListTodo className="h-4 w-4 text-muted-foreground" />
      </span>
      <div className="min-w-0 flex-1 space-y-1">
        {href ? (
          <Link
            to={href}
            className="block rounded-sm font-medium hover:underline focus-visible:underline focus-visible:outline-none"
          >
            {text}
          </Link>
        ) : (
          <p className="font-medium">{text}</p>
        )}
        {meta.length > 0 && (
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
            {meta.map((m, i) => (
              <span key={i} className="inline-flex min-w-0 items-center gap-2">
                {i > 0 && <span aria-hidden="true">·</span>}
                {m}
              </span>
            ))}
          </div>
        )}
      </div>
      {onDone && (
        <Button
          size="sm"
          variant="outline"
          className="shrink-0"
          aria-label={`Mark “${text}” done`}
          onClick={onDone}
          disabled={pending}
        >
          <Check aria-hidden="true" className="mr-1.5 h-3.5 w-3.5" />
          Done
        </Button>
      )}
    </li>
  );
}

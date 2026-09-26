import { Skeleton } from "@/components/ui/skeleton";
import { TodoItem } from "@/components/TodoItem";
import type { ProjectTodo } from "@/lib/auth";
import { inboxHref } from "@/lib/todos";

/**
 * What the project's mail asks people to do, whoever's mailbox it arrived in.
 * Everyone on the project sees the same list and anyone can close an item;
 * only its owner can open the mail behind it.
 */
export function TodosSection({
  todos,
  loading,
  projectID,
  busy,
  onDone,
}: {
  todos: ProjectTodo[];
  loading: boolean;
  projectID: string;
  busy: boolean;
  onDone: (todoID: string) => void;
}) {
  if (!loading && todos.length === 0) return null;
  return (
    <section aria-labelledby="todos-heading" className="space-y-3">
      <div>
        <div className="flex items-baseline gap-2">
          <h2 id="todos-heading" className="font-display text-xl font-medium">
            To-dos
          </h2>
          {!loading && (
            <span className="rounded-full bg-secondary px-2 py-0.5 text-xs font-medium text-foreground">{todos.length}</span>
          )}
        </div>
        <p className="text-sm text-muted-foreground">
          What this project&apos;s mail asks the team to do. Anyone on the project can mark one done; only its owner can
          open the mail.
        </p>
      </div>
      {loading ? (
        <div aria-label="Loading to-dos" className="surface-card space-y-2 p-4">
          <Skeleton className="h-4 w-64" />
          <Skeleton className="h-4 w-48" />
        </div>
      ) : (
        <ul aria-label="To-dos" className="surface-card divide-y divide-border/70 overflow-hidden">
          {todos.map((t) => (
            <TodoItem
              key={t.id}
              text={t.text}
              href={t.is_mine && t.message_id && t.account_id ? inboxHref(t.message_id, t.account_id) : undefined}
              owner={t.owner_label}
              issueTitle={t.issue_title}
              issueHref={t.issue_id ? `/projects/${projectID}/issues/${t.issue_id}` : undefined}
              dueAt={t.due_at}
              pending={busy}
              onDone={() => onDone(t.id)}
            />
          ))}
        </ul>
      )}
    </section>
  );
}


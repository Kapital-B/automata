import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { FactDetail, ProjectDetail } from "@/lib/auth";
import { factKeyFor, selectClass, type ItemRef } from "./format";

export type { ItemRef } from "./format";

function SubmitButton({ pending, children, disabled }: { pending: boolean; children: React.ReactNode; disabled?: boolean }) {
  return (
    <Button type="submit" disabled={disabled || pending}>
      {pending && <Loader2 aria-hidden="true" className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
      {children}
    </Button>
  );
}

export type FactInput = {
  subjectKey: string;
  label: string;
  value: string;
  unit: string;
  confirm: boolean;
  evidence: ItemRef[];
};

/**
 * Record a value. Picking an existing fact adds a new version of it (the old
 * one is kept as history); a new label starts a fact. The identifier behind
 * this is derived, never typed.
 */
export function FactDialog({
  open,
  onOpenChange,
  facts,
  seed,
  pending,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  facts: FactDetail[];
  /** Prefill from correspondence: its title, and the item as evidence. */
  seed?: { label?: string; evidence?: ItemRef[] };
  pending: boolean;
  onSubmit: (input: FactInput) => void;
}) {
  const [existing, setExisting] = useState("");
  const [label, setLabel] = useState("");
  const [value, setValue] = useState("");
  const [unit, setUnit] = useState("");
  const [confirm, setConfirm] = useState(true);

  useEffect(() => {
    if (!open) return;
    setExisting("");
    setLabel(seed?.label ?? "");
    setValue("");
    setUnit("");
    setConfirm(true);
  }, [open, seed]);

  const picked = facts.find((f) => f.subject_key === existing);
  useEffect(() => {
    if (!picked) return;
    const active = picked.versions.find((v) => v.status === "active");
    setUnit(active?.unit ?? "");
  }, [picked]);

  const subjectKey = picked ? picked.subject_key : factKeyFor(label);
  const ready = subjectKey !== "" && (picked || label.trim() !== "") && value.trim() !== "";
  const evidence = seed?.evidence ?? [];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="font-display text-xl">Record a fact</DialogTitle>
          <DialogDescription>A value that is true about this project now. Changing one keeps the old value as history.</DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (!ready || pending) return;
            onSubmit({ subjectKey, label: picked ? picked.label : label.trim(), value: value.trim(), unit: unit.trim(), confirm, evidence });
          }}
        >
          {facts.length > 0 && (
            <div className="space-y-1.5">
              <Label htmlFor="fact-existing">Fact</Label>
              <select id="fact-existing" className={selectClass} value={existing} onChange={(e) => setExisting(e.target.value)}>
                <option value="">A new fact…</option>
                {facts.map((f) => (
                  <option key={f.id} value={f.subject_key}>
                    Update: {f.label}
                  </option>
                ))}
              </select>
            </div>
          )}
          {!picked && (
            <div className="space-y-1.5">
              <Label htmlFor="fact-label">What it is</Label>
              <Input id="fact-label" value={label} onChange={(e) => setLabel(e.target.value)} placeholder="Pump P-03 duty" autoFocus />
            </div>
          )}
          <div className="grid grid-cols-[1fr_7rem] gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="fact-value">{picked ? "New value" : "Value"}</Label>
              <Input id="fact-value" value={value} onChange={(e) => setValue(e.target.value)} placeholder="90" />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="fact-unit">
                Unit <span className="font-normal text-muted-foreground">(optional)</span>
              </Label>
              <Input id="fact-unit" value={unit} onChange={(e) => setUnit(e.target.value)} placeholder="kW" />
            </div>
          </div>
          <div className="flex items-start justify-between gap-4 rounded-md border border-border px-3 py-2.5">
            <div>
              <Label htmlFor="fact-confirm">Confirm now</Label>
              <p className="text-xs text-muted-foreground">
                {confirm ? "It becomes the current value straight away." : "It waits in Needs you for a second look."}
              </p>
            </div>
            <Switch id="fact-confirm" checked={confirm} onCheckedChange={setConfirm} />
          </div>
          {evidence.length > 0 && (
            <p className="text-xs text-muted-foreground">
              The correspondence you started from is attached as evidence.
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <SubmitButton pending={pending} disabled={!ready}>
              Save fact
            </SubmitButton>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function DecisionDialog({
  open,
  onOpenChange,
  pending,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pending: boolean;
  onSubmit: (input: { statement: string; accept: boolean }) => void;
}) {
  const [statement, setStatement] = useState("");
  const [accept, setAccept] = useState(true);
  useEffect(() => {
    if (open) {
      setStatement("");
      setAccept(true);
    }
  }, [open]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="font-display text-xl">Record a decision</DialogTitle>
          <DialogDescription>An approval, a go/no-go, a choice the project has committed to.</DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (statement.trim() && !pending) onSubmit({ statement: statement.trim(), accept });
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="decision-statement">Decision</Label>
            <Textarea
              id="decision-statement"
              value={statement}
              onChange={(e) => setStatement(e.target.value)}
              placeholder="Proceed with 90 kW duty for Pump P-03"
              rows={3}
              autoFocus
            />
          </div>
          <div className="flex items-start justify-between gap-4 rounded-md border border-border px-3 py-2.5">
            <div>
              <Label htmlFor="decision-accept">Accept now</Label>
              <p className="text-xs text-muted-foreground">
                {accept ? "It goes straight into the project's position." : "It waits in Needs you until someone accepts it."}
              </p>
            </div>
            <Switch id="decision-accept" checked={accept} onCheckedChange={setAccept} />
          </div>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <SubmitButton pending={pending} disabled={!statement.trim()}>
              Save decision
            </SubmitButton>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function IssueDialog({
  open,
  onOpenChange,
  seed,
  pending,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  seed?: { title?: string; items?: ItemRef[] };
  pending: boolean;
  onSubmit: (input: { title: string; note: string; items: ItemRef[] }) => void;
}) {
  const [title, setTitle] = useState("");
  const [note, setNote] = useState("");
  useEffect(() => {
    if (open) {
      setTitle(seed?.title ?? "");
      setNote("");
    }
  }, [open, seed]);
  const items = seed?.items ?? [];
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="font-display text-xl">New issue</DialogTitle>
          <DialogDescription>An open question or piece of work someone has to act on. It is assigned to you.</DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (title.trim() && !pending) onSubmit({ title: title.trim(), note: note.trim(), items });
          }}
        >
          <div className="space-y-1.5">
            <Label htmlFor="issue-title">Title</Label>
            <Input id="issue-title" value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Pump P-03 sizing" autoFocus />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="issue-note">
              Where it stands <span className="font-normal text-muted-foreground">(optional)</span>
            </Label>
            <Textarea id="issue-note" value={note} onChange={(e) => setNote(e.target.value)} rows={2} placeholder="Waiting on the vendor's revised curve" />
          </div>
          {items.length > 0 && (
            <p className="text-xs text-muted-foreground">Pre-attached from the correspondence you started from.</p>
          )}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <SubmitButton pending={pending} disabled={!title.trim()}>
              Create issue
            </SubmitButton>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export type ProjectEdit = {
  name: string;
  client: string;
  description: string;
  keywords: string[];
  role: string;
  discipline: string;
  scope: string;
};

export function EditProjectDialog({
  open,
  onOpenChange,
  project,
  pending,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  project: ProjectDetail;
  pending: boolean;
  onSubmit: (input: ProjectEdit) => void;
}) {
  const [form, setForm] = useState({ name: "", client: "", description: "", keywords: "", role: "", discipline: "", scope: "" });
  useEffect(() => {
    if (!open) return;
    setForm({
      name: project.name,
      client: project.client ?? "",
      description: project.description ?? "",
      keywords: (project.keywords ?? []).join(", "),
      role: project.member?.role ?? "",
      discipline: project.member?.discipline ?? "",
      scope: project.member?.current_scope ?? "",
    });
  }, [open, project]);
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
    setForm((f) => ({ ...f, [k]: e.target.value }));

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] max-w-xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="font-display text-xl">Project details</DialogTitle>
          <DialogDescription>
            The code <span className="font-mono text-foreground/80">{project.code}</span> cannot change: correspondence is filed by it.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-5"
          onSubmit={(e) => {
            e.preventDefault();
            if (!form.name.trim() || pending) return;
            onSubmit({
              name: form.name.trim(),
              client: form.client.trim(),
              description: form.description.trim(),
              keywords: Array.from(new Set(form.keywords.split(/[,;\n]+/).map((k) => k.trim()).filter(Boolean))),
              role: form.role.trim(),
              discipline: form.discipline.trim(),
              scope: form.scope.trim(),
            });
          }}
        >
          <fieldset className="space-y-3">
            <legend className="mb-1 text-sm font-medium">Project</legend>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="proj-name">Name</Label>
                <Input id="proj-name" value={form.name} onChange={set("name")} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="proj-client">Client</Label>
                <Input id="proj-client" value={form.client} onChange={set("client")} placeholder="Acme Data Centres" />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="proj-description">Description</Label>
              <Textarea id="proj-description" value={form.description} onChange={set("description")} rows={2} />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="proj-keywords">Keywords</Label>
              <Input id="proj-keywords" value={form.keywords} onChange={set("keywords")} placeholder="cooling, chiller, P-03" />
              <p className="text-xs text-muted-foreground">Separate with commas. Words that tie mail to this project.</p>
            </div>
          </fieldset>
          <fieldset className="space-y-3">
            <legend className="mb-1 text-sm font-medium">Your part in it</legend>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="member-role">Role</Label>
                <Input id="member-role" value={form.role} onChange={set("role")} placeholder="Mechanical engineer" />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="member-discipline">Discipline</Label>
                <Input id="member-discipline" value={form.discipline} onChange={set("discipline")} />
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="member-scope">Current scope</Label>
              <Input id="member-scope" value={form.scope} onChange={set("scope")} />
              <p className="text-xs text-muted-foreground">Helps decide which issues and questions are yours.</p>
            </div>
          </fieldset>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <SubmitButton pending={pending} disabled={!form.name.trim()}>
              Save changes
            </SubmitButton>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

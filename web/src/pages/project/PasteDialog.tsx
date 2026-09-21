import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { toast } from "@/hooks/use-toast";
import { ApiError, createManualItem, listContacts } from "@/lib/auth";
import { cn } from "@/lib/utils";

const CHANNELS = [
  { value: "teams", label: "Teams" },
  { value: "whatsapp", label: "WhatsApp" },
  { value: "sms", label: "SMS" },
  { value: "call", label: "Call" },
  { value: "meeting", label: "Meeting" },
  { value: "note", label: "Note" },
] as const;

export function PasteDialog({
  projectID,
  accessToken,
  onDone,
}: {
  projectID: string;
  accessToken: string;
  onDone: () => Promise<void>;
}) {
  const [channel, setChannel] = useState("teams");
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [occurredAt, setOccurredAt] = useState(() => {
    const d = new Date();
    d.setMinutes(d.getMinutes() - d.getTimezoneOffset());
    return d.toISOString().slice(0, 16);
  });
  const [selectedContacts, setSelectedContacts] = useState<string[]>([]);

  const contactsQuery = useQuery({
    queryKey: ["contacts", accessToken, "paste"],
    queryFn: () => listContacts(accessToken, { limit: 100 }),
  });

  const mutation = useMutation({
    mutationFn: async () => {
      const iso = new Date(occurredAt).toISOString();
      return createManualItem(accessToken, {
        channel,
        occurred_at: iso,
        title: title.trim() || channel,
        body_text: body.trim(),
        project_id: projectID,
        participant_contact_ids: selectedContacts,
      });
    },
    onSuccess: async () => {
      toast({ title: "Correspondence added" });
      await onDone();
    },
    onError: (err) => {
      toast({
        title: "Could not paste",
        description: err instanceof ApiError ? err.message : "Please try again.",
        variant: "destructive",
      });
    },
  });

  return (
    <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>Paste correspondence</DialogTitle>
        <DialogDescription>
          Add a Teams, WhatsApp, or other note to this project timeline. Body text is kept as
          evidence and cannot be edited later.
        </DialogDescription>
      </DialogHeader>
      <div className="space-y-3">
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground">Channel</label>
          <Select value={channel} onValueChange={setChannel}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {CHANNELS.map((c) => (
                <SelectItem key={c.value} value={c.value}>
                  {c.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground" htmlFor="occurred">
            When
          </label>
          <Input
            id="occurred"
            type="datetime-local"
            value={occurredAt}
            onChange={(e) => setOccurredAt(e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground" htmlFor="paste-title">
            Title
          </label>
          <Input
            id="paste-title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Optional short title"
          />
        </div>
        <div className="space-y-1">
          <label className="text-xs text-muted-foreground" htmlFor="paste-body">
            Body
          </label>
          <Textarea
            id="paste-body"
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={6}
            placeholder="Paste the message text…"
          />
        </div>
        {(contactsQuery.data?.length ?? 0) > 0 ? (
          <div className="space-y-2">
            <p className="text-xs text-muted-foreground">Participants (optional)</p>
            <ul className="max-h-32 space-y-1 overflow-y-auto text-sm">
              {contactsQuery.data!.map((c) => {
                const checked = selectedContacts.includes(c.id);
                return (
                  <li key={c.id}>
                    <label className="flex cursor-pointer items-center gap-2">
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={() =>
                          setSelectedContacts((prev) =>
                            checked ? prev.filter((x) => x !== c.id) : [...prev, c.id],
                          )
                        }
                      />
                      <span className={cn(checked && "font-medium")}>{c.display_name}</span>
                    </label>
                  </li>
                );
              })}
            </ul>
          </div>
        ) : null}
        <Button
          className="w-full"
          disabled={!body.trim() || mutation.isPending}
          onClick={() => mutation.mutate()}
        >
          {mutation.isPending ? "Saving…" : "Add to timeline"}
        </Button>
      </div>
    </DialogContent>
  );
}

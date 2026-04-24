import { PageHeader } from "@/components/PageHeader";

export default function SettingsPage() {
  return (
    <div className="space-y-6">
      <PageHeader
        eyebrow="Configuration"
        title="Settings"
        description="LLM endpoint, schedules, retention, and category vocabulary."
      />
      <div className="grid gap-4 md:grid-cols-2">
        {[
          { label: "LLM endpoint", value: "http://localhost:1234/v1" },
          { label: "Model", value: "llama-3.1-8b-instruct" },
          { label: "Sync interval", value: "Every 10 minutes" },
          { label: "Daily summary", value: "07:30 local" },
          { label: "Raw mail retention", value: "90 days" },
          { label: "Summary retention", value: "365 days" },
        ].map((s) => (
          <div key={s.label} className="surface-card p-5">
            <p className="text-[10px] uppercase tracking-widest text-muted-foreground">
              {s.label}
            </p>
            <p className="mt-1 font-display text-lg">{s.value}</p>
          </div>
        ))}
      </div>
      <p className="pt-4 text-xs text-muted-foreground">
        Settings are display-only in this UI mock.
      </p>
    </div>
  );
}

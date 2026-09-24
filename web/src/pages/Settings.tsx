import { useSearchParams } from "react-router-dom";
import { PageHeader } from "@/components/PageHeader";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { CategoriesSettings } from "@/components/settings/CategoriesSettings";
import { SummarySettings } from "@/components/settings/SummarySettings";
import { ScheduleSettings } from "@/components/settings/ScheduleSettings";

const tabs = ["categories", "summaries", "schedules"] as const;
type Tab = (typeof tabs)[number];

export default function SettingsPage() {
  // The tab lives in the URL so other pages can link to it and Back works.
  const [params, setParams] = useSearchParams();
  const requested = params.get("tab");
  const tab: Tab = tabs.includes(requested as Tab) ? (requested as Tab) : "categories";

  return (
    <div className="space-y-8">
      <PageHeader
        eyebrow="Configuration"
        title="Settings"
        description="How Automata sorts your mail, what it summarises, and when it runs on its own."
      />
      <Tabs value={tab} onValueChange={(v) => setParams({ tab: v }, { replace: true })} className="space-y-6">
        <TabsList>
          <TabsTrigger value="categories">Categories</TabsTrigger>
          <TabsTrigger value="summaries">Summaries</TabsTrigger>
          <TabsTrigger value="schedules">Schedules</TabsTrigger>
        </TabsList>
        <TabsContent value="categories">
          <CategoriesSettings />
        </TabsContent>
        <TabsContent value="summaries">
          <SummarySettings />
        </TabsContent>
        <TabsContent value="schedules">
          <ScheduleSettings />
        </TabsContent>
      </Tabs>
    </div>
  );
}

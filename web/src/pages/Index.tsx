import { AppShell } from "@/components/AppShell";
import AssistantHomePage from "./AssistantHome";

const Index = () => (
  <AppShell>
    {({ accountFilter }) => <AssistantHomePage accountFilter={accountFilter} />}
  </AppShell>
);

export default Index;

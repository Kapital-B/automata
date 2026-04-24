import { AppShell } from "@/components/AppShell";
import ChatPage from "./Chat";

const Index = () => (
  <AppShell>
    {({ accountFilter }) => <ChatPage accountFilter={accountFilter} />}
  </AppShell>
);

export default Index;

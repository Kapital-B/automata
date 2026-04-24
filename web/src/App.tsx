import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Route, Routes } from "react-router-dom";
import { Toaster as Sonner } from "@/components/ui/sonner";
import { Toaster } from "@/components/ui/toaster";
import { TooltipProvider } from "@/components/ui/tooltip";
import { AppShell } from "@/components/AppShell";
import Index from "./pages/Index.tsx";
import NotFound from "./pages/NotFound.tsx";
import TodayPage from "./pages/Today";
import InboxPage from "./pages/Inbox";
import AccountsPage from "./pages/Accounts";
import RulesPage from "./pages/Rules";
import DraftsPage from "./pages/Drafts";
import RunsPage from "./pages/Runs";
import SettingsPage from "./pages/Settings";
import LoginPage from "./pages/Login";
import RegisterPage from "./pages/Register";

const queryClient = new QueryClient();

const App = () => (
  <QueryClientProvider client={queryClient}>
    <TooltipProvider>
      <Toaster />
      <Sonner />
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<Index />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
          <Route
            path="/today"
            element={<AppShell>{(ctx) => <TodayPage {...ctx} />}</AppShell>}
          />
          <Route
            path="/inbox"
            element={<AppShell>{(ctx) => <InboxPage {...ctx} />}</AppShell>}
          />
          <Route
            path="/drafts"
            element={<AppShell>{(ctx) => <DraftsPage {...ctx} />}</AppShell>}
          />
          <Route
            path="/rules"
            element={<AppShell>{(ctx) => <RulesPage {...ctx} />}</AppShell>}
          />
          <Route
            path="/accounts"
            element={<AppShell>{() => <AccountsPage />}</AppShell>}
          />
          <Route
            path="/runs"
            element={<AppShell>{(ctx) => <RunsPage {...ctx} />}</AppShell>}
          />
          <Route
            path="/settings"
            element={<AppShell>{() => <SettingsPage />}</AppShell>}
          />
          {/* ADD ALL CUSTOM ROUTES ABOVE THE CATCH-ALL "*" ROUTE */}
          <Route path="*" element={<NotFound />} />
        </Routes>
      </BrowserRouter>
    </TooltipProvider>
  </QueryClientProvider>
);

export default App;

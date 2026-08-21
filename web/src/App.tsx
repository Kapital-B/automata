import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Route, Routes } from "react-router-dom";
import { Toaster as Sonner } from "@/components/ui/sonner";
import { Toaster } from "@/components/ui/toaster";
import { TooltipProvider } from "@/components/ui/tooltip";
import { AppShell } from "@/components/AppShell";
import { AuthProvider } from "@/components/auth/AuthProvider";
import { ProtectedRoute } from "@/components/auth/ProtectedRoute";
import Index from "./pages/Index.tsx";
import NotFound from "./pages/NotFound.tsx";
import TodayPage from "./pages/Today";
import InboxPage from "./pages/Inbox";
import AccountsPage from "./pages/Accounts";
import PeoplePage from "./pages/People";
import PersonDetailPage from "./pages/PersonDetail";
import ProjectsPage from "./pages/Projects";
import ProjectDetailPage from "./pages/ProjectDetail";
import UnassignedPage from "./pages/Unassigned";
import RulesPage from "./pages/Rules";
import DraftsPage from "./pages/Drafts";
import RunsPage from "./pages/Runs";
import SettingsPage from "./pages/Settings";
import LoginPage from "./pages/Login";
import RegisterPage from "./pages/Register";
import AuthCallbackPage from "./pages/AuthCallback";
import AuthErrorPage from "./pages/AuthError";
import AccountsConnectedPage from "./pages/AccountsConnected";
import AccountsErrorPage from "./pages/AccountsError";

const queryClient = new QueryClient();

const App = () => (
  <QueryClientProvider client={queryClient}>
    <TooltipProvider>
      <Toaster />
      <Sonner />
      <BrowserRouter>
        <AuthProvider>
          <Routes>
            <Route
              path="/"
              element={
                <ProtectedRoute>
                  <Index />
                </ProtectedRoute>
              }
            />
            <Route path="/login" element={<LoginPage />} />
            <Route path="/register" element={<RegisterPage />} />
            <Route path="/auth/callback" element={<AuthCallbackPage />} />
            <Route path="/auth/error" element={<AuthErrorPage />} />
            <Route path="/accounts/connected" element={<AccountsConnectedPage />} />
            <Route path="/accounts/error" element={<AccountsErrorPage />} />
            <Route
              path="/today"
              element={
                <ProtectedRoute>
                  <AppShell>{(ctx) => <TodayPage {...ctx} />}</AppShell>
                </ProtectedRoute>
              }
            />
            <Route
              path="/inbox"
              element={
                <ProtectedRoute>
                  <AppShell>{(ctx) => <InboxPage {...ctx} />}</AppShell>
                </ProtectedRoute>
              }
            />
            <Route
              path="/people"
              element={
                <ProtectedRoute>
                  <AppShell>{() => <PeoplePage />}</AppShell>
                </ProtectedRoute>
              }
            />
            <Route
              path="/people/:id"
              element={
                <ProtectedRoute>
                  <AppShell>{() => <PersonDetailPage />}</AppShell>
                </ProtectedRoute>
              }
            />
            <Route
              path="/projects"
              element={
                <ProtectedRoute>
                  <AppShell>{() => <ProjectsPage />}</AppShell>
                </ProtectedRoute>
              }
            />
            <Route
              path="/projects/:id"
              element={
                <ProtectedRoute>
                  <AppShell>{() => <ProjectDetailPage />}</AppShell>
                </ProtectedRoute>
              }
            />
            <Route
              path="/unassigned"
              element={
                <ProtectedRoute>
                  <AppShell>{() => <UnassignedPage />}</AppShell>
                </ProtectedRoute>
              }
            />
            <Route
              path="/drafts"
              element={
                <ProtectedRoute>
                  <AppShell>{(ctx) => <DraftsPage {...ctx} />}</AppShell>
                </ProtectedRoute>
              }
            />
            <Route
              path="/rules"
              element={
                <ProtectedRoute>
                  <AppShell>{(ctx) => <RulesPage {...ctx} />}</AppShell>
                </ProtectedRoute>
              }
            />
            <Route
              path="/accounts"
              element={
                <ProtectedRoute>
                  <AppShell>{() => <AccountsPage />}</AppShell>
                </ProtectedRoute>
              }
            />
            <Route
              path="/runs"
              element={
                <ProtectedRoute>
                  <AppShell>{(ctx) => <RunsPage {...ctx} />}</AppShell>
                </ProtectedRoute>
              }
            />
            <Route
              path="/settings"
              element={
                <ProtectedRoute>
                  <AppShell>{() => <SettingsPage />}</AppShell>
                </ProtectedRoute>
              }
            />
            {/* ADD ALL CUSTOM ROUTES ABOVE THE CATCH-ALL "*" ROUTE */}
            <Route path="*" element={<NotFound />} />
          </Routes>
        </AuthProvider>
      </BrowserRouter>
    </TooltipProvider>
  </QueryClientProvider>
);

export default App;

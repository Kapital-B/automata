import { useEffect } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { CheckCircle2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { toast } from "@/hooks/use-toast";

export default function AccountsConnectedPage() {
  const [searchParams] = useSearchParams();
  const accountID = searchParams.get("account_id");
  const nextHref = accountID
    ? `/accounts?connected_account_id=${encodeURIComponent(accountID)}`
    : "/accounts";

  useEffect(() => {
    toast({
      title: "Mailbox connected",
      description: accountID ? `Connected account ${accountID.slice(0, 8)}.` : "Your mailbox is now connected.",
    });
  }, [accountID]);

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4">
      <div className="surface-card max-w-md p-6 text-center">
        <div className="mx-auto grid h-10 w-10 place-items-center rounded-full bg-success/15 text-success">
          <CheckCircle2 className="h-5 w-5" />
        </div>
        <h1 className="mt-4 font-display text-2xl font-semibold">Account connected</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          You can now sync messages and review run history in the dashboard.
        </p>
        <Button asChild className="mt-6">
          <Link to={nextHref}>Go to Accounts</Link>
        </Button>
      </div>
    </div>
  );
}

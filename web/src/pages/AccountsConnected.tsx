import { Navigate, useSearchParams } from "react-router-dom";

// OAuth connects land here. The Accounts page does the confirming — one
// toast, the new account highlighted — the same way a password connect does,
// so this only forwards there.
export default function AccountsConnectedPage() {
  const [searchParams] = useSearchParams();
  const accountID = searchParams.get("account_id");
  const next = accountID ? `/accounts?connected_account_id=${encodeURIComponent(accountID)}` : "/accounts";
  return <Navigate to={next} replace />;
}

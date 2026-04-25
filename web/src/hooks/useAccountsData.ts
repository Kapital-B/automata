import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { listAccounts } from "@/lib/auth";
import { mapAccountsForUi } from "@/lib/accounts";
import { useAuth } from "@/components/auth/AuthProvider";

export function useAccountsData() {
  const { accessToken } = useAuth();
  const query = useQuery({
    queryKey: ["accounts", accessToken],
    queryFn: () => listAccounts(accessToken!),
    enabled: Boolean(accessToken),
  });

  const accounts = useMemo(() => mapAccountsForUi(query.data ?? []), [query.data]);
  return { ...query, accounts };
}

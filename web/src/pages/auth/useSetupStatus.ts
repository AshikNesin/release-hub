import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@groot/shell/lib/api";

/**
 * Whether the admin password still needs setting: `undefined` while loading.
 * A failed request reads as `false`, so sign-in stays reachable.
 */
export function useSetupStatus(): boolean | undefined {
  const { data, isError } = useQuery({
    queryKey: ["auth", "setup"],
    queryFn: () => apiClient.get<{ needsSetup: boolean }>("/auth/setup"),
    staleTime: 0,
    retry: false,
  });
  if (isError) return false;
  return data?.needsSetup;
}

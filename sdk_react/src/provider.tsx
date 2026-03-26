import { createContext, useContext } from "react";
import type { PropsWithChildren } from "react";
import type { AYBClientLike } from "./types";

const AYBContext = createContext<AYBClientLike | null>(null);

export function AYBProvider({ client, children }: PropsWithChildren<{ client: AYBClientLike }>) {
  return <AYBContext.Provider value={client}>{children}</AYBContext.Provider>;
}

export function useAYBClient(): AYBClientLike {
  const client = useContext(AYBContext);
  if (!client) {
    throw new Error("useAYBClient must be used within AYBProvider");
  }
  return client;
}

export type ProviderInfo = {
  id: string;
  available: boolean;
  model?: string;
  models?: Array<{ id: string; ready: boolean; runtime: string; compute_units?: string }>;
};

export type Capabilities = { providers: ProviderInfo[] };

export type GameSnapshot = {
  width: number;
  height: number;
  seed: number;
  body: number[][];
  food: number[] | null;
  score: number;
  length: number;
  ticks: number;
  alive: boolean;
  won: boolean;
  death_reason?: string;
};

export type SessionSnapshot = {
  session_id: string;
  status: string;
  provider: string;
  model: string;
  fps: number;
  guarded: boolean;
  prompt: string;
  game: GameSnapshot;
};

export type DecisionEvent = {
  probabilities: Record<string, number>;
  proposed: string;
  executed: string;
  safe_directions: string[];
  intervened: boolean;
  dead_end_risk: number;
  food_reachable: number;
  inference_ms: number;
  decision_ms: number;
  provider: string;
  model: string;
};

async function parseJSON<T>(res: Response): Promise<T> {
  const data = await res.json();
  if (!res.ok) {
    const msg = data?.error?.message ?? res.statusText;
    throw new Error(msg);
  }
  return data as T;
}

export async function getHealth(): Promise<{ status: string; version: string }> {
  return parseJSON(await fetch("/api/v1/health"));
}

export async function getCapabilities(): Promise<Capabilities> {
  return parseJSON(await fetch("/api/v1/capabilities"));
}

export async function createSession(body: {
  provider: string;
  fps: number;
  seed: number;
  guarded: boolean;
  prompt?: string;
}): Promise<{ session_id: string; status: string; snapshot: GameSnapshot; events_url: string }> {
  return parseJSON(
    await fetch("/api/v1/sessions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  );
}

export async function controlSession(
  id: string,
  body: { action: string; fps?: number; seed?: number },
): Promise<{ status: string; snapshot: SessionSnapshot }> {
  return parseJSON(
    await fetch(`/api/v1/sessions/${id}/controls`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  );
}

export async function deleteSession(id: string): Promise<void> {
  await fetch(`/api/v1/sessions/${id}`, { method: "DELETE" });
}

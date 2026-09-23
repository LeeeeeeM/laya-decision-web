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
  prompt: string;
  game: GameSnapshot;
};

export type DecisionEvent = {
  probabilities?: Record<string, number>;
  prob_move?: Record<string, number>;
  prob_action?: Record<string, number>;
  proposed: string;
  executed: string;
  safe_directions?: string[];
  intervened: boolean;
  dead_end_risk?: number;
  food_reachable?: number;
  inference_ms: number;
  decision_ms: number;
  provider: string;
  model: string;
  move?: string;
  action?: string;
};

export type PlatformGame = {
  seed: number;
  camera_x: number;
  view_w: number;
  view_h: number;
  tile_px: number;
  ground_y: number;
  player_x: number;
  player_y: number;
  player_w: number;
  player_h: number;
  facing: number; // +1 right, -1 left
  crouch: boolean;
  grounded: boolean;
  alive: boolean;
  score: number;
  distance: number;
  ticks: number;
  death_reason?: string;
  move: string;
  action: string;
  solids: number[];
  pits: [number, number][];
  enemies: Array<{ x: number; y: number; facing?: number; alive: boolean }>;
  bullets: Array<{ x: number; y: number }>;
  boxes: Array<{ x: number; y: number; hit: boolean }>;
  items: Array<{ x: number; y: number; alive: boolean }>;
};

export type PlatformSnapshot = {
  session_id: string;
  status: string;
  provider: string;
  model: string;
  decision_hz: number;
  mode: string;
  game: PlatformGame;
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

export async function createPlatformSession(body: {
  provider: string;
  seed: number;
  decision_hz: number;
  mode: string;
}): Promise<{ session_id: string; status: string; snapshot: PlatformSnapshot; events_url: string }> {
  return parseJSON(
    await fetch("/api/v1/platform/sessions", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  );
}

export async function controlPlatformSession(
  id: string,
  body: Record<string, unknown>,
): Promise<{ status: string; snapshot: PlatformSnapshot }> {
  return parseJSON(
    await fetch(`/api/v1/platform/sessions/${id}/controls`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  );
}

export async function deletePlatformSession(id: string): Promise<void> {
  await fetch(`/api/v1/platform/sessions/${id}`, { method: "DELETE" });
}

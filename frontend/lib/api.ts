export const API = (path: string) => `${process.env.NEXT_PUBLIC_API_URL || ""}${path}`;

export async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const r = await fetch(API(path), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!r.ok) throw new Error(`${r.status} ${await r.text()}`);
  return r.json() as Promise<T>;
}

export async function getJSON<T>(path: string): Promise<T> {
  const r = await fetch(API(path));
  if (!r.ok) throw new Error(`${r.status} ${await r.text()}`);
  return r.json() as Promise<T>;
}

export async function getText(path: string): Promise<string> {
  const r = await fetch(API(path));
  if (!r.ok) throw new Error(`${r.status} ${await r.text()}`);
  return r.text();
}

export type RecallVector = {
  id: string;
  body: string;
  kind: string;
  distance: number;
};

export type RecallResponse = {
  vector: RecallVector[];
  lexical: RecallVector[];
};

export type ChatResponse = {
  reply: string;
  run_id: string;
  tokens: number;
  recall: RecallVector[];
  compaction: { fired: boolean; reason: string; score?: number; ratio?: number };
};

export type PlanItem = {
  day_offset: number;
  slot: string;
  method: string;
  duration_min: number;
  notes: string;
};

export type PlanResponse = {
  plan_id: string;
  items: PlanItem[];
};

export type Note = {
  id: string;
  kind: string;
  body: string;
  created_at: string;
};

export type AuditEvent = {
  id: string;
  type: string;
  payload: string;
  ts: string;
};

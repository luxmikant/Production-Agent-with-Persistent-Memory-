"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  AuditEvent, ChatResponse, Note, PlanItem, PlanResponse, RecallVector,
  getJSON, getText, postJSON,
} from "@/lib/api";

type Msg = { role: "user" | "assistant" | "system"; body: string; ts: number };

export default function Page() {
  const [msgs, setMsgs] = useState<Msg[]>([
    { role: "system", body: "Welcome. I’m StillMind. Tell me what you want to work on today (sleep, focus, calm).", ts: Date.now() },
  ]);
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [recall, setRecall] = useState<RecallVector[]>([]);
  const [notes, setNotes] = useState<Note[]>([]);
  const [audit, setAudit] = useState<AuditEvent[]>([]);
  const [plan, setPlan] = useState<PlanResponse | null>(null);
  const [compact, setCompact] = useState<ChatResponse["compaction"] | null>(null);
  const logRef = useRef<HTMLDivElement>(null);

  const refresh = useCallback(async () => {
    try {
      const [n, a] = await Promise.all([
        getJSON<Note[]>("/memory/notes?limit=20"),
        getJSON<AuditEvent[]>("/memory/audit?limit=20"),
      ]);
      setNotes(n);
      setAudit(a);
    } catch (e) { /* ignore */ }
  }, []);

  useEffect(() => { refresh(); }, [refresh]);
  useEffect(() => { logRef.current?.scrollTo({ top: logRef.current.scrollHeight }); }, [msgs]);

  async function send() {
    if (!input.trim() || busy) return;
    const text = input.trim();
    setInput("");
    setMsgs((m) => [...m, { role: "user", body: text, ts: Date.now() }]);
    setBusy(true);
    try {
      const r = await postJSON<ChatResponse>("/chat", { message: text });
      setMsgs((m) => [...m, { role: "assistant", body: r.reply, ts: Date.now() }]);
      setRecall(r.recall || []);
      setCompact(r.compaction || null);
      refresh();
    } catch (e: any) {
      setMsgs((m) => [...m, { role: "assistant", body: "[error] " + (e?.message || "unknown"), ts: Date.now() }]);
    } finally {
      setBusy(false);
    }
  }

  async function createPlan() {
    setBusy(true);
    try {
      const r = await postJSON<PlanResponse>("/plan", {});
      setPlan(r);
      refresh();
    } catch (e: any) {
      setMsgs((m) => [...m, { role: "assistant", body: "[plan error] " + (e?.message || "unknown"), ts: Date.now() }]);
    } finally {
      setBusy(false);
    }
  }

  async function exportCSV() {
    if (!plan) return;
    const csv = await getText(`/plan/csv?plan_id=${plan.plan_id}`);
    const blob = new Blob([csv], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url; a.download = `plan-${plan.plan_id}.csv`; a.click();
    URL.revokeObjectURL(url);
  }

  async function syncSheets() {
    if (!plan) return;
    const r = await postJSON<{ ref: string }>(`/sheets/sync?plan_id=${plan.plan_id}`, {});
    setMsgs((m) => [...m, { role: "system", body: "Sheets synced: " + r.ref, ts: Date.now() }]);
  }

  async function simulateKill() {
    await postJSON("/plan", {}).catch(() => {});
    setMsgs((m) => [...m, { role: "system", body: "Simulated mid-run kill — resuming from CRDB checkpoint.", ts: Date.now() }]);
  }

  return (
    <div className="app">
      <aside className="panel">
        <h3>Memory · Notes</h3>
        <div className="list">
          {notes.length === 0 && <div className="kv">No notes yet.</div>}
          {notes.map((n) => (
            <div className="note-card" key={n.id}>
              <div className="meta">{n.kind} · {new Date(n.created_at).toLocaleString()}</div>
              {n.body}
            </div>
          ))}
        </div>
      </aside>

      <main className="panel" style={{ borderRight: "none" }}>
        <h3>Chat</h3>
        <div className="toolbar">
          <button onClick={createPlan} disabled={busy}>Generate 7-day plan</button>
          <button onClick={exportCSV} disabled={!plan}>Export CSV</button>
          <button onClick={syncSheets} disabled={!plan}>Sync to Sheets (mock)</button>
          <button onClick={simulateKill} disabled={busy}>Simulate resume</button>
          {compact && (
            <span className={`compact-pill ${compact.fired ? "ok" : "skip"}`}>
              compact: {compact.fired ? "fired" : "skipped"} · {compact.reason}
              {compact.fired && compact.score !== undefined && ` · score=${compact.score.toFixed(2)}`}
              {compact.fired && compact.ratio !== undefined && ` · ratio=${compact.ratio.toFixed(2)}`}
            </span>
          )}
        </div>
        <div className="chat-log" ref={logRef}>
          {msgs.map((m, i) => (
            <div className={`bubble ${m.role}`} key={i}>{m.body}</div>
          ))}
        </div>
        <div className="input">
          <input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && send()}
            placeholder="Type a message…"
            disabled={busy}
          />
          <button onClick={send} disabled={busy || !input.trim()}>Send</button>
        </div>
      </main>

      <aside className="panel right">
        <h3>Recall · Audit</h3>
        <div className="list">
          {recall.length > 0 && (
            <>
              <div className="kv" style={{ marginBottom: 6 }}><b>Recalled this turn</b></div>
              {recall.map((r) => (
                <div className="note-card" key={r.id}>
                  <div className="meta">{r.kind} · d={r.distance.toFixed(3)}</div>
                  {r.body}
                </div>
              ))}
            </>
          )}
          {plan && (
            <>
              <div className="kv" style={{ margin: "10px 0 6px" }}><b>7-day plan (mock sheet)</b></div>
              <table className="sheet-table">
                <thead><tr><th>day</th><th>slot</th><th>method</th><th>min</th><th>notes</th></tr></thead>
                <tbody>
                  {plan.items.map((it: PlanItem, i: number) => (
                    <tr key={i}>
                      <td>{it.day_offset}</td>
                      <td>{it.slot}</td>
                      <td>{it.method}</td>
                      <td>{it.duration_min}</td>
                      <td>{it.notes}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}
          <div className="kv" style={{ margin: "10px 0 6px" }}><b>Audit trail</b></div>
          {audit.map((a) => (
            <div className="note-card" key={a.id}>
              <div className="meta">{a.type} · {new Date(a.ts).toLocaleString()}</div>
              {a.payload}
            </div>
          ))}
        </div>
      </aside>
    </div>
  );
}

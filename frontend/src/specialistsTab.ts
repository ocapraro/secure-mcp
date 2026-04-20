import { apiUrl } from "./apiBase";

type SpecialistInfo = {
  name: string;
  resume: string;
  plugin_count: number;
};

type SpecialistLog = {
  id: number;
  created_at: string;
  specialist: string;
  task: string;
  script: string;
  ok: boolean;
  output: string;
  error: string;
};

function safeText(v: unknown): string {
  return typeof v === "string" ? v : "";
}

function formatTs(ts: string): string {
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return ts;
  return d.toLocaleString();
}

async function fetchSpecialists(): Promise<SpecialistInfo[]> {
  const res = await fetch(apiUrl("/api/specialists"));
  if (!res.ok) throw new Error(`Failed to list specialists (${res.status})`);
  const data = (await res.json()) as SpecialistInfo[];
  return Array.isArray(data) ? data : [];
}

async function fetchLogs(specialist: string): Promise<SpecialistLog[]> {
  const qs = specialist ? `?specialist=${encodeURIComponent(specialist)}` : "";
  const res = await fetch(apiUrl(`/api/specialist-logs${qs}`));
  if (!res.ok) throw new Error(`Failed to fetch logs (${res.status})`);
  const data = (await res.json()) as SpecialistLog[];
  return Array.isArray(data) ? data : [];
}

function renderSpecialistChips(listEl: HTMLElement, specialists: SpecialistInfo[], selected: string, onSelect: (name: string) => void) {
  listEl.replaceChildren();

  const allBtn = document.createElement("button");
  allBtn.type = "button";
  allBtn.className = `spec-chip${selected === "" ? " is-active" : ""}`;
  allBtn.textContent = "All specialists";
  allBtn.addEventListener("click", () => onSelect(""));
  listEl.appendChild(allBtn);

  for (const s of specialists) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = `spec-chip${selected === s.name ? " is-active" : ""}`;
    btn.title = safeText(s.resume);
    btn.textContent = `${safeText(s.name)} (${s.plugin_count})`;
    btn.addEventListener("click", () => onSelect(s.name));
    listEl.appendChild(btn);
  }
}

function renderLogs(tableBody: HTMLElement, logs: SpecialistLog[]) {
  tableBody.replaceChildren();

  if (logs.length === 0) {
    const tr = document.createElement("tr");
    tr.className = "spec-log-empty-row";
    tr.innerHTML = `<td colspan="6">No logs yet.</td>`;
    tableBody.appendChild(tr);
    return;
  }

  for (const l of logs) {
    const tr = document.createElement("tr");
    tr.className = l.ok ? "spec-log-row ok" : "spec-log-row fail";

    const outputText = l.error ? safeText(l.error) : safeText(l.output);
    const escaped = outputText
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;");
    const preview = (outputText || "(empty)")
      .replace(/\s+/g, " ")
      .trim()
      .slice(0, 120);
    const escapedPreview = preview
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;");

    tr.innerHTML = `
      <td class="spec-cell-time">${formatTs(l.created_at)}</td>
      <td class="spec-cell-specialist">${safeText(l.specialist)}</td>
      <td class="spec-cell-task">${safeText(l.task)}</td>
      <td class="spec-cell-script">${safeText(l.script)}</td>
      <td class="spec-cell-status"><span class="spec-badge ${l.ok ? "ok" : "fail"}">${l.ok ? "OK" : "FAIL"}</span></td>
      <td class="spec-cell-output">
        <details class="spec-output-panel">
          <summary>
            <span class="spec-output-label">${l.ok ? "Result" : "Error"}</span>
            <span class="spec-output-preview">${escapedPreview || "(empty)"}</span>
          </summary>
          <pre>${escaped || "(empty)"}</pre>
        </details>
      </td>
    `;

    tableBody.appendChild(tr);
  }
}

export function initSpecialistsTab() {
  const root = document.querySelector<HTMLElement>("#specialistsView");
  if (!root) return { refresh: async () => {} };

  const chipsEl = root.querySelector<HTMLElement>("#specialistsFilterChips");
  const tbodyEl = root.querySelector<HTMLElement>("#specialistsLogsBody");
  const refreshEl = root.querySelector<HTMLButtonElement>("#specialistsRefresh");
  const statusEl = root.querySelector<HTMLElement>("#specialistsStatus");

  if (!chipsEl || !tbodyEl || !refreshEl || !statusEl) {
    return { refresh: async () => {} };
  }

  let selected = "";
  let specialists: SpecialistInfo[] = [];

  async function refresh() {
    statusEl.textContent = "Loading specialists...";
    refreshEl.disabled = true;
    try {
      specialists = await fetchSpecialists();
      renderSpecialistChips(chipsEl, specialists, selected, async (name) => {
        selected = name;
        renderSpecialistChips(chipsEl, specialists, selected, async (n) => {
          selected = n;
          await refresh();
        });
        await refresh();
      });

      statusEl.textContent = "Loading logs...";
      const logs = await fetchLogs(selected);
      renderLogs(tbodyEl, logs);
      statusEl.textContent = `Showing ${logs.length} log${logs.length === 1 ? "" : "s"}${selected ? ` for ${selected}` : ""}.`;
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to load specialists logs";
      statusEl.textContent = msg;
      renderLogs(tbodyEl, []);
    } finally {
      refreshEl.disabled = false;
    }
  }

  refreshEl.addEventListener("click", () => {
    void refresh();
  });

  return { refresh };
}

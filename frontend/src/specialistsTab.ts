import { apiUrl } from "./apiBase";
import {
  fetchSecretRequests,
  fetchSecrets,
  removeSecret,
  upsertSecret,
  type SecretItem,
  type SecretRequestItem,
} from "./sessionApi";

type SpecialistInfo = {
  name: string;
  version?: string;
  resume: string;
  plugin_count: number;
  integrity_changed?: boolean;
  expected_source_hash?: string;
  current_source_hash?: string;
  specialist_directory?: string;
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

function formatMs(ms: number): string {
  const d = new Date(ms);
  if (Number.isNaN(d.getTime())) return "";
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
    if (s.integrity_changed) {
      btn.classList.add("integrity-failed");
    }
    btn.title = safeText(s.resume);
    const version = (s.version ?? "").trim();
    const base = safeText(s.name);
    btn.textContent = version ? `${base} v${version}` : base;
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

function renderSecretsTable(
  tbody: HTMLElement,
  requests: SecretRequestItem[],
  secretsByName: Map<string, SecretItem>,
  onSave: (name: string, value: string) => Promise<void>,
  onClear: (name: string) => Promise<void>,
) {
  tbody.replaceChildren();

  if (requests.length === 0) {
    const tr = document.createElement("tr");
    tr.className = "secret-empty-row";
    tr.innerHTML = `<td colspan="7">No plugin secret requests were declared in specialist bio.xml files.</td>`;
    tbody.appendChild(tr);
    return;
  }

  for (const req of requests) {
    const saved = secretsByName.get(req.name) ?? null;

    const tr = document.createElement("tr");
    tr.className = "secret-row";

    const nameTd = document.createElement("td");
    nameTd.className = "secret-cell-name";
    nameTd.textContent = req.name;

    const descTd = document.createElement("td");
    descTd.className = "secret-cell-desc";
    descTd.textContent = req.description || "(no description)";

    const usedByTd = document.createElement("td");
    usedByTd.className = "secret-cell-usedby";
    usedByTd.textContent = req.specialists.length > 0 ? req.specialists.join(", ") : "(unknown)";

    const stateTd = document.createElement("td");
    stateTd.className = "secret-cell-state";
    const badge = document.createElement("span");
    badge.className = `secret-state-badge ${saved?.hasValue ? "set" : "missing"}`;
    badge.textContent = saved?.hasValue ? "Configured" : req.required ? "Required" : "Optional";
    stateTd.appendChild(badge);

    const valueTd = document.createElement("td");
    valueTd.className = "secret-cell-value";
    const input = document.createElement("input");
    input.type = "password";
    input.className = "secret-input";
    input.placeholder = saved?.hasValue ? "Enter new value to rotate" : "Enter secret value";
    input.autocomplete = "off";

    const saveBtn = document.createElement("button");
    saveBtn.type = "button";
    saveBtn.className = "secret-save";
    saveBtn.textContent = saved?.hasValue ? "Update" : "Save";
    saveBtn.addEventListener("click", () => {
      void onSave(req.name, input.value.trim());
    });

    const valueWrap = document.createElement("div");
    valueWrap.className = "secret-edit-wrap";
    valueWrap.append(input, saveBtn);
    valueTd.appendChild(valueWrap);

    const updatedTd = document.createElement("td");
    updatedTd.className = "secret-cell-updated";
    updatedTd.textContent = saved ? formatMs(saved.updatedAt) : "-";

    const actionsTd = document.createElement("td");
    actionsTd.className = "secret-cell-actions";
    const delBtn = document.createElement("button");
    delBtn.type = "button";
    delBtn.className = "secret-delete";
    delBtn.textContent = "Clear";
    delBtn.disabled = !saved?.hasValue;
    delBtn.addEventListener("click", () => {
      void onClear(req.name);
    });
    actionsTd.appendChild(delBtn);

    tr.append(nameTd, descTd, usedByTd, stateTd, valueTd, updatedTd, actionsTd);
    tbody.appendChild(tr);
  }
}

export function initSpecialistsTab() {
  const root = document.querySelector<HTMLElement>("#specialistsView");
  if (!root) return { refresh: async () => {} };

  const chipsEl = root.querySelector<HTMLElement>("#specialistsFilterChips");
  const tbodyEl = root.querySelector<HTMLElement>("#specialistsLogsBody");
  const refreshEl = root.querySelector<HTMLButtonElement>("#specialistsRefresh");
  const statusEl = root.querySelector<HTMLElement>("#specialistsStatus");
  const integrityAlertEl = root.querySelector<HTMLElement>("#specialistsIntegrityAlert");
  const secretsStatusEl = root.querySelector<HTMLElement>("#secretsStatus");
  const secretsTbodyEl = root.querySelector<HTMLElement>("#secretsTableBody");

  if (
    !chipsEl ||
    !tbodyEl ||
    !refreshEl ||
    !statusEl ||
    !integrityAlertEl ||
    !secretsStatusEl ||
    !secretsTbodyEl
  ) {
    return { refresh: async () => {} };
  }

  let selected = "";
  let specialists: SpecialistInfo[] = [];
  let secretRequests: SecretRequestItem[] = [];
  let secretsByName = new Map<string, SecretItem>();

  function setSecretsStatus(text: string, isError = false) {
    secretsStatusEl.textContent = text;
    secretsStatusEl.classList.toggle("is-error", isError);
  }

  async function refreshSecrets() {
    setSecretsStatus("Loading secrets...");
    try {
      const [requests, savedSecrets] = await Promise.all([
        fetchSecretRequests(),
        fetchSecrets(),
      ]);

      secretRequests = requests;
      secretsByName = new Map(savedSecrets.map((s) => [s.name, s]));

      renderSecretsTable(
        secretsTbodyEl,
        secretRequests,
        secretsByName,
        async (name, value) => {
          if (!value) {
            setSecretsStatus("Secret value is required.", true);
            return;
          }
          setSecretsStatus(`Saving ${name}...`);
          try {
            await upsertSecret({ name, value });
            await refreshSecrets();
          } catch (err) {
            const msg = err instanceof Error ? err.message : "Failed to save secret";
            setSecretsStatus(msg, true);
          }
        },
        async (name) => {
          setSecretsStatus(`Clearing ${name}...`);
          try {
            await removeSecret(name);
            await refreshSecrets();
          } catch (err) {
            const msg = err instanceof Error ? err.message : "Failed to clear secret";
            setSecretsStatus(msg, true);
          }
        },
      );

      const configured = secretRequests.filter((r) => secretsByName.get(r.name)?.hasValue).length;
      if (secretRequests.length === 0) {
        setSecretsStatus("No secret requests declared by specialists.");
      } else {
        setSecretsStatus(`Configured ${configured}/${secretRequests.length} requested secrets.`);
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to load secrets";
      setSecretsStatus(msg, true);
      renderSecretsTable(secretsTbodyEl, [], new Map(), async () => {}, async () => {});
    }
  }

  async function refresh() {
    statusEl.textContent = "Loading specialists...";
    refreshEl.disabled = true;
    try {
      specialists = await fetchSpecialists();

      const changed = specialists.filter((s) => Boolean(s.integrity_changed));
      if (changed.length > 0) {
        const names = changed.map((s) => s.name).join(", ");
        integrityAlertEl.hidden = false;
        integrityAlertEl.textContent = `Integrity alert: ${changed.length} specialist source hash mismatch detected (${names}).`;
      } else {
        integrityAlertEl.hidden = true;
        integrityAlertEl.textContent = "";
      }

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

    await refreshSecrets();
  }

  refreshEl.addEventListener("click", () => {
    void refresh();
  });

  return { refresh };
}

const state = {
  catalogs: [],
  selectedCatalog: null,
  activeJobID: null,
  pollTimer: null,
};

const els = {
  form: document.getElementById("job-form"),
  banner: document.getElementById("banner"),
  catalog: document.getElementById("model-catalog"),
  catalogHint: document.getElementById("catalog-hint"),
  model: document.getElementById("model"),
  modelOptions: document.getElementById("model-options"),
  modelHint: document.getElementById("model-hint"),
  profile: document.getElementById("profile"),
  kind: document.getElementById("kind"),
  sourceLanguage: document.getElementById("source-language"),
  targetLanguage: document.getElementById("target-language"),
  source: document.getElementById("source"),
  glossary: document.getElementById("glossary"),
  style: document.getElementById("style"),
  audience: document.getElementById("audience"),
  apiKey: document.getElementById("api-key"),
  submit: document.getElementById("submit"),
  refreshHistory: document.getElementById("refresh-history"),
  statusBadge: document.getElementById("status-badge"),
  jobID: document.getElementById("job-id"),
  jobUpdated: document.getElementById("job-updated"),
  jobSource: document.getElementById("job-source"),
  jobSourceMeta: document.getElementById("job-source-meta"),
  jobResult: document.getElementById("job-result"),
  jobResultMeta: document.getElementById("job-result-meta"),
  history: document.getElementById("history"),
};

function apiHeaders(extra = {}) {
  const headers = { Accept: "application/json", ...extra };
  const key = els.apiKey.value.trim();
  if (key) {
    headers.Authorization = `Bearer ${key}`;
  }
  return headers;
}

async function api(path, options = {}) {
  const response = await fetch(path, {
    ...options,
    headers: apiHeaders(options.headers || {}),
  });
  const text = await response.text();
  let body = null;
  if (text) {
    try {
      body = JSON.parse(text);
    } catch {
      body = { error: text };
    }
  }
  if (!response.ok) {
    const message =
      (body && (body.error || body.message)) ||
      `${response.status} ${response.statusText}`;
    const error = new Error(message);
    error.status = response.status;
    error.body = body;
    throw error;
  }
  return body;
}

function clearBanner() {
  els.banner.hidden = true;
  els.banner.innerHTML = "";
}

function showBanner(kind, title, detail = "") {
  const tone =
    kind === "error"
      ? "mp-alert--error"
      : kind === "success"
        ? "mp-alert--success"
        : kind === "warning"
          ? "mp-alert--warning"
          : "mp-alert--info";
  els.banner.hidden = false;
  els.banner.innerHTML = `
    <div class="mp-alert ${tone}" role="status">
      <div class="mp-alert__content">
        <strong>${escapeHTML(title)}</strong>
        ${detail ? `<p>${escapeHTML(detail)}</p>` : ""}
      </div>
    </div>
  `;
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function fillSelect(select, values, { includeBlank = false, blankLabel = "—" } = {}) {
  const current = select.value;
  select.innerHTML = "";
  if (includeBlank) {
    const option = document.createElement("option");
    option.value = "";
    option.textContent = blankLabel;
    select.append(option);
  }
  for (const value of values) {
    const option = document.createElement("option");
    option.value = value;
    option.textContent = value;
    select.append(option);
  }
  if ([...select.options].some((option) => option.value === current)) {
    select.value = current;
  }
}

function selectedCatalog() {
  return state.catalogs.find((item) => item.id === els.catalog.value) || null;
}

function applyCatalog() {
  const catalog = selectedCatalog();
  state.selectedCatalog = catalog;
  if (!catalog) {
    els.catalogHint.textContent = "No catalog selected.";
    return;
  }

  const kinds = catalog.capabilities?.prompt_kinds?.length
    ? catalog.capabilities.prompt_kinds
    : ["text"];
  fillSelect(els.kind, kinds);
  if (!els.kind.value) {
    els.kind.value = kinds.includes("text") ? "text" : kinds[0];
  }

  const languages = catalog.languages || [];
  fillSelect(els.sourceLanguage, languages, {
    includeBlank: true,
    blankLabel: catalog.capabilities?.requires_source_language
      ? "Required"
      : "Auto / omit",
  });
  fillSelect(els.targetLanguage, languages);
  if (!els.targetLanguage.value && languages.includes("Korean")) {
    els.targetLanguage.value = "Korean";
  }
  if (
    catalog.capabilities?.requires_source_language &&
    !els.sourceLanguage.value &&
    languages.includes("English")
  ) {
    els.sourceLanguage.value = "English";
  }

  const profiles = catalog.profiles?.length ? catalog.profiles : ["official", "deterministic"];
  fillSelect(els.profile, profiles);
  if (!els.profile.value) {
    els.profile.value = profiles[0];
  }

  const aux = catalog.capabilities?.auxiliary_fields || {};
  const notes = [];
  notes.push(
    aux.glossary
      ? "Glossary supported (empty object allowed)."
      : "Glossary required as {}; non-empty terms rejected by this catalog.",
  );
  if (!aux.style) {
    notes.push("Style unsupported.");
  }
  if (!aux.audience) {
    notes.push("Audience unsupported.");
  }
  if (catalog.capabilities?.requires_source_language) {
    notes.push("Source language required.");
  }
  els.catalogHint.textContent = notes.join(" ");
}

function statusBadgeClass(status) {
  switch (status) {
    case "succeeded":
      return "mp-badge mp-badge--success";
    case "failed":
      return "mp-badge mp-badge--error";
    case "running":
      return "mp-badge mp-badge--pending";
    case "queued":
      return "mp-badge mp-badge--info";
    default:
      return "mp-badge mp-badge--pending";
  }
}

function renderJob(job) {
  if (!job) {
    els.statusBadge.className = "mp-badge mp-badge--pending";
    els.statusBadge.textContent = "IDLE";
    els.jobID.textContent = "—";
    els.jobUpdated.textContent = "—";
    els.jobSource.textContent = "No job yet.";
    els.jobSourceMeta.textContent = "empty";
    els.jobResult.textContent = "No job yet.";
    els.jobResultMeta.textContent = "idle";
    return;
  }

  state.activeJobID = job.id;
  els.statusBadge.className = statusBadgeClass(job.status);
  els.statusBadge.textContent = String(job.status || "unknown").toUpperCase();
  els.jobID.textContent = job.id || "—";
  els.jobUpdated.textContent = job.updated_at || job.created_at || "—";

  const sourceText = job.request?.translation?.source ?? "";
  els.jobSource.textContent = sourceText || "(empty source)";
  els.jobSourceMeta.textContent = sourceMeta(sourceText, job.request?.translation);

  if (job.status === "succeeded" && job.result) {
    const translation = job.result.translation ?? "";
    els.jobResult.textContent =
      translation || JSON.stringify(job.result, null, 2);
    els.jobResultMeta.textContent = resultMeta("succeeded", translation);
  } else if (job.status === "failed") {
    els.jobResult.textContent = job.error || "Job failed.";
    els.jobResultMeta.textContent = "failed";
  } else {
    els.jobResult.textContent = JSON.stringify(
      {
        status: job.status,
        model: job.request?.model || job.request?.model_catalog,
        profile: job.request?.profile,
      },
      null,
      2,
    );
    els.jobResultMeta.textContent = String(job.status || "pending");
  }

  for (const button of els.history.querySelectorAll(".ui-history__item")) {
    button.classList.toggle("is-active", button.dataset.jobId === job.id);
  }
}

function sourceMeta(sourceText, translation = {}) {
  const chars = [...sourceText].length;
  const parts = [`${chars} chars`];
  if (translation.source_language) {
    parts.push(String(translation.source_language));
  }
  if (translation.target_language) {
    parts.push(`→ ${translation.target_language}`);
  }
  return parts.join(" · ");
}

function resultMeta(status, text) {
  if (status !== "succeeded") {
    return status;
  }
  return `${[...text].length} chars`;
}

function stopPolling() {
  if (state.pollTimer) {
    clearTimeout(state.pollTimer);
    state.pollTimer = null;
  }
}

function schedulePoll(jobID, delay = 700) {
  stopPolling();
  state.pollTimer = setTimeout(async () => {
    try {
      const job = await api(`/v1/jobs/${encodeURIComponent(jobID)}`);
      renderJob(job);
      if (job.status === "queued" || job.status === "running") {
        schedulePoll(jobID, 900);
      } else if (job.status === "succeeded") {
        showBanner("success", "Job succeeded.", job.id);
        loadHistory();
      } else if (job.status === "failed") {
        showBanner("error", "Job failed.", job.error || job.id);
        loadHistory();
      }
    } catch (error) {
      showBanner("error", "Polling failed.", error.message);
    }
  }, delay);
}

function parseGlossary(raw) {
  let value;
  try {
    value = JSON.parse(raw);
  } catch {
    throw new Error("Glossary must be valid JSON object.");
  }
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new Error("Glossary must be a JSON object. Use {} when empty.");
  }
  for (const [key, entry] of Object.entries(value)) {
    if (typeof entry !== "string") {
      throw new Error(`Glossary entry "${key}" must be a string.`);
    }
  }
  return value;
}

function buildPayload() {
  const glossary = parseGlossary(els.glossary.value.trim() || "{}");
  const translation = {
    source: els.source.value,
    target_language: els.targetLanguage.value,
    kind: els.kind.value || "text",
    glossary,
  };
  if (els.sourceLanguage.value) {
    translation.source_language = els.sourceLanguage.value;
  }
  if (els.style.value.trim()) {
    translation.style = els.style.value.trim();
  }
  if (els.audience.value.trim()) {
    translation.audience = els.audience.value.trim();
  }

  const payload = {
    model_catalog: els.catalog.value,
    profile: els.profile.value || "official",
    translation,
  };
  if (els.model.value.trim()) {
    payload.model = els.model.value.trim();
  }
  return payload;
}

async function loadCatalogs() {
  const data = await api("/v1/model-catalogs");
  state.catalogs = data.model_catalogs || [];
  fillSelect(
    els.catalog,
    state.catalogs.map((item) => item.id),
  );
  if (!els.catalog.value && state.catalogs.length) {
    const preferred =
      state.catalogs.find((item) => item.id.startsWith("hymt2-")) || state.catalogs[0];
    els.catalog.value = preferred.id;
  }
  applyCatalog();
}

function extractModelIDs(payload) {
  const rows = Array.isArray(payload?.data)
    ? payload.data
    : Array.isArray(payload)
      ? payload
      : [];
  const ids = [];
  for (const row of rows) {
    if (typeof row === "string" && row.trim()) {
      ids.push(row.trim());
      continue;
    }
    if (row && typeof row.id === "string" && row.id.trim()) {
      ids.push(row.id.trim());
    }
  }
  return [...new Set(ids)].sort((a, b) => a.localeCompare(b));
}

function fillModelOptions(ids) {
  els.modelOptions.innerHTML = "";
  for (const id of ids) {
    const option = document.createElement("option");
    option.value = id;
    els.modelOptions.append(option);
  }
}

async function loadModels() {
  try {
    const data = await api("/v1/models");
    const ids = extractModelIDs(data);
    fillModelOptions(ids);
    els.modelHint.textContent = ids.length
      ? `${ids.length} models from provider. Type freely or pick one.`
      : "Provider returned no models. Type a server alias or leave empty.";
  } catch (error) {
    fillModelOptions([]);
    els.modelHint.textContent = `Provider models unavailable (${error.message}). Type a server alias or leave empty.`;
  }
}

function renderHistory(jobs) {
  if (!jobs?.length) {
    els.history.innerHTML = `<p class="mp-text--muted">No jobs loaded.</p>`;
    return;
  }
  els.history.innerHTML = "";
  for (const job of jobs) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "ui-history__item";
    button.dataset.jobId = job.id;
    if (job.id === state.activeJobID) {
      button.classList.add("is-active");
    }
    button.innerHTML = `
      <div class="ui-history__top">
        <span class="ui-history__id">${escapeHTML(job.id)}</span>
        <span class="${statusBadgeClass(job.status)}">${escapeHTML(
          String(job.status || "?").toUpperCase(),
        )}</span>
      </div>
      <div class="ui-history__meta">
        ${escapeHTML(job.request?.model_catalog || "—")}
        ·
        ${escapeHTML(job.updated_at || job.created_at || "")}
      </div>
    `;
    button.addEventListener("click", () => {
      renderJob(job);
      if (job.status === "queued" || job.status === "running") {
        schedulePoll(job.id, 0);
      } else {
        stopPolling();
      }
    });
    els.history.append(button);
  }
}

async function loadHistory() {
  try {
    const data = await api("/v1/jobs?limit=20");
    renderHistory(data.jobs || []);
  } catch (error) {
    els.history.innerHTML = `<p class="mp-text--muted">${escapeHTML(error.message)}</p>`;
  }
}

els.catalog.addEventListener("change", applyCatalog);

els.refreshHistory.addEventListener("click", () => {
  loadHistory().catch((error) => showBanner("error", "History failed.", error.message));
});

els.form.addEventListener("submit", async (event) => {
  event.preventDefault();
  clearBanner();
  els.submit.disabled = true;
  try {
    const payload = buildPayload();
    const job = await api("/v1/jobs", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    renderJob(job);
    showBanner("info", "Job queued.", job.id);
    schedulePoll(job.id, 400);
    await loadHistory();
  } catch (error) {
    showBanner("error", "Could not queue job.", error.message);
  } finally {
    els.submit.disabled = false;
  }
});

els.apiKey.addEventListener("change", () => {
  loadModels();
});

async function boot() {
  try {
    await loadCatalogs();
    await Promise.all([loadModels(), loadHistory()]);
  } catch (error) {
    showBanner("error", "Console bootstrap failed.", error.message);
    els.catalogHint.textContent = error.message;
  }
}

boot();

"use strict";

const csrf = document.querySelector('meta[name="csrf-token"]').content;
const maxUploadMeta = document.querySelector('meta[name="max-upload"]');
const maxUpload = Number(maxUploadMeta ? maxUploadMeta.content : 0) || 0;
const appName = document.querySelector(".brand-name").textContent.trim() || "HeimDrive";

const el = {
  backButton: document.getElementById("backButton"),
  breadcrumbs: document.getElementById("breadcrumbs"),
  fileList: document.getElementById("fileList"),
  loading: document.getElementById("loading"),
  emptyState: document.getElementById("emptyState"),
  emptyTitle: document.querySelector("#emptyState h2"),
  emptyText: document.querySelector("#emptyState p"),
  fileInput: document.getElementById("fileInput"),
  uploadButton: document.getElementById("uploadButton"),
  newFolderButton: document.getElementById("newFolderButton"),
  dropOverlay: document.getElementById("dropOverlay"),
  dropFolderName: document.getElementById("dropFolderName"),
  transferPanel: document.getElementById("transferPanel"),
  transferTitle: document.getElementById("transferTitle"),
  transferSubtitle: document.getElementById("transferSubtitle"),
  transferProgress: document.getElementById("transferProgress"),
  transferStatus: document.getElementById("transferStatus"),
  transferClose: document.getElementById("transferClose"),
  cancelUpload: document.getElementById("cancelUpload"),
  actionDialog: document.getElementById("actionDialog"),
  actionTitle: document.getElementById("actionTitle"),
  actionOpen: document.getElementById("actionOpen"),
  actionPreview: document.getElementById("actionPreview"),
  actionDownload: document.getElementById("actionDownload"),
  actionRename: document.getElementById("actionRename"),
  actionDelete: document.getElementById("actionDelete"),
  nameDialog: document.getElementById("nameDialog"),
  nameForm: document.getElementById("nameForm"),
  nameTitle: document.getElementById("nameTitle"),
  nameInput: document.getElementById("nameInput"),
  nameError: document.getElementById("nameError"),
  nameSubmit: document.getElementById("nameSubmit"),
  deleteDialog: document.getElementById("deleteDialog"),
  deleteText: document.getElementById("deleteText"),
  deleteError: document.getElementById("deleteError"),
  deleteConfirm: document.getElementById("deleteConfirm"),
  previewDialog: document.getElementById("previewDialog"),
  previewTitle: document.getElementById("previewTitle"),
  previewBody: document.getElementById("previewBody"),
  toasts: document.getElementById("toasts"),
};

const state = {
  path: "",
  entries: [],
  loadController: null,
  actionEntry: null,
  nameMode: null,
  deleteEntry: null,
  upload: null,
};

const imageExtensions = new Set(["jpg", "jpeg", "png", "gif", "webp", "bmp", "avif"]);
const videoExtensions = new Set(["mp4", "m4v", "webm", "mov"]);
const audioExtensions = new Set(["mp3", "m4a", "aac", "ogg", "oga", "wav", "flac"]);
const textExtensions = new Set(["txt", "log", "md", "csv", "json", "xml", "yaml", "yml", "toml", "ini", "conf"]);
const archiveExtensions = new Set(["zip", "tar", "gz", "tgz", "bz2", "xz", "7z", "rar"]);
const collator = new Intl.Collator("de", { numeric: true, sensitivity: "base" });
const dateFormatter = new Intl.DateTimeFormat("de-DE", { dateStyle: "medium", timeStyle: "short" });

function extension(name) {
  const dot = name.lastIndexOf(".");
  return dot > 0 ? name.slice(dot + 1).toLowerCase() : "";
}

function previewable(name) {
  const ext = extension(name);
  return imageExtensions.has(ext) || videoExtensions.has(ext) || audioExtensions.has(ext) ||
    textExtensions.has(ext) || ext === "pdf";
}

function kindOf(entry) {
  if (entry.isDir) return "folder";
  const ext = extension(entry.name);
  if (imageExtensions.has(ext) || ext === "svg") return "image";
  if (videoExtensions.has(ext) || ["mkv", "avi"].includes(ext)) return "video";
  if (audioExtensions.has(ext)) return "audio";
  if (ext === "pdf") return "pdf";
  if (archiveExtensions.has(ext)) return "archive";
  if (textExtensions.has(ext)) return "text";
  return "file";
}

function iconFor(kind) {
  switch (kind) {
    case "folder": return "folder";
    case "image": return "image";
    case "video": return "video";
    case "audio": return "audio";
    case "pdf": return "pdf";
    case "archive": return "archive";
    case "text": return "file";
    default: return "file";
  }
}

function makeIcon(name) {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("class", "icon");
  svg.setAttribute("aria-hidden", "true");
  const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
  use.setAttribute("href", "#icon-" + name);
  svg.append(use);
  return svg;
}

function normalizeRel(value) {
  return String(value || "")
    .replaceAll("\\", "/")
    .split("/")
    .filter((part) => part && part !== "." && part !== "..")
    .join("/");
}

function joinRel(base, name) {
  return normalizeRel(base ? base + "/" + name : name);
}

function pathFromHash() {
  let raw = location.hash;
  if (raw.startsWith("#/")) raw = raw.slice(2);
  else if (raw.startsWith("#")) raw = raw.slice(1);
  if (!raw) return "";
  return raw.split("/").filter(Boolean).map((part) => {
    try {
      return decodeURIComponent(part);
    } catch {
      return part;
    }
  }).filter((part) => part && part !== "." && part !== "..").join("/");
}

function hashForPath(path) {
  const encoded = normalizeRel(path).split("/").filter(Boolean).map(encodeURIComponent).join("/");
  return "#/" + encoded;
}

function navigate(path) {
  const target = normalizeRel(path);
  const hash = hashForPath(target);
  if (location.hash === hash) {
    void loadDir(target);
  } else {
    location.hash = hash;
  }
}

function parentPath(path) {
  const parts = normalizeRel(path).split("/").filter(Boolean);
  parts.pop();
  return parts.join("/");
}

function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  const digits = value >= 10 || unit === 0 ? 0 : 1;
  return new Intl.NumberFormat("de-DE", { maximumFractionDigits: digits }).format(value) + " " + units[unit];
}

function formatDate(value) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return dateFormatter.format(date);
}

function metaFor(entry) {
  const size = entry.isDir ? "Ordner" : formatBytes(entry.size);
  const date = formatDate(entry.modified);
  return date ? size + " · " + date : size;
}

async function readError(response) {
  const type = response.headers.get("content-type") || "";
  if (type.includes("application/json")) {
    try {
      const data = await response.json();
      if (data && typeof data.error === "string" && data.error) return data.error;
    } catch {
      // Fall through to the status-based message.
    }
  }
  if (response.status === 413) return "Die Datei ist größer als erlaubt.";
  if (response.status === 404) return "Nicht gefunden.";
  if (response.status === 403) return "Kein Zugriff.";
  return "Anfrage fehlgeschlagen (" + response.status + ").";
}

function redirectToLogin() {
  location.href = "/login";
}

async function apiFetch(url, options = {}) {
  const response = await fetch(url, { credentials: "same-origin", ...options });
  if (response.status === 401) {
    redirectToLogin();
    throw new Error("Anmeldung abgelaufen.");
  }
  if (!response.ok) throw new Error(await readError(response));
  return response;
}

function apiURL(pathname, params = {}) {
  const url = new URL(pathname, location.origin);
  for (const [key, value] of Object.entries(params)) url.searchParams.set(key, value);
  return url.pathname + url.search;
}

async function apiJSON(url, method, body) {
  const response = await apiFetch(url, {
    method,
    headers: {
      "Content-Type": "application/json",
      "X-CSRF-Token": csrf,
    },
    body: JSON.stringify(body),
  });
  if (response.status === 204) return null;
  return response.json();
}

function toast(message, type = "info", timeout = 4200) {
  const item = document.createElement("div");
  item.className = "toast " + type;
  item.textContent = message;
  el.toasts.append(item);
  while (el.toasts.children.length > 4) el.toasts.firstElementChild.remove();
  window.setTimeout(() => item.remove(), timeout);
}

function setEmpty(title, text, show) {
  el.emptyTitle.textContent = title;
  el.emptyText.textContent = text;
  el.emptyState.hidden = !show;
}

function updateChrome() {
  const parts = normalizeRel(state.path).split("/").filter(Boolean);
  const crumbs = document.createDocumentFragment();

  const makeSeparator = () => {
    const separator = document.createElement("span");
    separator.className = "crumb-separator";
    separator.textContent = "/";
    separator.setAttribute("aria-hidden", "true");
    return separator;
  };

  if (parts.length === 0) {
    const current = document.createElement("span");
    current.className = "crumb-home";
    current.setAttribute("aria-current", "page");
    current.append(makeIcon("home"));
    crumbs.append(current);
  } else {
    const root = document.createElement("button");
    root.type = "button";
    root.className = "crumb-home";
    root.title = "Hauptordner";
    root.setAttribute("aria-label", "Hauptordner");
    root.append(makeIcon("home"));
    root.addEventListener("click", () => navigate(""));
    crumbs.append(root);

    parts.forEach((part, index) => {
      crumbs.append(makeSeparator());
      const path = parts.slice(0, index + 1).join("/");
      if (index === parts.length - 1) {
        const current = document.createElement("span");
        current.setAttribute("aria-current", "page");
        current.textContent = part;
        current.title = part;
        crumbs.append(current);
      } else {
        const button = document.createElement("button");
        button.type = "button";
        button.textContent = part;
        button.title = part;
        button.addEventListener("click", () => navigate(path));
        crumbs.append(button);
      }
    });
  }
  el.breadcrumbs.replaceChildren(crumbs);
  el.backButton.hidden = parts.length === 0;
  document.title = parts.length ? parts[parts.length - 1] + " · " + appName : appName;
}

function compareEntries(left, right) {
  if (left.isDir !== right.isDir) return left.isDir ? -1 : 1;
  return collator.compare(left.name, right.name);
}

function renderFiles() {
  const sorted = [...state.entries].sort(compareEntries);
  state.entries = sorted;
  el.fileList.replaceChildren();

  if (sorted.length === 0) {
    setEmpty("Dieser Ordner ist leer", "Lade eine Datei hoch oder erstelle einen neuen Ordner.", true);
    return;
  }
  setEmpty("Dieser Ordner ist leer", "Lade eine Datei hoch oder erstelle einen neuen Ordner.", false);

  const fragment = document.createDocumentFragment();
  sorted.forEach((entry, index) => {
    const row = document.createElement("li");
    row.className = "file-row";
    row.dataset.index = String(index);

    const main = document.createElement("button");
    main.type = "button";
    main.className = "entry-main";
    main.dataset.action = "open";
    main.setAttribute("aria-label", (entry.isDir ? "Ordner " : "Datei ") + entry.name);

    const icon = document.createElement("span");
    const kind = kindOf(entry);
    icon.className = "entry-icon " + kind;
    icon.append(makeIcon(iconFor(kind)));

    const nameBox = document.createElement("span");
    nameBox.className = "entry-name";
    const strong = document.createElement("strong");
    strong.textContent = entry.name;
    const small = document.createElement("small");
    small.textContent = metaFor(entry);
    nameBox.append(strong, small);
    main.append(icon, nameBox);

    const size = document.createElement("span");
    size.className = "entry-size";
    size.textContent = entry.isDir ? "—" : formatBytes(entry.size);

    const date = document.createElement("time");
    date.className = "entry-date";
    date.textContent = formatDate(entry.modified);
    if (entry.modified) date.dateTime = entry.modified;

    const actions = document.createElement("div");
    actions.className = "entry-actions";

    const download = document.createElement("button");
    download.type = "button";
    download.className = "icon-button";
    download.dataset.action = "download";
    download.title = entry.isDir ? "Ordner als ZIP herunterladen" : "Herunterladen";
    download.setAttribute("aria-label", download.title + ": " + entry.name);
    download.append(makeIcon("download"));

    const more = document.createElement("button");
    more.type = "button";
    more.className = "icon-button";
    more.dataset.action = "menu";
    more.title = "Weitere Aktionen";
    more.setAttribute("aria-label", "Weitere Aktionen für " + entry.name);
    more.append(makeIcon("more"));

    actions.append(download, more);
    row.append(main, size, date, actions);
    fragment.append(row);
  });
  el.fileList.append(fragment);
}

async function loadDir(path, options = {}) {
  const target = normalizeRel(path);
  state.path = target;
  updateChrome();
  if (!options.keepScroll) window.scrollTo(0, 0);

  if (state.loadController) state.loadController.abort();
  const controller = new AbortController();
  state.loadController = controller;

  el.loading.hidden = false;
  el.emptyState.hidden = true;
  el.fileList.replaceChildren();

  try {
    const response = await fetch(apiURL("/api/list", { path: target }), {
      credentials: "same-origin",
      signal: controller.signal,
    });
    if (response.status === 401) {
      redirectToLogin();
      return;
    }
    if (!response.ok) {
      const message = await readError(response);
      if (response.status === 404 && target !== "") {
        toast("Dieser Ordner wurde nicht gefunden.", "error");
        navigate(parentPath(target));
        return;
      }
      throw new Error(message);
    }
    const data = await response.json();
    if (controller.signal.aborted) return;
    state.entries = Array.isArray(data.entries) ? data.entries : [];
    renderFiles();
  } catch (error) {
    if (error.name === "AbortError") return;
    setEmpty("Ordner konnte nicht geladen werden", error.message || "Unbekannter Fehler.", true);
    toast(error.message || "Ordner konnte nicht geladen werden.", "error");
  } finally {
    if (state.loadController === controller) {
      el.loading.hidden = true;
      state.loadController = null;
    }
  }
}

function openEntry(entry) {
  if (entry.isDir) {
    navigate(joinRel(state.path, entry.name));
  } else if (previewable(entry.name)) {
    showPreview(entry);
  } else {
    showActions(entry);
  }
}

function downloadEntry(entry) {
  const link = document.createElement("a");
  link.href = apiURL("/api/download", { path: joinRel(state.path, entry.name) });
  link.download = "";
  link.hidden = true;
  document.body.append(link);
  link.click();
  link.remove();
}

function showActions(entry) {
  state.actionEntry = entry;
  el.actionTitle.textContent = entry.name;
  el.actionTitle.title = entry.name;
  el.actionOpen.hidden = !entry.isDir;
  el.actionPreview.hidden = entry.isDir || !previewable(entry.name);
  el.actionDownload.hidden = false;
  el.actionRename.hidden = false;
  el.actionDelete.hidden = false;
  el.actionDialog.showModal();
}

function showPreview(entry) {
  const ext = extension(entry.name);
  el.previewTitle.textContent = entry.name;
  el.previewTitle.title = entry.name;
  el.previewBody.replaceChildren();
  const src = apiURL("/api/view", { path: joinRel(state.path, entry.name) });
  let media;

  if (imageExtensions.has(ext)) {
    media = document.createElement("img");
    media.alt = entry.name;
    media.src = src;
    media.addEventListener("error", () => {
      el.previewBody.replaceChildren();
      const message = document.createElement("p");
      message.className = "preview-message";
      message.textContent = "Die Vorschau konnte nicht geladen werden.";
      el.previewBody.append(message);
    });
  } else if (videoExtensions.has(ext)) {
    media = document.createElement("video");
    media.controls = true;
    media.preload = "metadata";
    media.src = src;
  } else if (audioExtensions.has(ext)) {
    media = document.createElement("audio");
    media.controls = true;
    media.preload = "metadata";
    media.src = src;
  } else if (ext === "pdf" || textExtensions.has(ext)) {
    media = document.createElement("iframe");
    media.title = "Vorschau: " + entry.name;
    media.src = src;
  } else {
    media = document.createElement("p");
    media.className = "preview-message";
    media.textContent = "Für diesen Dateityp gibt es keine Vorschau.";
  }
  el.previewBody.append(media);
  el.previewDialog.showModal();
}

function openNameDialog(mode, entry = null) {
  state.nameMode = { mode, entry, basePath: state.path };
  const renaming = mode === "rename";
  el.nameTitle.textContent = renaming ? "Umbenennen" : "Neuer Ordner";
  el.nameSubmit.textContent = renaming ? "Speichern" : "Erstellen";
  el.nameInput.value = renaming ? entry.name : "";
  el.nameError.hidden = true;
  el.nameError.textContent = "";
  el.nameSubmit.disabled = false;
  el.nameDialog.showModal();
  window.setTimeout(() => {
    el.nameInput.focus();
    el.nameInput.select();
  }, 20);
}

async function submitNameForm(event) {
  event.preventDefault();
  const mode = state.nameMode;
  if (!mode) return;
  const name = el.nameInput.value;
  if (!name.trim()) {
    el.nameError.textContent = "Bitte einen Namen eingeben.";
    el.nameError.hidden = false;
    return;
  }
  el.nameError.hidden = true;
  el.nameSubmit.disabled = true;

  const isRename = mode.mode === "rename";
  const path = isRename ? joinRel(mode.basePath, mode.entry.name) : mode.basePath;
  const endpoint = isRename ? "/api/rename" : "/api/mkdir";
  try {
    const result = await apiJSON(endpoint, "POST", { path, name });
    el.nameDialog.close();
    toast(isRename ? "Umbenannt." : "Ordner erstellt.", "success");
    if (state.path === mode.basePath) {
      if (isRename && result && result.name === name) {
        // No navigation needed: files stay in the same folder.
      }
      await loadDir(state.path, { keepScroll: true });
    }
  } catch (error) {
    el.nameError.textContent = error.message || "Aktion fehlgeschlagen.";
    el.nameError.hidden = false;
  } finally {
    el.nameSubmit.disabled = false;
  }
}

function openDeleteDialog(entry) {
  state.deleteEntry = { entry, basePath: state.path };
  el.deleteText.textContent = (entry.isDir
    ? "Der Ordner „" + entry.name + "“ und sein gesamter Inhalt werden endgültig gelöscht. Das kann nicht rückgängig gemacht werden."
    : "Die Datei „" + entry.name + "“ wird endgültig gelöscht. Das kann nicht rückgängig gemacht werden.");
  el.deleteError.hidden = true;
  el.deleteError.textContent = "";
  el.deleteConfirm.disabled = false;
  el.deleteDialog.showModal();
}

async function confirmDelete() {
  const pending = state.deleteEntry;
  if (!pending) return;
  el.deleteConfirm.disabled = true;
  el.deleteError.hidden = true;
  try {
    const response = await apiFetch(apiURL("/api/delete", {
      path: joinRel(pending.basePath, pending.entry.name),
    }), {
      method: "DELETE",
      headers: { "X-CSRF-Token": csrf },
    });
    if (!response.ok) throw new Error(await readError(response));
    el.deleteDialog.close();
    toast("Gelöscht.", "success");
    if (state.path === pending.basePath) await loadDir(state.path, { keepScroll: true });
  } catch (error) {
    el.deleteError.textContent = error.message || "Löschen fehlgeschlagen.";
    el.deleteError.hidden = false;
  } finally {
    el.deleteConfirm.disabled = false;
  }
}

function updateTransfer(u, label) {
  const ratio = u.totalBytes > 0
    ? Math.min(1, (u.completedBytes + u.currentLoaded) / u.totalBytes)
    : 1;
  el.transferProgress.value = Math.round(ratio * 1000);
  el.transferSubtitle.textContent = label || "";
  if (u.speed > 0) {
    el.transferStatus.textContent = Math.round(ratio * 100) + " % · " + formatBytes(u.speed) + "/s";
  } else {
    el.transferStatus.textContent = Math.round(ratio * 100) + " %";
  }
}

function uploadOne(u, file, index, count) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    u.xhr = xhr;
    u.currentLoaded = 0;
    u.speed = 0;
    u.fileStartedAt = performance.now();

    xhr.open("POST", apiURL("/api/upload", { path: u.targetPath }), true);
    xhr.setRequestHeader("X-CSRF-Token", csrf);

    xhr.upload.addEventListener("progress", (event) => {
      if (u.aborted) return;
      const loaded = event.lengthComputable ? Math.min(event.loaded, file.size) : 0;
      u.currentLoaded = loaded;
      const seconds = (performance.now() - u.fileStartedAt) / 1000;
      if (seconds > 0.25) u.speed = event.loaded / seconds;
      updateTransfer(u, "Datei " + (index + 1) + " von " + count + " · " + file.name);
    });

    xhr.addEventListener("load", () => {
      u.xhr = null;
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve(JSON.parse(xhr.responseText || "{}"));
        } catch {
          resolve({ saved: [] });
        }
        return;
      }
      if (xhr.status === 401) {
        redirectToLogin();
        reject(new Error("Anmeldung abgelaufen."));
        return;
      }
      let message = "Upload fehlgeschlagen (" + xhr.status + ").";
      try {
        const data = JSON.parse(xhr.responseText || "{}");
        if (data && data.error) message = data.error;
      } catch {
        // Keep the status-based message.
      }
      reject(new Error(message));
    });

    xhr.addEventListener("error", () => {
      u.xhr = null;
      reject(new Error("Netzwerkfehler beim Hochladen."));
    });
    xhr.addEventListener("abort", () => {
      u.xhr = null;
      reject(new DOMException("Upload abgebrochen.", "AbortError"));
    });

    const form = new FormData();
    form.append("file", file, file.name);
    xhr.send(form);
  });
}

async function startUploads(fileList) {
  const files = Array.from(fileList || []).filter((file) => file && typeof file.name === "string");
  if (files.length === 0) {
    toast("Keine Dateien ausgewählt.", "error");
    return;
  }
  if (state.upload && !state.upload.done) {
    toast("Es läuft bereits ein Upload. Bitte warte, bis er fertig ist.", "error");
    return;
  }
  if (maxUpload > 0) {
    const tooLarge = files.find((file) => file.size > maxUpload);
    if (tooLarge) {
      toast("„" + tooLarge.name + "“ ist größer als " + formatBytes(maxUpload) + ".", "error", 6000);
      return;
    }
  }

  const u = {
    targetPath: state.path,
    files,
    fileIndex: 0,
    fileCount: files.length,
    totalBytes: files.reduce((sum, file) => sum + file.size, 0),
    completedBytes: 0,
    completedFiles: 0,
    currentLoaded: 0,
    speed: 0,
    fileStartedAt: 0,
    xhr: null,
    aborted: false,
    done: false,
    failed: "",
    saved: [],
  };
  state.upload = u;
  el.transferPanel.hidden = false;
  el.transferTitle.textContent = "Upload";
  el.transferSubtitle.textContent = files.length + (files.length === 1 ? " Datei" : " Dateien");
  el.transferProgress.value = 0;
  el.transferStatus.textContent = "Wird vorbereitet …";
  el.transferClose.hidden = true;
  el.cancelUpload.hidden = false;
  el.cancelUpload.disabled = false;

  for (let index = 0; index < files.length; index++) {
    if (u.aborted) break;
    const file = files[index];
    u.fileIndex = index;
    u.currentLoaded = 0;
    u.speed = 0;
    updateTransfer(u, "Datei " + (index + 1) + " von " + files.length + " · " + file.name);
    try {
      const result = await uploadOne(u, file, index, files.length);
      u.completedFiles++;
      u.completedBytes += file.size;
      u.currentLoaded = 0;
      if (result && Array.isArray(result.saved)) u.saved.push(...result.saved);
    } catch (error) {
      if (u.aborted || error.name === "AbortError") break;
      u.failed = error.message || "Upload fehlgeschlagen.";
      break;
    }
  }

  u.done = true;
  u.xhr = null;
  el.cancelUpload.hidden = true;
  el.transferClose.hidden = false;
  el.transferProgress.value = u.failed || u.aborted ? Math.round(((u.completedBytes + u.currentLoaded) / Math.max(1, u.totalBytes)) * 1000) : 1000;

  if (u.aborted) {
    el.transferTitle.textContent = "Upload abgebrochen";
    el.transferSubtitle.textContent = u.completedFiles + " von " + files.length + " Dateien übertragen";
    el.transferStatus.textContent = "Abgebrochen";
  } else if (u.failed) {
    el.transferTitle.textContent = "Upload fehlgeschlagen";
    el.transferStatus.textContent = u.failed;
  } else {
    el.transferTitle.textContent = "Upload abgeschlossen";
    el.transferSubtitle.textContent = files.length + (files.length === 1 ? " Datei" : " Dateien");
    el.transferStatus.textContent = "Fertig";
  }

  const renamed = u.saved.filter((file) => file.renamed);
  if (renamed.length > 0) {
    const examples = renamed.slice(0, 2).map((file) => "„" + file.original + "“ → „" + file.name + "“").join(", ");
    toast("Namenskonflikt: " + examples + (renamed.length > 2 ? " und weitere" : ""), "info", 7000);
  }

  if (u.saved.length > 0 && state.path === u.targetPath) {
    await loadDir(state.path, { keepScroll: true });
  }
}

function cancelCurrentUpload() {
  const u = state.upload;
  if (!u || u.done) return;
  u.aborted = true;
  el.cancelUpload.disabled = true;
  el.transferStatus.textContent = "Wird abgebrochen …";
  if (u.xhr) u.xhr.abort();
}

function closeTransferPanel() {
  const u = state.upload;
  if (u && !u.done) return;
  el.transferPanel.hidden = true;
  state.upload = null;
}

let dragDepth = 0;

function hasFiles(event) {
  if (!event.dataTransfer) return false;
  return Array.from(event.dataTransfer.types || []).includes("Files");
}

function showDropOverlay(event) {
  const count = event.dataTransfer && event.dataTransfer.items
    ? Array.from(event.dataTransfer.items).filter((item) => item.kind === "file").length
    : 0;
  el.dropFolderName.textContent = count > 0 ? count + (count === 1 ? " Datei" : " Dateien") : "";
  el.dropOverlay.hidden = false;
}

function hideDropOverlay() {
  dragDepth = 0;
  el.dropOverlay.hidden = true;
}

function bindEvents() {
  el.fileList.addEventListener("click", (event) => {
    const row = event.target.closest(".file-row");
    if (!row) return;
    const entry = state.entries[Number(row.dataset.index)];
    if (!entry) return;
    const action = event.target.closest("[data-action]");
    if (action && action.dataset.action === "download") {
      downloadEntry(entry);
      return;
    }
    if (action && action.dataset.action === "menu") {
      showActions(entry);
      return;
    }
    openEntry(entry);
  });

  el.backButton.addEventListener("click", () => navigate(parentPath(state.path)));
  el.uploadButton.addEventListener("click", () => el.fileInput.click());
  el.newFolderButton.addEventListener("click", () => openNameDialog("mkdir"));
  el.fileInput.addEventListener("change", () => {
    const selected = el.fileInput.files;
    startUploads(selected);
    el.fileInput.value = "";
  });

  el.actionOpen.addEventListener("click", () => {
    const entry = state.actionEntry;
    el.actionDialog.close();
    if (entry) openEntry(entry);
  });
  el.actionPreview.addEventListener("click", () => {
    const entry = state.actionEntry;
    el.actionDialog.close();
    if (entry) showPreview(entry);
  });
  el.actionDownload.addEventListener("click", () => {
    const entry = state.actionEntry;
    el.actionDialog.close();
    if (entry) downloadEntry(entry);
  });
  el.actionRename.addEventListener("click", () => {
    const entry = state.actionEntry;
    el.actionDialog.close();
    if (entry) openNameDialog("rename", entry);
  });
  el.actionDelete.addEventListener("click", () => {
    const entry = state.actionEntry;
    el.actionDialog.close();
    if (entry) openDeleteDialog(entry);
  });

  el.nameForm.addEventListener("submit", submitNameForm);
  el.deleteConfirm.addEventListener("click", confirmDelete);
  el.transferClose.addEventListener("click", closeTransferPanel);
  el.cancelUpload.addEventListener("click", cancelCurrentUpload);

  document.querySelectorAll("[data-close]").forEach((button) => {
    button.addEventListener("click", () => {
      const dialog = button.closest("dialog");
      if (dialog) dialog.close();
    });
  });

  document.querySelectorAll("dialog").forEach((dialog) => {
    dialog.addEventListener("click", (event) => {
      if (event.target !== dialog) return;
      const rect = dialog.getBoundingClientRect();
      const outside = event.clientX < rect.left || event.clientX > rect.right ||
        event.clientY < rect.top || event.clientY > rect.bottom;
      if (outside) dialog.close();
    });
  });

  el.previewDialog.addEventListener("close", () => {
    el.previewBody.querySelectorAll("video, audio").forEach((media) => media.pause());
    el.previewBody.replaceChildren();
  });

  window.addEventListener("hashchange", () => {
    void loadDir(pathFromHash());
  });

  window.addEventListener("dragenter", (event) => {
    if (!hasFiles(event)) return;
    event.preventDefault();
    dragDepth++;
    showDropOverlay(event);
  });
  window.addEventListener("dragover", (event) => {
    if (!hasFiles(event)) return;
    event.preventDefault();
    if (event.dataTransfer) event.dataTransfer.dropEffect = "copy";
  });
  window.addEventListener("dragleave", (event) => {
    if (!hasFiles(event)) return;
    event.preventDefault();
    dragDepth = Math.max(0, dragDepth - 1);
    if (dragDepth === 0) hideDropOverlay();
  });
  window.addEventListener("drop", (event) => {
    if (!hasFiles(event)) return;
    event.preventDefault();
    const files = event.dataTransfer ? event.dataTransfer.files : null;
    hideDropOverlay();
    if (files && files.length > 0) startUploads(files);
    else toast("Keine Dateien erkannt. Ordner können hier nicht hochgeladen werden.", "error");
  });
  window.addEventListener("dragend", hideDropOverlay);
  window.addEventListener("beforeunload", (event) => {
    if (state.upload && !state.upload.done) {
      event.preventDefault();
      event.returnValue = "";
    }
  });
  window.addEventListener("pageshow", (event) => {
    if (event.persisted) void loadDir(state.path);
  });
}

function init() {
  bindEvents();
  if (!location.hash) history.replaceState(null, "", "#/");
  void loadDir(pathFromHash());
}

init();

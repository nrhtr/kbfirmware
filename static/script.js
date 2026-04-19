// kbfirmware script.js

// Theme — apply before paint to avoid flash
const html = document.documentElement;
const stored = localStorage.getItem('theme');
if (stored) html.dataset.theme = stored;

document.getElementById('theme-toggle')?.addEventListener('click', () => {
  const isDark = html.dataset.theme === 'dark' ||
    (!html.dataset.theme && window.matchMedia('(prefers-color-scheme: dark)').matches);
  const next = isDark ? 'light' : 'dark';
  html.dataset.theme = next;
  localStorage.setItem('theme', next);
});

// Entry rendering
function esc(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;').replace(/</g, '&lt;')
    .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function renderEntry(e) {
  const revision = e.pcb_revision ? ` <span class="pcb-revision">${esc(e.pcb_revision)}</span>` : '';
  const designer = e.pcb_designer ? `<span class="designer">by ${esc(e.pcb_designer)}</span>` : '';
  const tags = (e.tags || []).map(t => `<span class="tag">${esc(t)}</span>`).join('');
  const tagsHtml = tags ? `<div class="tags">${tags}</div>` : '';
  const files = (e.files || []).map(f => `
    <div class="file-row">
      <a class="file-link" href="/file/${f.id}" download="${esc(f.filename)}">
        <span class="file-tag">${esc(f.file_tag)}</span><span class="file-sep"> / </span><span class="file-name">${esc(f.filename)}</span>
      </a>
      <button class="hash-btn" data-hash="${esc(f.sha256)}" title="Click to copy SHA256"
        aria-label="Copy SHA256 hash">
        <span class="hash-label">SHA256</span>
        <code class="hash-value">${esc(f.sha256.slice(0, 16))}…</code>
      </button>
    </div>`).join('');
  const filesHtml = files ? `<div class="entry-files">${files}</div>` : '';
  const source = e.source_url
    ? `<div class="entry-source">Source: <a href="${esc(e.source_url)}" target="_blank" rel="noopener noreferrer">${esc(e.source_url)}</a></div>`
    : '';
  const notes = e.notes ? `<div class="entry-notes">${esc(e.notes)}</div>` : '';

  return `<article class="entry" data-id="${e.id}">
  <div class="entry-header">
    <div class="entry-title">
      <span class="pcb-name">${esc(e.pcb_name)}${revision}</span>
      <span class="firmware-name">${esc(e.firmware_name)}</span>
    </div>
    <button class="flag-btn" data-entry-id="${e.id}" title="Report an issue with this entry">&#9873; Report issue</button>
  </div>
  <div class="entry-meta">${designer}${tagsHtml}</div>
  ${filesHtml}${source}${notes}
</article>`;
}

// Search state
let allEntries = [];
let searchStrings = [];

function buildSearchString(e) {
  return [
    e.pcb_name, e.pcb_revision, e.pcb_designer,
    e.firmware_name, e.source_url, e.notes,
    ...(e.tags || []),
    ...(e.files || []).map(f => f.file_tag + ' ' + f.filename),
  ].join(' ').toLowerCase();
}

const container = document.getElementById('entries-container');
const noResults = document.getElementById('no-results');
const searchCount = document.getElementById('search-count');
const searchInput = document.getElementById('search');

function applyFilter() {
  const q = searchInput?.value.trim().toLowerCase() ?? '';
  const filtered = q ? allEntries.filter((_, i) => searchStrings[i].includes(q)) : allEntries;
  container.innerHTML = filtered.length
    ? filtered.map(renderEntry).join('')
    : '';
  if (noResults) noResults.classList.toggle('hidden', filtered.length > 0 || !q);
  if (searchCount) {
    searchCount.textContent = q
      ? `${filtered.length} of ${allEntries.length} entries`
      : `${allEntries.length} firmware entries`;
  }
}

// Fetch entries and render
fetch('/api/entries.json', { cache: 'default' })
  .then(r => {
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    return r.json();
  })
  .then(entries => {
    allEntries = entries;
    searchStrings = entries.map(buildSearchString);
    if (searchCount) searchCount.textContent = `${entries.length} firmware entries`;
    applyFilter();
  })
  .catch(() => {
    container.innerHTML = '<p class="loading-msg error">Failed to load entries. Please refresh the page.</p>';
  });

// Restore query from URL on load
const initialQ = new URLSearchParams(location.search).get('q');
if (initialQ && searchInput) searchInput.value = initialQ;

let debounce;
searchInput?.addEventListener('input', () => {
  clearTimeout(debounce);
  debounce = setTimeout(() => {
    const q = searchInput.value.trim();
    const url = q ? '?q=' + encodeURIComponent(q) : location.pathname;
    history.replaceState(null, '', url);
    applyFilter();
  }, 120);
});

// Hash copy
document.addEventListener('click', e => {
  const btn = e.target.closest('.hash-btn');
  if (!btn) return;
  const hash = btn.dataset.hash;
  navigator.clipboard.writeText(hash).then(() => {
    const label = btn.querySelector('.hash-label');
    const prev = label.textContent;
    label.textContent = '✓ Copied';
    btn.classList.add('hash-copied');
    setTimeout(() => { label.textContent = prev; btn.classList.remove('hash-copied'); }, 1500);
  });
});

// Flag dialog
const dialog = document.getElementById('flag-dialog');
let currentEntryId = null;

document.addEventListener('click', e => {
  const btn = e.target.closest('.flag-btn');
  if (btn && dialog) {
    currentEntryId = btn.dataset.entryId;
    document.getElementById('flag-reason').value = '';
    dialog.showModal();
  }
});

dialog?.querySelector('#flag-cancel')?.addEventListener('click', () => dialog.close());

dialog?.querySelector('#flag-submit')?.addEventListener('click', async () => {
  const reason = document.getElementById('flag-reason').value.trim();
  try {
    const res = await fetch(`/flag/${currentEntryId}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ reason }),
    });
    if (res.ok) {
      dialog.close();
      const btn = document.querySelector(`.flag-btn[data-entry-id="${currentEntryId}"]`);
      if (btn) { btn.textContent = '✓ Reported'; btn.disabled = true; }
    } else {
      const j = await res.json().catch(() => ({}));
      alert(j.error || 'Failed to submit report. Please try again.');
    }
  } catch {
    alert('Network error. Please try again.');
  }
});

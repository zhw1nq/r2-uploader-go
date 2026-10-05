/**
 * R2 Uploader & Storage Manager - Client Application (English, Minimalist)
 */

// Application State
const state = {
  activeKey: localStorage.getItem('r2_session_key') || '',
  bucketName: '',
  publicDomain: '',
  appUrl: '',
  rootFolder: 'uploader',
  currentPath: '',
  folders: [],
  files: [],
  filteredFiles: [],
  sessions: [],
  currentTab: 'files-view',
  filterType: 'all',
  searchQuery: '',
  viewMode: 'table'
};

function getAppOrigin() {
  return state.appUrl || window.location.origin;
}

// Utilities
function formatBytes(bytes, decimals = 2) {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}

function escapeHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

function formatDate(dateString) {
  if (!dateString) return 'Never';
  const d = new Date(dateString);
  if (isNaN(d.getTime())) return dateString;
  return d.toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  });
}

function formatIP(ip) {
  if (!ip) return '-';
  let str = String(ip).trim();
  if (!str) return '-';
  if (str.startsWith('[') && str.includes(']')) {
    const endBracket = str.indexOf(']');
    str = str.substring(1, endBracket);
  } else if (str.includes(':')) {
    const parts = str.split(':');
    if (parts.length === 2 && parts[0].includes('.')) {
      str = parts[0];
    }
  }
  if (str === '::1' || str.startsWith('::1')) {
    return '127.0.0.1';
  }
  if (str.startsWith('::ffff:')) {
    return str.replace('::ffff:', '');
  }
  if (str.includes(':')) {
    return '-';
  }
  return str;
}

function getFileType(filename) {
  const ext = filename.split('.').pop().toLowerCase();
  const imageExts = ['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg', 'bmp', 'ico', 'avif'];
  const videoExts = ['mp4', 'webm', 'mov', 'mkv', 'avi', 'wmv', 'flv'];
  const audioExts = ['mp3', 'wav', 'ogg', 'aac', 'flac', 'm4a', 'wma'];
  const archiveExts = ['zip', 'rar', '7z', 'tar', 'gz', 'bz2', 'xz', 'iso'];
  const codeExts = ['js', 'ts', 'jsx', 'tsx', 'html', 'css', 'scss', 'sql', 'go', 'py', 'java', 'c', 'cpp', 'rs', 'php', 'sh', 'bat', 'ps1'];
  const docExts = ['pdf', 'doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx', 'txt', 'md', 'json', 'csv', 'xml', 'yaml', 'yml', 'log'];

  if (imageExts.includes(ext)) return 'image';
  if (videoExts.includes(ext)) return 'video';
  if (audioExts.includes(ext)) return 'audio';
  if (archiveExts.includes(ext)) return 'archive';
  if (codeExts.includes(ext)) return 'code';
  if (docExts.includes(ext)) return 'document';
  return 'other';
}

function getFileTypeIcon(filename) {
  const ext = filename.split('.').pop().toLowerCase();
  
  if (ext === 'pdf') return 'ph-file-pdf';
  if (['xls', 'xlsx', 'csv'].includes(ext)) return 'ph-file-xls';
  if (['doc', 'docx'].includes(ext)) return 'ph-file-doc';
  if (['ppt', 'pptx'].includes(ext)) return 'ph-file-ppt';
  if (['zip', 'rar', '7z', 'tar', 'gz', 'bz2', 'xz', 'iso'].includes(ext)) return 'ph-file-archive';
  if (['js', 'ts', 'jsx', 'tsx', 'html', 'css', 'scss', 'json', 'xml', 'yaml', 'yml', 'sql', 'go', 'py', 'java', 'c', 'cpp', 'rs', 'php', 'sh', 'bat', 'ps1'].includes(ext)) return 'ph-file-code';

  const type = getFileType(filename);
  switch (type) {
    case 'image': return 'ph-file-image';
    case 'video': return 'ph-file-video';
    case 'audio': return 'ph-file-audio';
    case 'archive': return 'ph-file-archive';
    case 'code': return 'ph-file-code';
    case 'document': return 'ph-file-text';
    default: return 'ph-file';
  }
}

function showToast(message, type = 'success') {
  const container = document.getElementById('toastContainer');
  const toast = document.createElement('div');
  toast.className = `toast ${type}`;
  toast.textContent = message;
  container.appendChild(toast);

  setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transform = 'translateY(10px)';
    toast.style.transition = 'all 0.2s ease';
    setTimeout(() => toast.remove(), 200);
  }, 3000);
}

// API Request Wrapper with Session Key
async function apiRequest(endpoint, options = {}) {
  const headers = options.headers || {};
  if (state.activeKey) {
    headers['X-Session-Key'] = state.activeKey;
  }
  
  try {
    const res = await fetch(endpoint, { ...options, headers });
    if (res.status === 401 || res.status === 403) {
      showToast('Unauthorized: invalid session key or missing permissions', 'error');
      openAuthModal();
      throw new Error('Unauthorized');
    }
    const data = await res.json();
    if (!res.ok) {
      throw new Error(data.error || 'Request failed');
    }
    return data;
  } catch (err) {
    if (err.message !== 'Unauthorized') {
      showToast(err.message, 'error');
    }
    throw err;
  }
}

// Initial App Setup
document.addEventListener('DOMContentLoaded', () => {
  try { loadConfig(); } catch (e) { console.error('Config load error:', e); }
  try { setupNavigation(); } catch (e) { console.error('Nav setup error:', e); }
  try { setupDropzone(); } catch (e) { console.error('Dropzone setup error:', e); }
  try { setupModals(); } catch (e) { console.error('Modals setup error:', e); }
  try { setupFilterAndSearch(); } catch (e) { console.error('Filter setup error:', e); }
  try { setupDirectoryNavigation(); } catch (e) { console.error('Dir nav setup error:', e); }
  try { setupLogsControls(); } catch (e) { console.error('Logs controls error:', e); }
  try { loadStats(); } catch (e) { console.error('Stats load error:', e); }
  try { loadFiles(''); } catch (e) { console.error('Files load error:', e); }
  try { loadSessions(); } catch (e) { console.error('Sessions load error:', e); }
});

// Load System Config
async function loadConfig() {
  try {
    const data = await apiRequest('/api/config');
    state.bucketName = data.bucket_name || 'R2 Bucket';
    state.publicDomain = data.public_domain || '';
    if (data.app_url) {
      state.appUrl = data.app_url.replace(/\/+$/, '');
    }
    const bucketEl = document.getElementById('r2BucketName');
    if (bucketEl) {
      bucketEl.textContent = state.bucketName;
    }
    updateActiveKeyDisplay();
  } catch (e) {
    const bucketEl = document.getElementById('r2BucketName');
    if (bucketEl) {
      bucketEl.textContent = 'Disconnected';
    }
    const healthEl = document.getElementById('statR2Health');
    if (healthEl) {
      healthEl.textContent = 'Offline';
      healthEl.style.color = 'var(--text-muted)';
    }
  }
}

function updateActiveKeyDisplay() {
  const label = document.getElementById('activeKeyLabel');
  if (label) {
    if (!state.activeKey) {
      label.textContent = 'None';
      label.style.color = 'var(--text-muted)';
    } else {
      label.textContent = 'Active';
      label.style.color = 'var(--text-primary)';
    }
  }

  const docInput = document.getElementById('docKeyInput');
  // Security: NEVER auto-expose private keys or master admin keys in API documentation examples!
  const tokenToUse = (docInput && docInput.value.trim()) || 'YOUR_SESSION_KEY';
  document.querySelectorAll('.doc-active-key').forEach(el => {
    el.textContent = tokenToUse;
  });

  const origin = getAppOrigin();
  if (origin && origin !== 'null') {
    document.querySelectorAll('.doc-origin').forEach(el => {
      el.textContent = origin;
    });
  }

  if (state.bucketName) {
    document.querySelectorAll('.doc-bucket-name').forEach(el => {
      el.textContent = state.bucketName;
    });
  }

  if (state.publicDomain) {
    const pubDomain = state.publicDomain.replace(/\/+$/, '');
    document.querySelectorAll('.doc-public-domain').forEach(el => {
      el.textContent = pubDomain;
    });
  } else {
    document.querySelectorAll('.doc-public-domain').forEach(el => {
      el.textContent = 'https://cdn.yourdomain.com';
    });
  }
}

// Navigation Tabs
function setupNavigation() {
  const tabs = document.querySelectorAll('.nav-tab');
  tabs.forEach(tab => {
    tab.addEventListener('click', () => {
      const target = tab.dataset.tab;
      tabs.forEach(t => t.classList.remove('active'));
      tab.classList.add('active');

      document.querySelectorAll('.tab-content').forEach(content => {
        content.style.display = content.id === target ? 'block' : 'none';
      });

      state.currentTab = target;
      if (target === 'sessions-view') {
        loadSessions();
      } else if (target === 'files-view') {
        loadFiles(state.currentPath);
      } else if (target === 'logs-view') {
        loadLogs();
      } else if (target === 'docs-view') {
        updateActiveKeyDisplay();
      }
    });
  });

  const docInput = document.getElementById('docKeyInput');
  if (docInput) {
    docInput.addEventListener('input', () => {
      updateActiveKeyDisplay();
    });
  }

  const btnFillDocKey = document.getElementById('btnFillDocKey');
  if (btnFillDocKey) {
    btnFillDocKey.addEventListener('click', () => {
      if (docInput) {
        if (state.activeKey) {
          docInput.value = state.activeKey;
          updateActiveKeyDisplay();
          showToast('Doc examples previewing with active session key');
        } else {
          showToast('No active session key currently selected', 'error');
        }
      }
    });
  }

  localStorage.removeItem('r2_view_mode');

  document.getElementById('btnRefreshFiles').addEventListener('click', () => {
    loadFiles(state.currentPath);
    loadStats();
    showToast('Directory refreshed');
  });

  const btnRefreshLogs = document.getElementById('btnRefreshLogs');
  if (btnRefreshLogs) {
    btnRefreshLogs.addEventListener('click', () => {
      loadLogs();
      showToast('Activity logs refreshed');
    });
  }
}

// Directory Navigation & Breadcrumbs
function goUpDirectory() {
  if (!state.currentPath) return;
  const parts = state.currentPath.split('/');
  parts.pop();
  const parentPath = parts.join('/');
  loadFiles(parentPath);
}

function setupDirectoryNavigation() {
  const btnGoUp = document.getElementById('btnGoUp');
  if (btnGoUp) {
    btnGoUp.addEventListener('click', goUpDirectory);
  }

  // New Folder Modal
  const folderModal = document.getElementById('folderModal');
  const btnNewFolder = document.getElementById('btnNewFolder');
  const btnCloseFolder = document.getElementById('btnCloseFolderModal');
  const btnCancelFolder = document.getElementById('btnCancelFolderModal');
  const btnSaveFolder = document.getElementById('btnSaveFolder');
  const folderNameInput = document.getElementById('folderNameInput');

  btnNewFolder.addEventListener('click', () => {
    folderNameInput.value = '';
    folderModal.classList.add('active');
  });

  [btnCloseFolder, btnCancelFolder].forEach(btn => {
    btn.addEventListener('click', () => folderModal.classList.remove('active'));
  });

  btnSaveFolder.addEventListener('click', async () => {
    const name = folderNameInput.value.trim();
    if (!name) {
      showToast('Please enter folder name', 'error');
      return;
    }

    const fullRelPath = state.currentPath ? `${state.currentPath}/${name}` : name;

    try {
      await apiRequest('/api/folder', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: fullRelPath })
      });
      showToast(`Folder created: ${name}`);
      folderModal.classList.remove('active');
      loadFiles(state.currentPath);
    } catch (err) {}
  });
}

function renderBreadcrumbs() {
  const container = document.getElementById('breadcrumbsContainer');
  const btnGoUp = document.getElementById('btnGoUp');
  
  if (state.currentPath) {
    btnGoUp.style.display = 'inline-flex';
  } else {
    btnGoUp.style.display = 'none';
  }

  const rootLabel = state.rootFolder || 'root';
  let html = `<span class="breadcrumb-item ${!state.currentPath ? 'active' : ''}" onclick="loadFiles('')"><i class="ph ph-folder-open"></i> /${rootLabel}</span>`;

  if (state.currentPath) {
    const parts = state.currentPath.split('/');
    let cumulative = '';
    parts.forEach((part, index) => {
      cumulative += (cumulative ? '/' : '') + part;
      const isLast = index === parts.length - 1;
      html += ` <span class="breadcrumb-sep">/</span> `;
      if (isLast) {
        html += `<span class="breadcrumb-item active"><i class="ph ph-folder"></i> ${part}</span>`;
      } else {
        const targetPath = cumulative;
        html += `<span class="breadcrumb-item" onclick="loadFiles('${targetPath}')"><i class="ph ph-folder"></i> ${part}</span>`;
      }
    });
  }

  container.innerHTML = html;

  // Sync uploadFolderPrefix input
  const folderInput = document.getElementById('uploadFolderPrefix');
  if (folderInput) {
    folderInput.value = state.currentPath ? `${state.currentPath}/` : '';
  }
}

// Load Stats
async function loadStats() {
  try {
    const data = await apiRequest('/api/stats');
    document.getElementById('statTotalFiles').textContent = data.total_files.toLocaleString();
    document.getElementById('statTotalSize').textContent = formatBytes(data.total_size);
    document.getElementById('statR2Health').textContent = data.health || 'Online';
    document.getElementById('statActiveKeys').textContent = `${data.active_keys || 0} Active`;
    if (data.root_folder) {
      state.rootFolder = data.root_folder;
    }
  } catch (e) {
    console.error('Failed to load stats', e);
  }
}

// Load Files & Folders for current directory
async function loadFiles(targetPath = '') {
  try {
    const query = targetPath ? `?path=${encodeURIComponent(targetPath)}` : '';
    const data = await apiRequest(`/api/files${query}`);
    
    state.currentPath = data.current_path || '';
    state.rootFolder = data.root_folder || state.rootFolder;
    state.folders = data.folders || [];
    state.files = data.files || [];
    
    renderBreadcrumbs();
    filterFiles();
  } catch (e) {
    console.error('Failed to load directory', e);
  }
}

// Filter and Search Setup
function setupFilterAndSearch() {
  const searchInput = document.getElementById('searchInput');
  searchInput.addEventListener('input', (e) => {
    state.searchQuery = e.target.value.toLowerCase().trim();
    filterFiles();
  });

  const filterChips = document.querySelectorAll('.explorer-filter-strip .filter-chip[data-filter]');
  filterChips.forEach(chip => {
    chip.addEventListener('click', () => {
      filterChips.forEach(c => c.classList.remove('active'));
      chip.classList.add('active');
      state.filterType = chip.dataset.filter || 'all';
      filterFiles();
    });
  });
}

function filterFiles() {
  let list = state.files;

  if (state.filterType !== 'all') {
    list = list.filter(f => getFileType(f.key) === state.filterType);
  }

  if (state.searchQuery) {
    list = list.filter(f => f.key.toLowerCase().includes(state.searchQuery));
  }

  state.filteredFiles = list;
  renderFiles();
}

// Render Files & Folders (Table View)
function renderFiles() {
  const tableBody = document.getElementById('filesTableBody');
  const emptyState = document.getElementById('emptyFilesState');

  const totalItems = state.folders.length + state.filteredFiles.length;
  const currentFolderSize = state.filteredFiles.reduce((acc, f) => acc + (f.size || 0), 0);

  const statItemCount = document.getElementById('statItemCount');
  if (statItemCount) {
    statItemCount.textContent = `${totalItems} ${totalItems === 1 ? 'item' : 'items'}`;
  }
  const statTotalSize = document.getElementById('statTotalSize');
  if (statTotalSize) {
    statTotalSize.textContent = formatBytes(currentFolderSize);
  }

  const isSubfolder = Boolean(state.currentPath);

  // If completely empty at root level
  if (totalItems === 0 && !isSubfolder) {
    tableBody.innerHTML = '';
    emptyState.style.display = 'block';
    return;
  }

  emptyState.style.display = 'none';

  let tableHTML = '';

  // 1. Render Parent Directory (..) navigation row if inside a subfolder
  if (isSubfolder) {
    tableHTML += `
      <tr class="up-directory-row" style="background: rgba(255, 255, 255, 0.02); cursor: pointer;" onclick="goUpDirectory()" title="Go to parent directory">
        <td>
          <div class="item-name-cell">
            <i class="ph ph-arrow-bend-up-left" style="font-size: 1.15rem; color: var(--accent-primary); margin-right: 0.35rem;"></i>
            <span class="item-title" style="font-weight: 600; color: var(--text-primary);">..</span>
            <span class="text-muted" style="font-size: 0.75rem; margin-left: 0.35rem;">(Parent directory)</span>
          </div>
        </td>
        <td><span class="badge" style="opacity: 0.85;">UP</span></td>
        <td class="cell-mono text-muted">-</td>
        <td class="cell-mono text-muted">-</td>
        <td style="text-align: right;">
          <div class="row-actions">
            <button class="btn-icon" onclick="event.stopPropagation(); goUpDirectory()" title="Up to parent directory"><i class="ph ph-arrow-bend-up-left"></i></button>
          </div>
        </td>
      </tr>
    `;
  }

  // If subfolder is completely empty, show empty notice below .. row
  if (totalItems === 0 && isSubfolder) {
    tableHTML += `
      <tr>
        <td colspan="5" style="text-align: center; color: var(--text-muted); padding: 3rem 1rem;">
          <i class="ph ph-folder-open" style="font-size: 2.2rem; display: block; margin-bottom: 0.5rem; color: var(--text-muted); opacity: 0.7;"></i>
          <div style="font-size: 0.95rem; font-weight: 600; color: var(--text-secondary); margin-bottom: 0.25rem;">This directory is empty</div>
          <div style="font-size: 0.8rem;">Drag and drop files here or click "Upload Files" to upload into this folder.</div>
        </td>
      </tr>
    `;
    tableBody.innerHTML = tableHTML;
    return;
  }

  // 2. Render Folders
  state.folders.forEach(folder => {
    tableHTML += `
      <tr>
        <td>
          <div class="item-name-cell" onclick="loadFiles('${encodeURIComponent(folder.path)}')">
            <i class="ph ph-folder" style="font-size: 1.15rem; color: var(--text-muted); margin-right: 0.25rem;"></i>
            <span class="item-title">${folder.name}/</span>
          </div>
        </td>
        <td><span class="badge">DIR</span></td>
        <td class="cell-mono text-muted">-</td>
        <td class="cell-mono text-muted">-</td>
        <td style="text-align: right;">
          <div class="row-actions">
            <button class="btn-icon" onclick="loadFiles('${encodeURIComponent(folder.path)}')" title="Open folder"><i class="ph ph-arrow-square-out"></i></button>
            <button class="btn-icon btn-icon-danger" onclick="deleteFolder('${encodeURIComponent(folder.path)}')" title="Delete folder"><i class="ph ph-trash"></i></button>
          </div>
        </td>
      </tr>
    `;
  });

  // 3. Render Files
  state.filteredFiles.forEach(file => {
    const ext = file.name.split('.').pop().toUpperCase();
    const typeIcon = getFileTypeIcon(file.name);
    const fileUrl = getFileDownloadUrl(file.key);

    tableHTML += `
      <tr>
        <td>
          <div class="item-name-cell" onclick="openPreview('${encodeURIComponent(file.key)}')">
            <i class="ph ${typeIcon}" style="font-size: 1.15rem; color: var(--text-muted); margin-right: 0.25rem;"></i>
            <span class="item-title" title="${file.name}">${file.name}</span>
          </div>
        </td>
        <td><span class="badge">${ext}</span></td>
        <td class="cell-mono">${formatBytes(file.size)}</td>
        <td class="cell-mono">${formatDate(file.last_modified)}</td>
        <td style="text-align: right;">
          <div class="row-actions">
            <button class="btn-icon" onclick="openPreview('${encodeURIComponent(file.key)}')" title="View / Preview"><i class="ph ph-eye"></i></button>
            <button class="btn-icon" onclick="copyFileUrl('${encodeURIComponent(file.key)}')" title="Copy Public CDN URL"><i class="ph ph-copy"></i></button>
            <button class="btn-icon" onclick="openFileInfo('${encodeURIComponent(file.key)}')" title="File Information & Metadata (Hash, ETag, MIME)"><i class="ph ph-info"></i></button>
            <a href="${fileUrl}" download="${file.name}" class="btn-icon" title="Download file"><i class="ph ph-download-simple"></i></a>
            <button class="btn-icon btn-icon-danger" onclick="deleteFile('${encodeURIComponent(file.key)}')" title="Delete file"><i class="ph ph-trash"></i></button>
          </div>
        </td>
      </tr>
    `;
  });

  tableBody.innerHTML = tableHTML;
}

function getFileDownloadUrl(key) {
  if (state.publicDomain) {
    const rf = (state.rootFolder || '').replace(/^\/+|\/+$/g, '');
    const cleanKey = (key || '').replace(/^\/+/, '');
    const fullKey = rf ? `${rf}/${cleanKey}` : cleanKey;
    const pathEncoded = fullKey.split('/').map(encodeURIComponent).join('/');
    return `${state.publicDomain.replace(/\/+$/, '')}/${pathEncoded}`;
  }
  return `/api/file?key=${encodeURIComponent(key)}`;
}

// Copy URL
function copyFileUrl(encodedKey) {
  const key = decodeURIComponent(encodedKey);
  const url = getFileDownloadUrl(key);
  const fullUrl = url.startsWith('http') ? url : getAppOrigin() + url;
  navigator.clipboard.writeText(fullUrl).then(() => {
    showToast('File URL copied to clipboard');
  });
}

// Delete File
async function deleteFile(encodedKey) {
  const key = decodeURIComponent(encodedKey);
  if (!confirm(`Are you sure you want to permanently delete:\n"${key}"?`)) {
    return;
  }

  try {
    await apiRequest(`/api/file?key=${encodeURIComponent(key)}`, {
      method: 'DELETE'
    });
    showToast(`Deleted: ${key}`);
    loadFiles(state.currentPath);
    loadStats();
  } catch (err) {
    console.error('Delete error', err);
  }
}

// Delete Folder
async function deleteFolder(encodedPath) {
  const folderPath = decodeURIComponent(encodedPath);
  if (!confirm(`Are you sure you want to permanently delete folder and all contents:\n"${folderPath}/"?`)) {
    return;
  }

  try {
    await apiRequest(`/api/folder?path=${encodeURIComponent(folderPath)}`, {
      method: 'DELETE'
    });
    showToast(`Deleted folder: ${folderPath}`);
    loadFiles(state.currentPath);
    loadStats();
  } catch (err) {
    console.error('Delete folder error', err);
  }
}

// Upload Handling & Dropzone
function setupDropzone() {
  const dropzone = document.getElementById('dropzone');
  const fileInput = document.getElementById('fileInput');
  const btnSelect = document.getElementById('btnSelectFiles');

  btnSelect.addEventListener('click', (e) => {
    e.stopPropagation();
    fileInput.click();
  });

  dropzone.addEventListener('click', () => fileInput.click());

  ['dragenter', 'dragover'].forEach(eventName => {
    dropzone.addEventListener(eventName, (e) => {
      e.preventDefault();
      e.stopPropagation();
      dropzone.classList.add('dragover');
    });
  });

  ['dragleave', 'drop'].forEach(eventName => {
    dropzone.addEventListener(eventName, (e) => {
      e.preventDefault();
      e.stopPropagation();
      dropzone.classList.remove('dragover');
    });
  });

  dropzone.addEventListener('drop', (e) => {
    const files = e.dataTransfer.files;
    if (files.length > 0) {
      handleFilesUpload(files);
    }
  });

  fileInput.addEventListener('change', () => {
    if (fileInput.files.length > 0) {
      handleFilesUpload(fileInput.files);
      fileInput.value = '';
    }
  });
}

async function handleFilesUpload(files) {
  const queueContainer = document.getElementById('uploadQueueContainer');
  queueContainer.style.display = 'flex';
  
  // Target folder from input or current active folder path
  let targetFolder = document.getElementById('uploadFolderPrefix').value.trim();
  if (!targetFolder && state.currentPath) {
    targetFolder = state.currentPath + '/';
  }

  for (const file of Array.from(files)) {
    const itemEl = document.createElement('div');
    itemEl.className = 'upload-item';
    itemEl.innerHTML = `
      <div style="flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
        <strong>${file.name}</strong> (${formatBytes(file.size)})
      </div>
      <div class="upload-progress-bar">
        <div class="upload-progress-fill" style="width: 0%;"></div>
      </div>
      <div class="upload-status-text" style="font-size: 0.75rem; color: var(--text-dim); min-width: 60px; text-align: right;">Uploading...</div>
    `;
    queueContainer.prepend(itemEl);

    const progressFill = itemEl.querySelector('.upload-progress-fill');
    const statusText = itemEl.querySelector('.upload-status-text');

    try {
      const formData = new FormData();
      formData.append('file', file);
      if (targetFolder) {
        formData.append('folder', targetFolder);
      }

      await new Promise((resolve, reject) => {
        const xhr = new XMLHttpRequest();
        xhr.open('POST', '/api/upload');
        if (state.activeKey) {
          xhr.setRequestHeader('X-Session-Key', state.activeKey);
        }

        xhr.upload.onprogress = (e) => {
          if (e.lengthComputable) {
            const percent = Math.round((e.loaded / e.total) * 100);
            progressFill.style.width = percent + '%';
            statusText.textContent = percent + '%';
          }
        };

        xhr.onload = () => {
          if (xhr.status >= 200 && xhr.status < 300) {
            progressFill.style.width = '100%';
            progressFill.style.background = 'var(--text-primary)';
            statusText.textContent = 'Completed';
            statusText.style.color = 'var(--text-primary)';
            resolve(JSON.parse(xhr.responseText));
          } else {
            let errorMsg = 'Upload failed';
            try {
              const res = JSON.parse(xhr.responseText);
              errorMsg = res.error || errorMsg;
            } catch (_) {}
            reject(new Error(errorMsg));
          }
        };

        xhr.onerror = () => reject(new Error('Network error'));
        xhr.send(formData);
      });

      showToast(`Uploaded: ${file.name}`);
    } catch (err) {
      progressFill.style.background = 'var(--border-strong)';
      statusText.textContent = 'Failed';
      statusText.style.color = 'var(--text-muted)';
      showToast(`Upload failed for ${file.name}: ${err.message}`, 'error');
    }
  }

  loadFiles(state.currentPath);
  loadStats();
}

// Session Key Management
async function loadSessions() {
  try {
    const data = await apiRequest('/api/sessions');
    state.sessions = data.sessions || [];
    state.isMasterAdmin = Boolean(data.is_master_admin);

    // Hide "+ Create Session Key" button if user is not Master Admin
    const btnCreate = document.getElementById('btnOpenCreateSessionModal');
    if (btnCreate) {
      btnCreate.style.display = state.isMasterAdmin ? 'inline-flex' : 'none';
    }

    renderSessions();
    updateActiveKeyDisplay();
  } catch (err) {
    console.error('Failed to load sessions', err);
  }
}

function renderSessions() {
  const tbody = document.getElementById('sessionsTableBody');
  if (!tbody) return;

  if (!state.sessions || state.sessions.length === 0) {
    tbody.innerHTML = `<tr><td colspan="6" style="text-align: center; color: var(--text-dim); padding: 2rem;">No session keys found. Click "Create Session Key" to create one.</td></tr>`;
    return;
  }

  tbody.innerHTML = state.sessions.map(s => {
    const isMaster = Boolean(s.is_master || s.id === 'sess_master');
    const isCurrent = state.activeKey === s.key;
    const expiresText = isMaster ? '<span style="color: var(--text-secondary); font-weight: 500;">Immortal</span>' : (s.expires_at ? formatDate(s.expires_at) : '<span style="color: var(--text-secondary);">Never</span>');

    const perms = (s.permissions || []).map(p => {
      return `<span class="badge" style="margin-right: 4px;">${escapeHtml(p)}</span>`;
    }).join('');

    // Toggle switch: Master key cannot be toggled off
    let switchHtml = '';
    if (isMaster) {
      switchHtml = `
        <label class="switch" title="Master Admin Key is system-protected and cannot be deactivated" style="opacity: 0.65; cursor: not-allowed;">
          <input type="checkbox" checked disabled>
          <span class="slider" style="cursor: not-allowed;"></span>
        </label>
      `;
    } else {
      switchHtml = `
        <label class="switch">
          <input type="checkbox" ${s.is_active ? 'checked' : ''} onchange="toggleSessionStatus('${s.id}', this.checked)">
          <span class="slider"></span>
        </label>
      `;
    }

    // Actions column: Master key and current active key cannot be deleted
    let deleteActionHtml = '';
    if (isMaster) {
      deleteActionHtml = `<span class="btn-icon" style="opacity: 0.35; cursor: not-allowed;" title="Master Admin Key is system-protected and cannot be deleted"><i class="ph ph-lock-key"></i></span>`;
    } else if (isCurrent) {
      deleteActionHtml = `<span class="btn-icon" style="opacity: 0.35; cursor: not-allowed;" title="Currently active session key cannot be deleted. Switch session first."><i class="ph ph-shield-check"></i></span>`;
    } else {
      deleteActionHtml = `<button class="btn-icon btn-icon-danger" onclick="deleteSessionKey('${s.id}', '${escapeHtml(s.name)}')" title="Revoke Key"><i class="ph ph-trash"></i></button>`;
    }

    return `
      <tr class="${isMaster ? 'row-master-session' : ''}">
        <td>
          <div style="font-weight: 500; display: flex; align-items: center; gap: 6px; flex-wrap: wrap;">
            ${escapeHtml(s.name)}
            ${isMaster ? '<span class="badge" style="font-size: 0.65rem; background: rgba(16, 185, 129, 0.15); color: #10b981; border: 1px solid rgba(16, 185, 129, 0.3); font-weight: 600;"><i class="ph ph-crown"></i> Master Key</span>' : ''}
            ${isCurrent ? '<span class="badge" style="font-size: 0.65rem; background: var(--bg-card-hover); color: var(--text-primary);"><i class="ph ph-check"></i> Active</span>' : ''}
          </div>
          <div style="font-size: 0.725rem; color: var(--text-muted); font-family: var(--font-mono);">${formatDate(s.created_at)}</div>
        </td>
        <td>${perms}</td>
        <td>${switchHtml}</td>
        <td class="cell-mono text-muted">${expiresText}</td>
        <td class="cell-mono text-muted">${formatDate(s.last_used_at)}</td>
        <td style="text-align: right;">
          <div class="row-actions">
            <button class="btn-icon" onclick="copyToClipboard('${s.key}', 'Session key copied to clipboard')" title="Copy Key"><i class="ph ph-copy"></i></button>
            ${!isCurrent ? `<button class="btn-icon" onclick="useSessionKey('${s.key}')" title="Set as Active Key"><i class="ph ph-shield-check"></i></button>` : ''}
            ${deleteActionHtml}
          </div>
        </td>
      </tr>
    `;
  }).join('');
}

async function toggleSessionStatus(id, isActive) {
  try {
    await apiRequest(`/api/sessions/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ is_active: isActive })
    });
    showToast(isActive ? 'Session key activated' : 'Session key disabled');
    loadSessions();
    loadStats();
  } catch (err) {
    loadSessions();
  }
}

async function deleteSessionKey(id, name) {
  const label = name ? `'${name}'` : 'this session key';
  if (!confirm(`Are you sure you want to revoke and delete ${label}? This action cannot be undone.`)) {
    return;
  }

  try {
    await apiRequest(`/api/sessions/${id}`, { method: 'DELETE' });
    showToast('Session key deleted successfully');
    loadSessions();
    loadStats();
  } catch (err) {}
}

function useSessionKey(key) {
  state.activeKey = key;
  localStorage.setItem('r2_session_key', key);
  updateActiveKeyDisplay();
  showToast('Switched to selected session key');
  loadSessions();
}

function copyToClipboard(text, msg = 'Copied to clipboard') {
  navigator.clipboard.writeText(text).then(() => showToast(msg));
}

// Modals Handling
function setupModals() {
  const sessionModal = document.getElementById('sessionModal');
  const btnOpenCreate = document.getElementById('btnOpenCreateSessionModal');
  const btnCloseSession = document.getElementById('btnCloseSessionModal');
  const btnCancelSession = document.getElementById('btnCancelSessionModal');
  const btnSaveSession = document.getElementById('btnSaveSession');

  btnOpenCreate.addEventListener('click', () => {
    document.getElementById('sessionModalTitle').textContent = 'Create Session Key';
    document.getElementById('editSessionId').value = '';
    document.getElementById('sessionNameInput').value = '';
    document.getElementById('sessionKeyInput').value = '';
    document.getElementById('customKeyGroup').style.display = 'block';
    document.getElementById('expiryGroup').style.display = 'block';
    sessionModal.classList.add('active');
  });

  [btnCloseSession, btnCancelSession].forEach(btn => {
    btn.addEventListener('click', () => sessionModal.classList.remove('active'));
  });

  btnSaveSession.addEventListener('click', async () => {
    const name = document.getElementById('sessionNameInput').value.trim();
    if (!name) {
      showToast('Please enter a session name', 'error');
      return;
    }

    const customKey = document.getElementById('sessionKeyInput').value.trim();
    const expiryDays = parseInt(document.getElementById('sessionExpirySelect').value, 10);
    const checkedPerms = Array.from(document.querySelectorAll('input[name="permissions"]:checked')).map(cb => cb.value);

    try {
      await apiRequest('/api/sessions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: name,
          key: customKey || undefined,
          permissions: checkedPerms,
          expires_in_days: expiryDays
        })
      });

      showToast('Session key created successfully');
      sessionModal.classList.remove('active');
      loadSessions();
      loadStats();
    } catch (err) {}
  });

  // Auth / Switch Key Modal
  const authModal = document.getElementById('authModal');
  const btnActiveSession = document.getElementById('btnActiveSession');
  const btnCloseAuth = document.getElementById('btnCloseAuthModal');
  const btnCancelAuth = document.getElementById('btnCancelAuthModal');
  const btnApplyAuth = document.getElementById('btnApplyAuthKey');
  const authKeyInput = document.getElementById('authKeyInput');

  window.openAuthModal = () => {
    authKeyInput.value = state.activeKey;
    authModal.classList.add('active');
  };

  if (btnActiveSession) {
    btnActiveSession.addEventListener('click', window.openAuthModal);
  }
  const bucketBadge = document.getElementById('r2BucketName');
  if (bucketBadge) {
    bucketBadge.style.cursor = 'pointer';
    bucketBadge.title = 'Click to switch session key / authenticate';
    bucketBadge.addEventListener('click', window.openAuthModal);
  }
  [btnCloseAuth, btnCancelAuth].forEach(btn => {
    btn.addEventListener('click', () => authModal.classList.remove('active'));
  });

  btnApplyAuth.addEventListener('click', () => {
    const val = authKeyInput.value.trim();
    state.activeKey = val;
    if (val) {
      localStorage.setItem('r2_session_key', val);
      showToast('Session key saved');
    } else {
      localStorage.removeItem('r2_session_key');
      showToast('Session key cleared');
    }
    updateActiveKeyDisplay();
    authModal.classList.remove('active');
    loadFiles(state.currentPath);
    loadSessions();
    loadStats();
  });

  // Preview Modal
  const previewModal = document.getElementById('previewModal');
  const btnClosePreview = document.getElementById('btnClosePreviewModal');
  if (btnClosePreview) {
    btnClosePreview.addEventListener('click', () => {
      previewModal.classList.remove('active');
      document.getElementById('previewContainer').innerHTML = '';
    });
  }

  // File Info Modal
  const fileInfoModal = document.getElementById('fileInfoModal');
  const btnCloseFileInfo = document.getElementById('btnCloseFileInfoModal');
  const btnCloseFileInfoBtn = document.getElementById('btnCloseFileInfoBtn');
  const btnCopyInfoPublicUrl = document.getElementById('btnCopyInfoPublicUrl');
  const btnCopyInfoJson = document.getElementById('btnCopyInfoJson');

  [btnCloseFileInfo, btnCloseFileInfoBtn].forEach(b => {
    if (b) b.addEventListener('click', () => fileInfoModal.classList.remove('active'));
  });

  if (btnCopyInfoPublicUrl) {
    btnCopyInfoPublicUrl.addEventListener('click', () => {
      const url = document.getElementById('infoPublicUrl').value;
      copyToClipboard(url, 'Public CDN URL copied');
    });
  }

  if (btnCopyInfoJson) {
    btnCopyInfoJson.addEventListener('click', () => {
      if (currentFileInfoData) {
        copyToClipboard(JSON.stringify(currentFileInfoData, null, 2), 'File metadata copied as JSON');
      }
    });
  }
}

// Open File Previewer
async function openPreview(encodedKey) {
  const key = decodeURIComponent(encodedKey);
  const type = getFileType(key);
  const fileUrl = getFileDownloadUrl(key);
  const fileName = key.split('/').pop();

  const previewModal = document.getElementById('previewModal');
  const previewContainer = document.getElementById('previewContainer');
  document.getElementById('previewFileName').textContent = fileName;
  document.getElementById('btnDownloadPreview').href = fileUrl;
  document.getElementById('btnDownloadPreview').download = fileName;

  document.getElementById('btnCopyPreviewUrl').onclick = () => {
    const fullUrl = fileUrl.startsWith('http') ? fileUrl : getAppOrigin() + fileUrl;
    copyToClipboard(fullUrl, 'File link copied');
  };

  previewContainer.innerHTML = '';

  if (type === 'image') {
    previewContainer.innerHTML = `
      <div class="media-preview-box">
        <img src="${fileUrl}" alt="${fileName}">
      </div>
    `;
  } else if (type === 'video') {
    previewContainer.innerHTML = `
      <div class="media-preview-box">
        <video controls autoplay style="width: 100%;">
          <source src="${fileUrl}">
          Your browser does not support HTML5 video.
        </video>
      </div>
    `;
  } else if (type === 'audio') {
    previewContainer.innerHTML = `
      <div class="media-preview-box" style="padding: 2.5rem 1rem;">
        <audio controls autoplay style="width: 90%;">
          <source src="${fileUrl}">
          Your browser does not support HTML5 audio.
        </audio>
      </div>
    `;
  } else if (type === 'document' && (key.endsWith('.txt') || key.endsWith('.json') || key.endsWith('.md') || key.endsWith('.csv'))) {
    previewContainer.innerHTML = `<div class="text-preview-area">Loading file content...</div>`;
    try {
      const res = await fetch(fileUrl);
      const text = await res.text();
      previewContainer.querySelector('.text-preview-area').textContent = text.slice(0, 100000);
    } catch (e) {
      previewContainer.querySelector('.text-preview-area').textContent = 'Unable to preview text file content.';
    }
  } else {
    previewContainer.innerHTML = `
      <div class="empty-state" style="padding: 2rem;">
        <p>Direct preview is not supported for this file type.</p>
        <p style="font-size: 0.8rem; color: var(--text-dim); margin-top: 0.25rem;">You can download the file to open it with local applications.</p>
      </div>
    `;
  }

  previewModal.classList.add('active');
}

// File Info & Metadata Modal
let currentFileInfoData = null;

async function openFileInfo(encodedKey) {
  const key = decodeURIComponent(encodedKey);
  const file = state.filteredFiles.find(f => f.key === key) || { key, name: key.split('/').pop() };

  const modal = document.getElementById('fileInfoModal');
  const loading = document.getElementById('fileInfoLoading');
  const content = document.getElementById('fileInfoContent');

  modal.classList.add('active');
  loading.style.display = 'block';
  content.style.display = 'none';

  const fileName = file.name || key.split('/').pop();
  const fullKey = state.rootFolder ? `${state.rootFolder}/${key}` : key;
  const pubUrl = getFileDownloadUrl(key);

  // Pre-populate with currently known data
  document.getElementById('infoFileName').textContent = fileName;
  document.getElementById('infoFullKey').textContent = fullKey;
  document.getElementById('infoFileSize').textContent = file.size !== undefined ? `${formatBytes(file.size)} (${file.size.toLocaleString()} bytes)` : '-';
  document.getElementById('infoContentType').textContent = getFileType(fileName);
  document.getElementById('infoETag').textContent = file.etag ? `"${file.etag}"` : '-';
  document.getElementById('infoLastModified').textContent = file.last_modified ? formatDate(file.last_modified) : '-';
  document.getElementById('infoStorageClass').textContent = 'STANDARD';
  document.getElementById('infoBucket').textContent = document.getElementById('r2BucketName').textContent || '-';
  document.getElementById('infoPublicUrl').value = pubUrl.startsWith('http') ? pubUrl : getAppOrigin() + pubUrl;
  document.getElementById('btnDownloadFromInfo').href = pubUrl;
  document.getElementById('btnDownloadFromInfo').download = fileName;

  currentFileInfoData = {
    name: fileName,
    key: key,
    full_key: fullKey,
    size: file.size,
    etag: file.etag,
    last_modified: file.last_modified,
    public_url: pubUrl.startsWith('http') ? pubUrl : getAppOrigin() + pubUrl
  };

  try {
    const data = await apiRequest(`/api/file/info?key=${encodeURIComponent(key)}`);
    currentFileInfoData = data;
    loading.style.display = 'none';
    content.style.display = 'block';

    document.getElementById('infoFileName').textContent = data.name || fileName;
    document.getElementById('infoFullKey').textContent = data.full_key || fullKey;
    document.getElementById('infoFileSize').textContent = `${formatBytes(data.size)} (${(data.size || 0).toLocaleString()} bytes)`;
    document.getElementById('infoContentType').textContent = data.content_type || 'application/octet-stream';
    document.getElementById('infoETag').textContent = data.etag ? `"${data.etag}" (MD5 Hash)` : '-';
    document.getElementById('infoLastModified').textContent = formatDate(data.last_modified);
    document.getElementById('infoStorageClass').textContent = data.storage_class || 'STANDARD';
    document.getElementById('infoBucket').textContent = data.bucket || '-';

    if (data.public_url) {
      document.getElementById('infoPublicUrl').value = data.public_url;
      document.getElementById('btnDownloadFromInfo').href = data.public_url;
    }

    const metaRow = document.getElementById('infoMetadataRow');
    if (data.metadata && Object.keys(data.metadata).length > 0) {
      metaRow.style.display = '';
      document.getElementById('infoMetadata').textContent = JSON.stringify(data.metadata, null, 2);
    } else {
      metaRow.style.display = 'none';
    }
  } catch (err) {
    loading.style.display = 'none';
    content.style.display = 'block';
  }
}


// Activity Logs State & Handlers
let allLogs = [];
let currentLogFilter = 'all';
let currentLogSearch = '';

function setupLogsControls() {
  const searchInput = document.getElementById('searchLogsInput');
  if (searchInput) {
    searchInput.addEventListener('input', (e) => {
      currentLogSearch = e.target.value.toLowerCase().trim();
      filterAndRenderLogs();
    });
  }

  const chips = document.querySelectorAll('#logFilterChips .filter-chip');
  chips.forEach(chip => {
    chip.addEventListener('click', () => {
      chips.forEach(c => c.classList.remove('active'));
      chip.classList.add('active');
      currentLogFilter = chip.dataset.logFilter || 'all';
      filterAndRenderLogs();
    });
  });

  const btnPurge = document.getElementById('btnPurgeLogs');
  if (btnPurge) {
    btnPurge.addEventListener('click', async () => {
      if (!confirm('Are you sure you want to purge all activity logs older than 7 days?')) {
        return;
      }
      try {
        const res = await apiRequest('/api/logs', { method: 'DELETE' });
        showToast(res.message || 'Purged logs older than 7 days');
        loadLogs();
      } catch (err) {}
    });
  }
}

// Load Activity Logs from SQLite
async function loadLogs() {
  try {
    const data = await apiRequest('/api/logs');
    allLogs = data.logs || [];
    filterAndRenderLogs();
  } catch (err) {
    console.error('Failed to load logs', err);
  }
}

function filterAndRenderLogs() {
  const tbody = document.getElementById('logsTableBody');
  const countBadge = document.getElementById('statLogsCount');
  if (!tbody) return;

  let list = allLogs;

  if (currentLogFilter !== 'all') {
    list = list.filter(l => (l.action || '').toUpperCase().includes(currentLogFilter));
  }

  if (currentLogSearch) {
    list = list.filter(l =>
      (l.action || '').toLowerCase().includes(currentLogSearch) ||
      (l.target_key || '').toLowerCase().includes(currentLogSearch) ||
      (l.file_name || '').toLowerCase().includes(currentLogSearch) ||
      (l.session_name || '').toLowerCase().includes(currentLogSearch) ||
      (l.ip_address || '').toLowerCase().includes(currentLogSearch) ||
      (l.details || '').toLowerCase().includes(currentLogSearch)
    );
  }

  if (countBadge) {
    countBadge.textContent = `${list.length} ${list.length === 1 ? 'event' : 'events'}`;
  }

  if (list.length === 0) {
    tbody.innerHTML = `<tr><td colspan="7" style="text-align: center; color: var(--text-muted); padding: 2.5rem;"><i class="ph ph-clock-counter-clockwise" style="font-size: 1.5rem; display: block; margin-bottom: 0.5rem;"></i>No activity logs found for the selected filter.</td></tr>`;
    return;
  }

  tbody.innerHTML = list.map(l => {
    let badgeClass = 'badge-action';
    let iconClass = 'ph-info';
    const act = (l.action || 'ACTION').toUpperCase();

    if (act.includes('UPLOAD')) {
      badgeClass += ' badge-upload';
      iconClass = 'ph-upload-simple';
    } else if (act.includes('DELETE')) {
      badgeClass += ' badge-delete';
      iconClass = 'ph-trash';
    } else if (act.includes('FOLDER')) {
      badgeClass += ' badge-folder';
      iconClass = 'ph-folder';
    } else if (act.includes('PRESIGN')) {
      badgeClass += ' badge-presign';
      iconClass = 'ph-link';
    } else if (act.includes('KEY')) {
      badgeClass += ' badge-key';
      iconClass = 'ph-key';
    }

    const sizeDisplay = l.file_size > 0 ? formatBytes(l.file_size) : '-';
    let targetDisplay = l.target_key || l.file_name || '-';

    // Prevent token strings from leaking in target column for key events
    if (act.includes('KEY') && targetDisplay.length > 20 && !targetDisplay.startsWith('sess_')) {
      targetDisplay = l.file_name || 'Session Key';
    }

    let sessionName = l.session_name;
    if (!sessionName && l.session_id) {
      const match = state.sessions.find(s => s.id === l.session_id);
      if (match) sessionName = match.name;
    }
    if (!sessionName) {
      sessionName = 'Master Admin Key';
    } else if (sessionName === 'Authorized Session' || sessionName.startsWith('Token (') || sessionName.includes('...') || sessionName.startsWith('sk_') || sessionName.startsWith('admin_')) {
      sessionName = 'Master Admin Key';
    }

    let sessionDisplay = '';
    if (sessionName === 'Master Admin Key') {
      sessionDisplay = `<span class="badge" style="font-size: 0.725rem; background: rgba(16, 185, 129, 0.15); color: #10b981; border: 1px solid rgba(16, 185, 129, 0.3); font-weight: 500;"><i class="ph ph-crown" style="font-size: 0.75rem; margin-right: 3px;"></i> Master Admin</span>`;
    } else if (sessionName === 'Web Dashboard') {
      sessionDisplay = `<span class="badge" style="font-size: 0.725rem;"><i class="ph ph-browser" style="font-size: 0.75rem; margin-right: 3px;"></i> Dashboard</span>`;
    } else {
      sessionDisplay = `<span class="badge" style="font-size: 0.725rem;"><i class="ph ph-key" style="font-size: 0.75rem; margin-right: 3px;"></i> ${escapeHtml(sessionName)}</span>`;
    }

    return `
      <tr>
        <td><span class="${badgeClass}"><i class="ph ${iconClass}"></i> ${act}</span></td>
        <td><code style="font-family: var(--font-mono); font-size: 0.775rem; color: var(--text-primary); word-break: break-all;">${escapeHtml(targetDisplay)}</code></td>
        <td class="cell-mono text-muted">${sizeDisplay}</td>
        <td>${sessionDisplay}</td>
        <td class="cell-mono text-muted">${escapeHtml(formatIP(l.ip_address))}</td>
        <td style="color: var(--text-secondary); font-size: 0.775rem; max-width: 320px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title="${escapeHtml(l.details || '')}">${escapeHtml(l.details || '-')}</td>
        <td class="cell-mono text-muted">${formatDate(l.created_at)}</td>
      </tr>
    `;
  }).join('');
}

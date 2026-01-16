/**
 * DLNA Movie Cast - Main Application
 */

import * as api from './api.js';

// State
let state = {
  movies: [],
  devices: [],
  currentMovie: null,
  selectedDevice: null,
  selectedSubtitle: null,
  isPlaying: false,
  playingMovieId: null,
  playingDeviceId: null,
  searchQuery: '',
};

// DOM Elements
const elements = {
  movieGrid: document.getElementById('movieGrid'),
  emptyState: document.getElementById('emptyState'),
  deviceList: document.getElementById('deviceList'),
  noDevices: document.getElementById('noDevices'),
  movieModal: document.getElementById('movieModal'),
  movieDetail: document.getElementById('movieDetail'),
  closeModal: document.getElementById('closeModal'),
  scanBtn: document.getElementById('scanBtn'),
  searchInput: document.getElementById('searchInput'),
  refreshDevicesBtn: document.getElementById('refreshDevicesBtn'),
  nowPlaying: document.getElementById('nowPlaying'),
  npTitle: document.getElementById('npTitle'),
  npDevice: document.getElementById('npDevice'),
  playPauseBtn: document.getElementById('playPauseBtn'),
  stopBtn: document.getElementById('stopBtn'),
  progressBar: document.getElementById('progressBar'),
  timeDisplay: document.getElementById('timeDisplay'),
  toastContainer: document.getElementById('toastContainer'),
};

// Initialize the app
async function init() {
  setupNavigation();
  setupEventListeners();
  await loadMovies();
  await loadDevices();

  // Start polling for playback status if playing
  setInterval(updatePlaybackStatus, 1000);
}

// Navigation
function setupNavigation() {
  document.querySelectorAll('.nav-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const view = btn.dataset.view;

      // Update active button
      document.querySelectorAll('.nav-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');

      // Show view
      document.querySelectorAll('.view').forEach(v => v.classList.remove('active'));
      document.getElementById(`${view}View`).classList.add('active');
    });
  });
}

// Event Listeners
function setupEventListeners() {
  // Search
  elements.searchInput.addEventListener('input', (e) => {
    state.searchQuery = e.target.value.toLowerCase();
    renderMovies();
  });

  // Scan library
  elements.scanBtn.addEventListener('click', async () => {
    elements.scanBtn.classList.add('spinning');
    try {
      await api.scanLibrary();
      showToast('Library scan started', 'success');
      // Reload movies after a delay
      setTimeout(loadMovies, 3000);
    } catch (error) {
      showToast('Failed to start scan: ' + error.message, 'error');
    } finally {
      elements.scanBtn.classList.remove('spinning');
    }
  });

  // Refresh devices
  elements.refreshDevicesBtn.addEventListener('click', async () => {
    await api.refreshDevices();
    showToast('Searching for devices...', 'success');
    setTimeout(loadDevices, 2000);
  });

  // Close modal
  elements.closeModal.addEventListener('click', closeModal);
  elements.movieModal.querySelector('.modal-backdrop').addEventListener('click', closeModal);

  // Playback controls
  elements.playPauseBtn.addEventListener('click', togglePlayPause);
  elements.stopBtn.addEventListener('click', stopPlayback);
  elements.progressBar.addEventListener('input', seekTo);

  // Keyboard shortcuts
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && elements.movieModal.classList.contains('active')) {
      closeModal();
    }
  });
}

// Load movies
async function loadMovies() {
  try {
    state.movies = await api.getMovies();
    renderMovies();
  } catch (error) {
    console.error('Failed to load movies:', error);
    showToast('Failed to load movies', 'error');
  }
}

// Render movies
function renderMovies() {
  const filtered = state.movies.filter(movie =>
    movie.title.toLowerCase().includes(state.searchQuery)
  );

  if (filtered.length === 0) {
    elements.movieGrid.innerHTML = '';
    elements.emptyState.style.display = 'flex';
    return;
  }

  elements.emptyState.style.display = 'none';
  elements.movieGrid.innerHTML = filtered.map(movie => `
    <div class="movie-card" data-id="${movie.id}">
      ${movie.thumbnail_path
      ? `<img class="movie-poster" src="${api.getMovieThumbnailURL(movie.id)}" alt="${escapeHtml(movie.title)}" loading="lazy">`
      : `<div class="movie-poster-placeholder">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <rect x="2" y="2" width="20" height="20" rx="2.18" ry="2.18"/>
              <line x1="7" y1="2" x2="7" y2="22"/>
              <line x1="17" y1="2" x2="17" y2="22"/>
              <line x1="2" y1="12" x2="22" y2="12"/>
              <line x1="2" y1="7" x2="7" y2="7"/>
              <line x1="2" y1="17" x2="7" y2="17"/>
              <line x1="17" y1="17" x2="22" y2="17"/>
              <line x1="17" y1="7" x2="22" y2="7"/>
            </svg>
            <span>${escapeHtml(movie.title)}</span>
          </div>`
    }
      <div class="movie-info">
        <div class="movie-title">${escapeHtml(movie.title)}</div>
        <div class="movie-meta">
          ${movie.year ? `<span class="movie-year">${movie.year}</span>` : ''}
          <span>${api.formatDuration(movie.duration)}</span>
          ${movie.has_subtitles ? `
            <span class="movie-badge">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="2" y="4" width="20" height="16" rx="2"/>
                <path d="M6 12h4M14 12h4M6 16h12"/>
              </svg>
              CC
            </span>
          ` : ''}
        </div>
      </div>
    </div>
  `).join('');

  // Add click handlers
  document.querySelectorAll('.movie-card').forEach(card => {
    card.addEventListener('click', () => openMovieDetail(card.dataset.id));
  });
}

// Open movie detail
async function openMovieDetail(movieId) {
  try {
    const movie = await api.getMovie(movieId);
    state.currentMovie = movie;
    state.selectedSubtitle = null;
    renderMovieDetail(movie);
    elements.movieModal.classList.add('active');
  } catch (error) {
    showToast('Failed to load movie details', 'error');
  }
}

// Render movie detail
function renderMovieDetail(movie) {
  elements.movieDetail.innerHTML = `
    <div class="movie-detail-header">
      <div class="movie-detail-backdrop" style="background-image: url('${api.getMovieThumbnailURL(movie.id)}')"></div>
      <div class="movie-detail-content">
        ${movie.thumbnail_path
      ? `<img class="movie-detail-poster" src="${api.getMovieThumbnailURL(movie.id)}" alt="${escapeHtml(movie.title)}">`
      : `<div class="movie-detail-poster movie-poster-placeholder">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="2" y="2" width="20" height="20" rx="2.18" ry="2.18"/>
              </svg>
            </div>`
    }
        <div class="movie-detail-info">
          <h2 class="movie-detail-title">${escapeHtml(movie.title)}</h2>
          <div class="movie-detail-meta">
            ${movie.year ? `<span>${movie.year}</span>` : ''}
            <span>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10"/>
                <polyline points="12 6 12 12 16 14"/>
              </svg>
              ${api.formatDuration(movie.duration)}
            </span>
            <span>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/>
                <polyline points="14 2 14 8 20 8"/>
              </svg>
              ${api.formatFileSize(movie.file_size)}
            </span>
          </div>
          <div class="movie-detail-tech">
            <span class="tech-badge">${movie.video_codec?.toUpperCase() || 'Unknown'}</span>
            <span class="tech-badge">${movie.video_width}×${movie.video_height}</span>
            <span class="tech-badge">${movie.audio_codec?.toUpperCase() || 'Unknown'}</span>
            ${movie.audio_channels ? `<span class="tech-badge">${movie.audio_channels}ch</span>` : ''}
          </div>
        </div>
      </div>
    </div>
    
    ${movie.subtitles && movie.subtitles.length > 0 ? `
      <div class="movie-detail-body">
        <div class="movie-detail-section">
          <h3>Subtitles</h3>
          <div class="subtitle-list">
            <label class="subtitle-item">
              <input type="radio" name="subtitle" value="" checked>
              <span>No subtitles</span>
            </label>
            ${movie.subtitles.map((sub, idx) => `
              <label class="subtitle-item" data-path="${escapeHtml(sub.file_path || '')}" data-index="${sub.index}">
                <input type="radio" name="subtitle" value="${idx}">
                <span>${sub.language ? sub.language.toUpperCase() : 'Unknown'} ${sub.is_external ? '(External SRT)' : '(Embedded)'}</span>
              </label>
            `).join('')}
          </div>
        </div>
      </div>
    ` : ''}
    
    <div class="cast-section">
      <div class="device-select">
        <label>Cast to device</label>
        <select id="deviceSelect">
          ${state.devices.length > 0
      ? state.devices.map(d => `<option value="${d.uuid}">${escapeHtml(d.friendly_name || d.uuid)}</option>`).join('')
      : '<option value="">No devices found</option>'
    }
        </select>
      </div>
      <div class="cast-actions">
        <button class="btn btn-primary btn-lg" id="castBtn" ${state.devices.length === 0 ? 'disabled' : ''}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polygon points="5 3 19 12 5 21 5 3"/>
          </svg>
          Cast to TV
        </button>
        <button class="btn btn-secondary btn-lg" id="castWithSubBtn" ${state.devices.length === 0 ? 'disabled' : ''}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <rect x="2" y="4" width="20" height="16" rx="2"/>
            <path d="M6 12h4M14 12h4M6 16h12"/>
          </svg>
          Cast with Subtitles
        </button>
      </div>
    </div>
  `;

  // Subtitle selection
  document.querySelectorAll('.subtitle-item').forEach(item => {
    item.addEventListener('click', () => {
      document.querySelectorAll('.subtitle-item').forEach(i => i.classList.remove('selected'));
      item.classList.add('selected');
      state.selectedSubtitle = {
        path: item.dataset.path,
        index: parseInt(item.dataset.index) || -1
      };
    });
  });

  // Cast button
  document.getElementById('castBtn').addEventListener('click', () => castMovie(false));
  document.getElementById('castWithSubBtn').addEventListener('click', () => castMovie(true));
}

// Cast movie
async function castMovie(withSubtitles) {
  const deviceSelect = document.getElementById('deviceSelect');
  const deviceUuid = deviceSelect.value;

  if (!deviceUuid || !state.currentMovie) {
    showToast('Please select a device', 'error');
    return;
  }

  try {
    const options = {
      transcode: withSubtitles,
    };

    if (withSubtitles && state.selectedSubtitle) {
      if (state.selectedSubtitle.path) {
        options.subtitlePath = state.selectedSubtitle.path;
      } else if (state.selectedSubtitle.index >= 0) {
        options.subtitleIndex = state.selectedSubtitle.index;
      }
    }

    await api.castMovie(state.currentMovie.id, deviceUuid, options);

    state.isPlaying = true;
    state.playingMovieId = state.currentMovie.id;
    state.playingDeviceId = deviceUuid;

    elements.npTitle.textContent = state.currentMovie.title;
    elements.npDevice.textContent = `Playing on ${getDeviceName(deviceUuid)}`;
    elements.nowPlaying.style.display = 'block';

    showToast('Casting to TV...', 'success');
    closeModal();
  } catch (error) {
    showToast('Failed to cast: ' + error.message, 'error');
  }
}

// Load devices
async function loadDevices() {
  try {
    state.devices = await api.getDevices();
    renderDevices();
  } catch (error) {
    console.error('Failed to load devices:', error);
  }
}

// Render devices
function renderDevices() {
  if (state.devices.length === 0) {
    elements.deviceList.innerHTML = '';
    elements.noDevices.style.display = 'flex';
    return;
  }

  elements.noDevices.style.display = 'none';
  elements.deviceList.innerHTML = state.devices.map(device => `
    <div class="device-card" data-uuid="${device.uuid}">
      <div class="device-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <rect x="2" y="7" width="20" height="15" rx="2" ry="2"/>
          <polyline points="17 2 12 7 7 2"/>
        </svg>
      </div>
      <div class="device-info">
        <div class="device-name">${escapeHtml(device.friendly_name || device.uuid)}</div>
        <div class="device-type">${device.manufacturer || 'DLNA Renderer'}</div>
      </div>
      <div class="device-status">Available</div>
    </div>
  `).join('');
}

// Close modal
function closeModal() {
  elements.movieModal.classList.remove('active');
  state.currentMovie = null;
}

// Playback controls
async function togglePlayPause() {
  if (!state.playingDeviceId) return;

  try {
    const action = state.isPlaying ? 'pause' : 'play';
    await api.controlPlayback(state.playingDeviceId, action);
    state.isPlaying = !state.isPlaying;
    updatePlayPauseButton();
  } catch (error) {
    showToast('Control failed: ' + error.message, 'error');
  }
}

async function stopPlayback() {
  if (!state.playingDeviceId) return;

  try {
    await api.controlPlayback(state.playingDeviceId, 'stop');
    state.isPlaying = false;
    state.playingMovieId = null;
    state.playingDeviceId = null;
    elements.nowPlaying.style.display = 'none';
  } catch (error) {
    showToast('Stop failed: ' + error.message, 'error');
  }
}

async function seekTo() {
  if (!state.playingDeviceId) return;

  const percent = elements.progressBar.value;
  const movie = state.movies.find(m => m.id === state.playingMovieId);
  if (!movie) return;

  const seconds = Math.floor(movie.duration * (percent / 100));
  const position = formatTimeForSeek(seconds);

  try {
    await api.controlPlayback(state.playingDeviceId, 'seek', position);
  } catch (error) {
    console.error('Seek failed:', error);
  }
}

async function updatePlaybackStatus() {
  if (!state.playingDeviceId) return;

  try {
    const status = await api.getPlaybackStatus(state.playingDeviceId);

    if (status.transport_state === 'STOPPED') {
      state.isPlaying = false;
      state.playingMovieId = null;
      state.playingDeviceId = null;
      elements.nowPlaying.style.display = 'none';
      return;
    }

    state.isPlaying = status.transport_state === 'PLAYING';
    updatePlayPauseButton();

    const current = api.parseDuration(status.current_position);
    const duration = api.parseDuration(status.duration);

    if (duration > 0) {
      elements.progressBar.value = (current / duration) * 100;
      elements.timeDisplay.textContent = `${status.current_position} / ${status.duration}`;
    }
  } catch (error) {
    // Silently fail - device might be unavailable
  }
}

function updatePlayPauseButton() {
  const icon = state.isPlaying
    ? '<rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/>'
    : '<polygon points="5 3 19 12 5 21 5 3"/>';
  elements.playPauseBtn.innerHTML = `<svg viewBox="0 0 24 24" fill="currentColor">${icon}</svg>`;
}

// Helper functions
function getDeviceName(uuid) {
  const device = state.devices.find(d => d.uuid === uuid);
  return device ? (device.friendly_name || uuid) : uuid;
}

function formatTimeForSeek(seconds) {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = seconds % 60;
  return `${hours}:${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
}

function escapeHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

// Toast notifications
function showToast(message, type = 'info') {
  const toast = document.createElement('div');
  toast.className = `toast ${type}`;

  const icon = type === 'success'
    ? '<path d="M20 6L9 17l-5-5"/>'
    : '<circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>';

  toast.innerHTML = `
    <svg class="toast-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">${icon}</svg>
    <span>${escapeHtml(message)}</span>
  `;

  elements.toastContainer.appendChild(toast);

  setTimeout(() => {
    toast.style.animation = 'slideIn 0.3s ease reverse';
    setTimeout(() => toast.remove(), 300);
  }, 3000);
}

// Initialize on DOM ready
document.addEventListener('DOMContentLoaded', init);

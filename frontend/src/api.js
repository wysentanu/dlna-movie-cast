/**
 * API Client for DLNA Movie Cast
 */

const API_BASE = '/api';

/**
 * Fetch with error handling
 */
async function fetchJSON(url, options = {}) {
    const response = await fetch(url, {
        ...options,
        headers: {
            'Content-Type': 'application/json',
            ...options.headers,
        },
    });

    if (!response.ok) {
        const error = await response.text();
        throw new Error(error || `HTTP ${response.status}`);
    }

    return response.json();
}

/**
 * Get all movies
 */
export async function getMovies() {
    return fetchJSON(`${API_BASE}/movies`);
}

/**
 * Get a single movie by ID
 */
export async function getMovie(id) {
    return fetchJSON(`${API_BASE}/movies/${id}`);
}

/**
 * Get movie thumbnail URL
 */
export function getMovieThumbnailURL(id) {
    return `${API_BASE}/movies/${id}/thumbnail`;
}

/**
 * Get all DLNA devices
 */
export async function getDevices() {
    return fetchJSON(`${API_BASE}/devices`);
}

/**
 * Refresh DLNA devices
 */
export async function refreshDevices() {
    return fetchJSON(`${API_BASE}/devices/refresh`, { method: 'POST' });
}

/**
 * Cast a movie to a device
 */
export async function castMovie(movieId, deviceUuid, options = {}) {
    return fetchJSON(`${API_BASE}/cast`, {
        method: 'POST',
        body: JSON.stringify({
            movie_id: movieId,
            device_uuid: deviceUuid,
            subtitle_path: options.subtitlePath || '',
            subtitle_index: options.subtitleIndex || 0,
            transcode: options.transcode || false,
        }),
    });
}

/**
 * Control playback
 */
export async function controlPlayback(deviceUuid, action, position = null) {
    const body = {
        device_uuid: deviceUuid,
        action: action,
    };

    if (position !== null) {
        body.position = position;
    }

    return fetchJSON(`${API_BASE}/cast/control`, {
        method: 'POST',
        body: JSON.stringify(body),
    });
}

/**
 * Get playback status
 */
export async function getPlaybackStatus(deviceUuid) {
    return fetchJSON(`${API_BASE}/cast/control?device_uuid=${deviceUuid}`);
}

/**
 * Trigger library scan
 */
export async function scanLibrary() {
    return fetchJSON(`${API_BASE}/scan`, { method: 'POST' });
}

/**
 * Format duration from seconds to HH:MM:SS
 */
export function formatDuration(seconds) {
    if (!seconds || seconds <= 0) return '0:00';

    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const secs = Math.floor(seconds % 60);

    if (hours > 0) {
        return `${hours}:${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
    }
    return `${minutes}:${secs.toString().padStart(2, '0')}`;
}

/**
 * Parse HH:MM:SS to seconds
 */
export function parseDuration(timeStr) {
    if (!timeStr) return 0;

    const parts = timeStr.split(':').map(Number);
    if (parts.length === 3) {
        return parts[0] * 3600 + parts[1] * 60 + parts[2];
    } else if (parts.length === 2) {
        return parts[0] * 60 + parts[1];
    }
    return parts[0] || 0;
}

/**
 * Format file size
 */
export function formatFileSize(bytes) {
    if (!bytes) return '0 B';

    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let index = 0;
    let size = bytes;

    while (size >= 1024 && index < units.length - 1) {
        size /= 1024;
        index++;
    }

    return `${size.toFixed(1)} ${units[index]}`;
}

// Photos load from their publishers through DuckDuckGo's image proxy (src/lib/media.js). When the
// proxy cannot fetch one it answers with its own 260×180 "no image" picture (HTTP 400), which the
// browser draws without an error: proxyFailed() recognises it. The reader may then load the photo
// straight from its publisher; that choice is remembered per domain in this browser only.
export const proxyFailed = (img) => /external-content\.duckduckgo\.com/.test(img.currentSrc || img.src) && img.naturalWidth === 260 && img.naturalHeight === 180;

const KEY = "tracker:media-hosts";
export const allowedHosts = () => { try { return new Set(JSON.parse(localStorage.getItem(KEY) || "[]")); } catch { return new Set(); } };
export const allowHost = (host) => { try { const s = allowedHosts(); s.add(host); localStorage.setItem(KEY, JSON.stringify([...s])); } catch { /* private mode: this page only */ } };
export const isAllowed = (host) => !!host && allowedHosts().has(host);

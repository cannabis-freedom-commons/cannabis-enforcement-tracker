// How a report's photo or video is shown on its page: a photo with a direct address as a picture, a
// video file as a player, a YouTube or Vimeo video as their embedded player; anything else stays a
// link. Pictures and videos load from their publishers (docs/WRITING.md §7); lists and larger
// photo grids use our 320px thumbnails where there is one (src/lib/media-manifest.js).
// Every outside picture is loaded through DuckDuckGo's image proxy: readers' browsers never contact
// the publisher (no address or referrer reaches it), and sites that refuse hotlinking still show.
// Our own images (a path on this site) are left as they are.
export const imageProxy = (url) => (/^https?:\/\//.test(url ?? "") ? `https://external-content.duckduckgo.com/iu/?u=${encodeURIComponent(url)}&f=1` : url);

const web = (u) => { try { const url = new URL(u); return /^https?:$/.test(url.protocol) ? url : null; } catch { return null; } };

export function mediaView(m) {
  const url = web(m?.media_url || "");
  if (!url) return null;
  if (m.kind === "photo") return { type: "photo", src: imageProxy(url.href) };
  if (m.kind !== "video") return null;
  if (/\.(mp4|webm|ogv|mov|m4v)$/i.test(url.pathname)) return { type: "video", src: url.href };
  const host = url.hostname.replace(/^(www|m)\./, "");
  let id = null;
  if (host === "youtu.be") id = url.pathname.slice(1);
  else if (host === "youtube.com" || host === "youtube-nocookie.com") id = url.searchParams.get("v") || url.pathname.match(/^\/(?:embed|shorts|live)\/([^/]+)/)?.[1];
  if (id && /^[\w-]{6,20}$/.test(id)) return { type: "embed", src: `https://www.youtube-nocookie.com/embed/${id}` };
  const vimeo = (host === "vimeo.com" || host === "player.vimeo.com") && url.pathname.match(/(?:^|\/)(\d{5,12})(?:$|\/)/)?.[1];
  if (vimeo) return { type: "embed", src: `https://player.vimeo.com/video/${vimeo}?dnt=1` };
  return null;
}

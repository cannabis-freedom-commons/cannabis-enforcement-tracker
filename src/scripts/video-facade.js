// A video's poster becomes its player only when the reader clicks play (VideoFacade.astro): until
// then neither YouTube, Vimeo nor the publisher is contacted. It then starts playing at once.
document.addEventListener("click", (e) => {
  const facade = e.target.closest?.(".video-play")?.closest(".video-facade");
  if (!facade) return;
  const { kind, src, title } = facade.dataset;
  let player;
  if (kind === "embed") {
    player = Object.assign(document.createElement("iframe"), { src: `${src}${src.includes("?") ? "&" : "?"}autoplay=1`, title, allowFullscreen: true });
    player.allow = "autoplay; fullscreen; picture-in-picture";
    player.referrerPolicy = "strict-origin-when-cross-origin";
  } else {
    player = Object.assign(document.createElement("video"), { src, controls: true, autoplay: true, playsInline: true });
    player.setAttribute("referrerpolicy", "no-referrer");
    player.setAttribute("aria-label", title);
    player.addEventListener("error", () => {
      const note = Object.assign(document.createElement("p"), { className: "video-error", textContent: "The video could not be played here; it may still play on its source page." });
      player.replaceWith(note);
    });
  }
  facade.replaceWith(player);
  player.focus?.();
});

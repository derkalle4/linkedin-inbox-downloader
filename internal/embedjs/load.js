() => {
  if (window.__lidJob && !window.__lidJob.done) return false;
  window.__lidJob = {
    kind: 'load',
    count: 0,
    round: 0,
    maxRounds: 180,
    done: false,
    error: null,
    result: null,
  };
  (async () => {
    const job = window.__lidJob;
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
    const jitter = (lo, hi) => lo + Math.floor(Math.random() * (hi - lo + 1));
    try {
      const box = document.querySelector('.msg-thread .msg-s-message-list.scrollable')
        || document.querySelector('.msg-s-message-list.scrollable')
        || document.querySelector('.msg-s-message-list');
      if (!box) throw new Error('No chat pane found.');

      box.querySelectorAll('.lt-line-clamp__more, .inline-show-more-text__button, button.see-more')
        .forEach((b) => { try { b.click(); } catch (e) {} });

      let last = -1;
      let stable = 0;
      for (let i = 0; i < job.maxRounds; i++) {
        job.round = i + 1;
        const loader = box.querySelector('.msg-s-message-list__loader:not(.hidden)');
        box.scrollTop = 0;
        await sleep(loader ? jitter(1500, 2500) : jitter(800, 1500));
        // Occasional extra pause so scroll cadence looks less robotic.
        if (i > 0 && i % 7 === 0) {
          await sleep(jitter(400, 1200));
        }
        const n = box.querySelectorAll('li.msg-s-message-list__event .msg-s-event-listitem').length;
        job.count = Math.max(0, n);
        if (n === last && !loader && box.scrollTop <= 2) {
          stable += 1;
          if (stable >= 4) break;
        } else {
          stable = 0;
          last = n;
        }
        box.scrollTop = Math.min(40, box.scrollHeight);
        await sleep(jitter(40, 120));
      }
      box.scrollTop = box.scrollHeight;
      await sleep(jitter(200, 400));
      // Swap GhostImage placeholders for the real CDN URL (attributes only).
      const reveal = (img) => {
        if (!img) return;
        img.loading = 'eager';
        img.removeAttribute('loading');
        const url = [
          img.getAttribute('data-delayed-url'),
          img.getAttribute('data-ghost-url'),
          img.getAttribute('data-src'),
          img.dataset?.delayedUrl,
          img.dataset?.ghostUrl,
        ].find((u) => u && !u.startsWith('data:'));
        if (url && img.src !== url) img.src = url;
      };
      const events = [...box.querySelectorAll('li.msg-s-message-list__event')];
      const step = Math.max(1, Math.floor(events.length / 24));
      for (let i = 0; i < events.length; i += step) {
        try {
          events[i].scrollIntoView({ block: 'center' });
        } catch (e) {}
        events[i].querySelectorAll('img.msg-s-event-listitem__profile-picture, img.presence-entity__image, img.evi-image')
          .forEach(reveal);
        await sleep(jitter(40, 90));
      }
      document.querySelectorAll('.msg-s-profile-card img, img.msg-s-event-listitem__profile-picture')
        .forEach(reveal);
      box.scrollTop = box.scrollHeight;
      await sleep(jitter(400, 800));
      job.count = Math.max(0, last);
      job.result = last;
      job.done = true;
    } catch (e) {
      job.error = String(e && e.message ? e.message : e);
      job.done = true;
    }
  })();
  return true;
}

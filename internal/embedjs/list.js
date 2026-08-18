() => {
  if (window.__lidJob && !window.__lidJob.done) return false;
  window.__lidJob = {
    kind: 'list',
    found: 0,
    round: 0,
    maxRounds: 80,
    done: false,
    error: null,
    result: null,
  };
  (async () => {
    const job = window.__lidJob;
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
    const jitter = (lo, hi) => lo + Math.floor(Math.random() * (hi - lo + 1));
    try {
      const list = document.querySelector('ul.msg-conversations-container__conversations-list');
      if (!list) throw new Error('Conversation list not found.');

      const scrollerOf = (el) => {
        let n = el;
        while (n && n !== document.body) {
          const oy = getComputedStyle(n).overflowY;
          if ((oy === 'auto' || oy === 'scroll') && n.scrollHeight > n.clientHeight + 8) return n;
          n = n.parentElement;
        }
        return el.parentElement || el;
      };
      const scroller = scrollerOf(list);

      const harvest = (into) => {
        for (const li of list.querySelectorAll('li.msg-conversation-listitem')) {
          const name = li.querySelector('.msg-conversation-listitem__participant-names')?.innerText?.trim();
          if (!name) continue;
          const time = li.querySelector('.msg-conversation-listitem__time-stamp')?.innerText?.trim() || '';
          const snippet = li.querySelector('.msg-conversation-card__message-snippet')?.innerText?.trim() || '';
          const photo = li.querySelector('.msg-facepile-grid__img')?.currentSrc
            || li.querySelector('img')?.currentSrc || '';
          const link = li.querySelector('a.msg-conversation-listitem__link') || li.querySelector('a[href*="/messaging/thread/"]');
          const href = link?.href || link?.getAttribute?.('href') || '';
          const key = href || [name, time, snippet].join('\n');
          if (!into.has(key)) into.set(key, { name, time, snippet, photo, key, href });
        }
      };

      const seen = new Map();
      harvest(seen);
      job.found = seen.size;
      let stable = 0;
      let last = -1;
      for (let i = 0; i < job.maxRounds; i++) {
        job.round = i + 1;
        const more = [...document.querySelectorAll('button')].find((b) =>
          /Weitere Unterhaltungen laden|Load more conversations|See more/i.test(b.innerText || ''));
        if (more) {
          more.click();
          await sleep(jitter(1200, 2000));
        }
        scroller.scrollTop = scroller.scrollHeight;
        await sleep(jitter(800, 1600));
        harvest(seen);
        job.found = seen.size;
        if (seen.size === last && !more) {
          stable += 1;
          if (stable >= 3) break;
        } else {
          stable = 0;
          last = seen.size;
        }
      }
      scroller.scrollTop = 0;
      await sleep(jitter(200, 500));
      harvest(seen);
      job.found = seen.size;
      job.result = [...seen.values()];
      job.done = true;
    } catch (e) {
      job.error = String(e && e.message ? e.message : e);
      job.done = true;
    }
  })();
  return true;
}

() => {
  if (window.__lidJob && !window.__lidJob.done) return false;
  window.__lidJob = {
    kind: 'list',
    found: 0,
    round: 0,
    maxRounds: 160,
    done: false,
    error: null,
    result: null,
  };
  (async () => {
    const job = window.__lidJob;
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
    const jitter = (lo, hi) => lo + Math.floor(Math.random() * (hi - lo + 1));

    const PARTITION_RE = /^(Focused|Other|Fokussiert|Wichtig|Relevant|Andere|Sonstige)$/i;
    const SKIP_RE = /Unread|Ungelesen|Starred|Favorit|InMail|Jobs|Archive|Archiv/i;
    const ALL_RE = /^(All|Alle)$/i;
    const LOAD_MORE_RE = /Weitere Unterhaltungen laden|Load more conversations|See more|Mehr anzeigen|Mehr laden/i;

    try {
      const container = document.querySelector('.msg-conversations-container') || document;
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

      const labelOf = (el) => {
        const t = (el.innerText || '').replace(/\s+/g, ' ').trim();
        const a = (el.getAttribute('aria-label') || '').replace(/\s+/g, ' ').trim();
        return t || a;
      };

      const findTabs = () => {
        const roots = [
          container,
          document.querySelector('.msg-overlay-list-bubble'),
          document,
        ].filter(Boolean);
        const found = [];
        const seen = new Set();
        for (const root of roots) {
          for (const el of root.querySelectorAll('button, [role="tab"], a[role="tab"]')) {
            const label = labelOf(el);
            if (!label || seen.has(label.toLowerCase())) continue;
            if (SKIP_RE.test(label)) continue;
            // Strip unread counts like "Focused (3)" / "Andere 2"
            const base = label.replace(/\s*[\(\[]?\d+[\)\]]?\s*$/, '').trim();
            if (PARTITION_RE.test(base) || PARTITION_RE.test(label.split(/\s+/)[0])) {
              seen.add(label.toLowerCase());
              found.push({ el, label: base || label });
            }
          }
        }
        return found;
      };

      const findAllPill = () => {
        for (const el of container.querySelectorAll('button, [role="tab"], a[role="tab"]')) {
          const label = labelOf(el).replace(/\s*[\(\[]?\d+[\)\]]?\s*$/, '').trim();
          if (ALL_RE.test(label)) return el;
        }
        return null;
      };

      const loadMoreButton = () => [...document.querySelectorAll('button, [role="button"]')].find((b) => {
        const text = `${b.innerText || ''} ${b.getAttribute('aria-label') || ''}`;
        return LOAD_MORE_RE.test(text);
      });

      const listLoaderVisible = (scroller) => {
        const root = scroller || list.parentElement || list;
        const loader = root.querySelector(
          '.msg-conversations-container__convo-item-list-loader:not(.hidden), '
          + '.artdeco-loader:not(.hidden), '
          + '[class*="loader"]:not(.hidden)',
        );
        if (!loader) return false;
        const style = getComputedStyle(loader);
        return style.display !== 'none' && style.visibility !== 'hidden' && loader.offsetParent !== null;
      };

      const harvest = (into) => {
        for (const li of list.querySelectorAll('li.msg-conversation-listitem')) {
          const name = (
            li.querySelector('.msg-conversation-listitem__participant-names')?.innerText
            || li.querySelector('.msg-conversation-card__participant-names')?.innerText
            || ''
          ).trim();
          const link = li.querySelector('a.msg-conversation-listitem__link')
            || li.querySelector('a[href*="/messaging/thread/"]');
          const href = link?.href || link?.getAttribute?.('href') || '';
          if (!name && !href) continue;
          const time = li.querySelector('.msg-conversation-listitem__time-stamp')?.innerText?.trim() || '';
          const snippet = li.querySelector('.msg-conversation-card__message-snippet')?.innerText?.trim() || '';
          const photo = li.querySelector('.msg-facepile-grid__img')?.currentSrc
            || li.querySelector('img')?.currentSrc || '';
          const displayName = name || 'Unknown';
          const key = href || [displayName, time, snippet].join('\n');
          if (!into.has(key)) into.set(key, { name: displayName, time, snippet, photo, key, href });
        }
      };

      const paginateCurrent = async (into) => {
        const scroller = scrollerOf(list);
        scroller.scrollTop = 0;
        await sleep(jitter(300, 600));
        harvest(into);
        job.found = into.size;

        let stable = 0;
        let last = into.size;
        const maxRounds = Math.max(40, Math.floor(job.maxRounds / 2));
        for (let i = 0; i < maxRounds; i++) {
          job.round += 1;
          const more = loadMoreButton();
          if (more) {
            more.click();
            await sleep(jitter(1200, 2000));
          }
          const step = Math.max(200, Math.floor(scroller.clientHeight * 0.85));
          const before = scroller.scrollTop;
          scroller.scrollTop = Math.min(scroller.scrollHeight, before + step);
          // If already at bottom, nudge once more to trigger lazy load.
          if (scroller.scrollTop === before || scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - 4) {
            scroller.scrollTop = scroller.scrollHeight;
          }
          await sleep(jitter(900, 1700));
          if (listLoaderVisible(scroller)) {
            await sleep(jitter(800, 1400));
          }
          harvest(into);
          job.found = into.size;
          const loading = !!loadMoreButton() || listLoaderVisible(scroller);
          if (into.size === last && !loading) {
            stable += 1;
            if (stable >= 6) break;
          } else {
            stable = 0;
            last = into.size;
          }
        }
        scroller.scrollTop = 0;
        await sleep(jitter(200, 500));
        harvest(into);
        job.found = into.size;
      };

      const seen = new Map();
      const tabs = findTabs();
      const hasOther = tabs.some((t) => /^(Other|Andere|Sonstige)$/i.test(t.label));

      if (!hasOther) {
        const allPill = findAllPill();
        if (allPill) {
          allPill.click();
          await sleep(jitter(800, 1400));
        }
      }

      if (tabs.length > 0) {
        for (const tab of tabs) {
          tab.el.click();
          await sleep(jitter(1000, 1800));
          await paginateCurrent(seen);
        }
      } else {
        await paginateCurrent(seen);
      }

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

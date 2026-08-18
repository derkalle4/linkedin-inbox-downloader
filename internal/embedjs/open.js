async (want) => {
  // Bounded fallback when no thread href was harvested: click a visible row
  // that matches name/time/snippet. Does NOT click "Load more" or re-paginate.
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const list = document.querySelector('ul.msg-conversations-container__conversations-list');
  if (!list) return false;

  if (want && want.href) {
    const abs = want.href;
    const link = [...list.querySelectorAll('a.msg-conversation-listitem__link, a[href*="/messaging/thread/"]')]
      .find((a) => a.href === abs || (a.getAttribute('href') || '') === abs
        || (a.href && abs && a.href.indexOf(abs) !== -1));
    if (link) {
      link.click();
      return true;
    }
  }

  const match = (li) => {
    const name = li.querySelector('.msg-conversation-listitem__participant-names')?.innerText?.trim() || '';
    const time = li.querySelector('.msg-conversation-listitem__time-stamp')?.innerText?.trim() || '';
    const snippet = li.querySelector('.msg-conversation-card__message-snippet')?.innerText?.trim() || '';
    return name === want.name && time === want.time && snippet === want.snippet;
  };

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
  scroller.scrollTop = 0;
  await sleep(200);

  // At most a few scroll steps over already-loaded rows — never "Load more".
  for (let i = 0; i < 12; i++) {
    const li = [...list.querySelectorAll('li.msg-conversation-listitem')].find(match);
    if (li) {
      const link = li.querySelector('.msg-conversation-listitem__link') || li;
      link.click();
      return true;
    }
    const before = scroller.scrollTop;
    scroller.scrollTop += Math.max(280, Math.floor(scroller.clientHeight * 0.7));
    await sleep(250);
    if (scroller.scrollTop === before || scroller.scrollTop + scroller.clientHeight >= scroller.scrollHeight - 8) {
      break;
    }
  }
  return false;
}

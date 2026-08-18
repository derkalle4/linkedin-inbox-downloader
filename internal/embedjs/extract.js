async () => {
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const isPlaceholder = (src) => {
    if (!src) return true;
    if (src.startsWith('data:image/gif')) return true;
    if (/placeholder|spacer|blank|transparent/i.test(src)) return true;
    // 1×1 / tiny data GIFs LinkedIn uses before GhostImage swaps in.
    if (src.startsWith('data:') && src.length < 80) return true;
    return false;
  };

  const imgURL = (img) => {
    if (!img) return '';
    const attrs = [
      img.currentSrc,
      img.src,
      img.getAttribute?.('src'),
      img.getAttribute?.('data-delayed-url'),
      img.getAttribute?.('data-ghost-url'),
      img.getAttribute?.('data-src'),
      img.getAttribute?.('data-original'),
      img.dataset?.delayedUrl,
      img.dataset?.ghostUrl,
      img.dataset?.src,
    ];
    for (const a of attrs) {
      const s = (a || '').trim();
      if (s && !isPlaceholder(s)) return s;
    }
    const srcset = img.getAttribute?.('srcset') || img.srcset || '';
    if (srcset) {
      const parts = srcset.split(',').map((p) => p.trim().split(/\s+/)[0]).filter(Boolean);
      for (let i = parts.length - 1; i >= 0; i--) {
        if (!isPlaceholder(parts[i])) return parts[i];
      }
    }
    return '';
  };

  const toData = async (src) => {
    if (!src || isPlaceholder(src)) return '';
    if (src.startsWith('data:')) return src;
    // Prefer HTTP cache — avatars were usually already painted in the thread.
    for (const init of [
      { credentials: 'include', mode: 'cors', cache: 'force-cache' },
      { credentials: 'include', mode: 'cors', cache: 'default' },
      { credentials: 'omit', mode: 'cors', cache: 'force-cache' },
    ]) {
      try {
        const r = await fetch(src, init);
        if (!r.ok) continue;
        const buf = await r.arrayBuffer();
        if (!buf || buf.byteLength < 32) continue;
        let mime = (r.headers.get('content-type') || 'image/jpeg').split(';')[0].trim();
        if (!mime.startsWith('image/')) mime = 'image/jpeg';
        const bytes = new Uint8Array(buf);
        let bin = '';
        const step = 0x8000;
        for (let i = 0; i < bytes.length; i += step) {
          bin += String.fromCharCode.apply(null, bytes.subarray(i, i + step));
        }
        return `data:${mime};base64,` + btoa(bin);
      } catch (e) {
        /* try next */
      }
    }
    // Never return raw https — PDF is printed from a data: document and
    // cannot load LinkedIn CDN URLs. Empty → initials fallback in the PDF.
    return '';
  };

  const toDataAll = async (urls) => {
    const out = [];
    for (const u of urls) {
      out.push(await toData(u));
    }
    return out;
  };

  const root = document.querySelector('.msg-thread') || document;
  const box = root.querySelector('.msg-s-message-list.scrollable')
    || root.querySelector('.msg-s-message-list')
    || root;

  // Scroll every event into view so GhostImage / lazy avatars resolve before we read them.
  const events = [...box.querySelectorAll('li.msg-s-message-list__event')];
  for (let i = 0; i < events.length; i++) {
    try {
      events[i].scrollIntoView({ block: 'nearest', inline: 'nearest' });
    } catch (e) {}
    if (i % 8 === 0) await sleep(40);
  }
  await sleep(200);
  const pending = [...box.querySelectorAll('img')].map((img) => {
    try {
      if (img.decode) return img.decode().catch(() => {});
    } catch (e) {}
    return Promise.resolve();
  });
  await Promise.all(pending);
  await sleep(100);

  const card = root.querySelector('.msg-s-profile-card');
  const lockup = root.querySelector('.msg-entity-lockup');
  const name = (lockup?.querySelector('.msg-entity-lockup__entity-title')
    || card?.querySelector('.profile-card-one-to-one__profile-link, .artdeco-entity-lockup__title')
    || root.querySelector('.msg-thread__link-to-profile h2'))
    ?.innerText?.replace(/\s+/g, ' ').trim() || 'LinkedIn-Unterhaltung';
  const headline = (lockup?.querySelector('.msg-entity-lockup__entity-info')
    || card?.querySelector('.artdeco-entity-lockup__subtitle'))
    ?.innerText?.replace(/\s+/g, ' ').trim() || '';
  const degree = (card?.querySelector('.artdeco-entity-lockup__degree')
    || card?.querySelector('.artdeco-entity-lockup__badge'))
    ?.innerText?.replace(/\s+/g, ' ').trim() || '';
  const profileUrl = (root.querySelector('a.msg-thread__link-to-profile')
    || card?.querySelector('a[href*="/in/"]'))?.href || '';
  let photoSrc = imgURL(card?.querySelector('img.presence-entity__image, img.evi-image'))
    || imgURL(root.querySelector('.msg-s-event-listitem--other img.msg-s-event-listitem__profile-picture'))
    || imgURL(root.querySelector('img.msg-s-event-listitem__profile-picture'));
  const photo = await toData(photoSrc);

  const items = [];
  let lastOtherPhoto = photo;
  let lastSelfPhoto = '';
  for (const li of root.querySelectorAll('li.msg-s-message-list__event')) {
    const heading = li.querySelector('.msg-s-message-list__time-heading')?.innerText?.trim() || '';
    const item = li.querySelector('.msg-s-event-listitem');
    if (!item) {
      if (heading) items.push({ type: 'day', heading });
      continue;
    }
    const body = item.querySelector('.msg-s-event-listitem__body, .msg-s-event__content');
    const imgUrls = [...item.querySelectorAll('.msg-s-event-listitem__message-bubble img, .msg-s-event__content img')]
      .map((img) => imgURL(img))
      .filter((src) => src && !isPlaceholder(src)
        && !/profile-displayphoto|EntityPhoto|presence-entity|msg-s-event-listitem__profile-picture/.test(src));
    const self = !item.classList.contains('msg-s-event-listitem--other');
    let senderPhoto = await toData(imgURL(item.querySelector('img.msg-s-event-listitem__profile-picture')));
    // LinkedIn only puts the avatar on the first bubble in a group — reuse it.
    if (senderPhoto) {
      if (self) lastSelfPhoto = senderPhoto;
      else lastOtherPhoto = senderPhoto;
    } else {
      senderPhoto = self ? lastSelfPhoto : lastOtherPhoto;
    }
    items.push({
      type: 'msg',
      heading,
      sender: item.querySelector('.msg-s-message-group__name')?.innerText?.trim() || '',
      time: item.querySelector('.msg-s-message-group__timestamp')?.innerText?.trim() || '',
      subject: item.querySelector('.msg-s-event-listitem__subject')?.innerText?.trim() || '',
      html: body ? body.innerHTML : '',
      text: body ? body.innerText.trim() : '',
      self,
      images: await toDataAll(imgUrls),
      senderPhoto,
    });
  }
  return { name, headline, degree, photo, profileUrl, url: location.href, items };
}

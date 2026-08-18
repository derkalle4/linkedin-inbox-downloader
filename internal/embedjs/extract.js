async () => {
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const isPlaceholder = (src) => {
    if (!src) return true;
    if (src.startsWith('blob:')) return true;
    if (src.startsWith('data:image/gif')) return true;
    if (/placeholder|spacer|blank|transparent/i.test(src)) return true;
    if (src.startsWith('data:') && src.length < 80) return true;
    return false;
  };

  const fromSrcset = (srcset) => {
    if (!srcset) return '';
    const parts = srcset.split(',').map((p) => p.trim().split(/\s+/)[0]).filter(Boolean);
    for (let i = parts.length - 1; i >= 0; i--) {
      if (!isPlaceholder(parts[i])) return parts[i];
    }
    return '';
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
    const fromSet = fromSrcset(img.getAttribute?.('srcset') || img.srcset || '');
    if (fromSet) return fromSet;
    const picture = img.closest?.('picture');
    if (picture) {
      for (const source of picture.querySelectorAll('source')) {
        const u = fromSrcset(source.getAttribute('srcset') || '');
        if (u) return u;
      }
    }
    return '';
  };

  const reveal = (img) => {
    if (!img) return '';
    img.loading = 'eager';
    img.removeAttribute('loading');
    const url = imgURL(img);
    if (url && !url.startsWith('data:') && img.src !== url) img.src = url;
    return url;
  };

  const toData = async (src) => {
    if (!src || isPlaceholder(src)) return '';
    if (src.startsWith('data:')) return src;
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
        /* CORS / cache miss — try next, then keep the URL for Chrome */
      }
    }
    return src;
  };

  const fromImg = async (img) => {
    const url = reveal(img);
    try {
      if (img && img.decode) await img.decode();
    } catch (e) {}
    return toData(url);
  };

  const root = document.querySelector('.msg-thread') || document;
  const box = root.querySelector('.msg-s-message-list.scrollable')
    || root.querySelector('.msg-s-message-list')
    || root;

  const PROFILE_SEL = [
    'img.msg-s-event-listitem__profile-picture',
    'img.presence-entity__image',
    'img.evi-image',
    '.msg-s-profile-card img',
    '.msg-entity-lockup img',
  ].join(', ');

  const events = [...box.querySelectorAll('li.msg-s-message-list__event')];
  for (let i = 0; i < events.length; i++) {
    try {
      events[i].scrollIntoView({ block: 'nearest', inline: 'nearest' });
    } catch (e) {}
    events[i].querySelectorAll(PROFILE_SEL + ', img').forEach(reveal);
    if (i % 8 === 0) await sleep(40);
  }
  const profileImgs = [...root.querySelectorAll(PROFILE_SEL)];
  profileImgs.forEach(reveal);
  const deadline = Date.now() + 6000;
  while (Date.now() < deadline) {
    const pending = profileImgs.filter((img) => !(img.complete && img.naturalWidth > 1));
    if (pending.length === 0) break;
    await Promise.all(pending.map((img) => img.decode ? img.decode().catch(() => {}) : sleep(50)));
    await sleep(80);
  }

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
  const photoImg = card?.querySelector('img.presence-entity__image, img.evi-image, img')
    || root.querySelector('.msg-s-event-listitem--other img.msg-s-event-listitem__profile-picture')
    || root.querySelector('img.msg-s-event-listitem__profile-picture');
  const photo = await fromImg(photoImg);

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
    let senderPhoto = await fromImg(item.querySelector('img.msg-s-event-listitem__profile-picture'));
    if (senderPhoto) {
      if (self) lastSelfPhoto = senderPhoto;
      else lastOtherPhoto = senderPhoto;
    } else {
      senderPhoto = self ? lastSelfPhoto : lastOtherPhoto;
    }
    const images = [];
    for (const u of imgUrls) images.push(await toData(u));
    items.push({
      type: 'msg',
      heading,
      sender: item.querySelector('.msg-s-message-group__name')?.innerText?.trim() || '',
      time: item.querySelector('.msg-s-message-group__timestamp')?.innerText?.trim() || '',
      subject: item.querySelector('.msg-s-event-listitem__subject')?.innerText?.trim() || '',
      html: body ? body.innerHTML : '',
      text: body ? body.innerText.trim() : '',
      self,
      images,
      senderPhoto,
    });
  }
  return { name, headline, degree, photo, profileUrl, url: location.href, items };
}

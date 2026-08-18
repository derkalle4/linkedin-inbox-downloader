async () => {
  const toData = async (src) => {
    if (!src) return '';
    if (src.startsWith('data:')) return src;
    try {
      const r = await fetch(src, { credentials: 'include' });
      if (!r.ok) return src;
      const buf = await r.arrayBuffer();
      const mime = (r.headers.get('content-type') || 'image/jpeg').split(';')[0];
      const bytes = new Uint8Array(buf);
      let bin = '';
      const step = 0x8000;
      for (let i = 0; i < bytes.length; i += step) {
        bin += String.fromCharCode.apply(null, bytes.subarray(i, i + step));
      }
      return `data:${mime};base64,` + btoa(bin);
    } catch (e) {
      return src;
    }
  };

  // Sequential fetches — avoid bursting LinkedIn CDN with Promise.all.
  const toDataAll = async (urls) => {
    const out = [];
    for (const u of urls) {
      out.push(await toData(u));
    }
    return out;
  };

  const root = document.querySelector('.msg-thread') || document;
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
  let photoSrc = card?.querySelector('img.presence-entity__image, img.evi-image')?.currentSrc
    || root.querySelector('.msg-s-event-listitem--other img.msg-s-event-listitem__profile-picture')?.currentSrc
    || '';
  const photo = await toData(photoSrc);

  const items = [];
  for (const li of root.querySelectorAll('li.msg-s-message-list__event')) {
    const heading = li.querySelector('.msg-s-message-list__time-heading')?.innerText?.trim() || '';
    const item = li.querySelector('.msg-s-event-listitem');
    if (!item) {
      if (heading) items.push({ type: 'day', heading });
      continue;
    }
    const body = item.querySelector('.msg-s-event-listitem__body, .msg-s-event__content');
    const imgUrls = [...item.querySelectorAll('.msg-s-event-listitem__message-bubble img, .msg-s-event__content img')]
      .map((img) => img.currentSrc || img.src)
      .filter((src) => src && !src.startsWith('data:image/gif') && !/profile-displayphoto|EntityPhoto|presence-entity|msg-s-event-listitem__profile-picture/.test(src));
    const senderPhotoSrc = item.querySelector('img.msg-s-event-listitem__profile-picture')?.currentSrc || '';
    items.push({
      type: 'msg',
      heading,
      sender: item.querySelector('.msg-s-message-group__name')?.innerText?.trim() || '',
      time: item.querySelector('.msg-s-message-group__timestamp')?.innerText?.trim() || '',
      subject: item.querySelector('.msg-s-event-listitem__subject')?.innerText?.trim() || '',
      html: body ? body.innerHTML : '',
      text: body ? body.innerText.trim() : '',
      self: !item.classList.contains('msg-s-event-listitem--other'),
      images: await toDataAll(imgUrls),
      senderPhoto: await toData(senderPhotoSrc),
    });
  }
  return { name, headline, degree, photo, profileUrl, url: location.href, items };
}

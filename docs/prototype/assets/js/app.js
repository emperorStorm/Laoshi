/* Laoshi prototype interactions. Classic script: works from file:// too. */
(function () {
  'use strict';

  const products = Array.isArray(window.LAOSHI_PRODUCTS) ? window.LAOSHI_PRODUCTS : [];
  const bySlug = new Map(products.map((product) => [product.slug, product]));

  const grid = document.getElementById('works-grid');
  const ledgerBody = document.getElementById('ledger-body');
  const filterStatus = document.getElementById('filter-status');
  const dialog = document.getElementById('product-dialog');
  const drawerPanel = dialog ? dialog.querySelector('.drawer-panel') : null;
  const drawerLogo = document.getElementById('drawer-logo');
  const drawerKicker = document.getElementById('drawer-kicker');
  const drawerTitle = document.getElementById('drawer-title');
  const drawerSub = document.getElementById('drawer-sub');
  const drawerBody = document.getElementById('drawer-body');
  const drawerActions = document.getElementById('drawer-actions');
  const drawerClose = document.getElementById('drawer-close');
  const menuToggle = document.getElementById('menu-toggle');
  const siteNav = document.getElementById('site-nav');
  const header = document.getElementById('site-header');

  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  let activeSlug = null;
  let lastTrigger = null;
  let pushedEntry = false;

  const escapeHtml = (value) => String(value).replace(/[&<>"']/g, (char) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;'
  })[char]);

  const pad = (index) => String(index).padStart(2, '0');

  function renderCards() {
    if (!grid) return;

    grid.innerHTML = products.map((product, index) => {
      const style = `--accent:${product.accent};--accent-soft:${product.accentSoft}`;
      const tags = product.featureTags.map((tag) => `<li>${escapeHtml(tag)}</li>`).join('');
      const platforms = product.platforms.map(escapeHtml).join(' · ');
      const download = product.download.status === 'soon'
        ? `<span class="card-download is-disabled" aria-disabled="true">${escapeHtml(product.download.label)}</span>`
        : `<a class="card-download" href="${escapeHtml(product.download.url)}" target="_blank" rel="noopener noreferrer">${escapeHtml(product.download.label)} ↗</a>`;

      return `
        <article class="work-card reveal" data-category="${escapeHtml(product.category)}" data-slug="${escapeHtml(product.slug)}" style="${style}">
          <span class="card-hit" aria-hidden="true" data-open="${escapeHtml(product.slug)}"></span>
          <div class="card-inner">
            <div class="card-top">
              <span class="card-logo">
                <img src="${escapeHtml(product.logo)}" alt="${escapeHtml(product.logoAlt)}" width="40" height="40" loading="lazy">
              </span>
              <span class="card-index">${pad(index + 1)}</span>
            </div>
            <div class="card-heading">
              <h3>${escapeHtml(product.nameZh)}</h3>
              <p class="card-en">${escapeHtml(product.nameEn)} <span>v${escapeHtml(product.version)}</span></p>
            </div>
            <p class="card-tagline">${escapeHtml(product.tagline)}</p>
            <ul class="card-tags">${tags}</ul>
            <p class="card-platforms">
              <span class="card-category">${escapeHtml(product.categoryLabel)}</span>
              <span class="card-sep">/</span>
              <span>${platforms}</span>
            </p>
            <div class="card-actions">
              <button class="card-detail" type="button" data-open="${escapeHtml(product.slug)}">详情</button>
              ${download}
            </div>
          </div>
        </article>
      `;
    }).join('');
  }

  function renderLedger() {
    if (!ledgerBody) return;

    ledgerBody.innerHTML = products.map((product) => {
      const primary = product.download.status === 'soon'
        ? `<span class="ledger-soon">${escapeHtml(product.download.label)}</span>`
        : `<a class="ledger-link" href="${escapeHtml(product.download.url)}" target="_blank" rel="noopener noreferrer">${escapeHtml(product.download.label)} ↗</a>`;
      const demo = product.links.demo
        ? `<a class="ledger-link is-demo" href="${escapeHtml(product.links.demo)}" target="_blank" rel="noopener noreferrer">在线原型 ↗</a>`
        : '';

      return `
        <tr>
          <td data-label="作品">
            <button class="ledger-product" type="button" data-open="${escapeHtml(product.slug)}" aria-label="查看 ${escapeHtml(product.nameZh)} ${escapeHtml(product.nameEn)} 详情">
              <img src="${escapeHtml(product.logo)}" alt="" width="34" height="34" loading="lazy">
              <span>
                <strong>${escapeHtml(product.nameZh)}</strong>
                <em>${escapeHtml(product.nameEn)}</em>
              </span>
            </button>
          </td>
          <td data-label="形态">
            <span class="ledger-form">${escapeHtml(product.categoryLabel)}<small>${product.localFirst ? '本地优先' : '服务端支持'}</small></span>
          </td>
          <td data-label="版本"><span class="ledger-version">v${escapeHtml(product.version)}</span></td>
          <td data-label="获取"><div class="ledger-actions">${primary}${demo}</div></td>
        </tr>
      `;
    }).join('');
  }

  function renderDrawer(product) {
    if (!dialog) return;

    drawerLogo.src = product.logo;
    drawerLogo.alt = product.logoAlt;
    drawerKicker.textContent = `${product.categoryLabel} · v${product.version}${product.localFirst ? ' · 本地优先' : ''}`;
    drawerTitle.textContent = product.nameZh;
    drawerSub.textContent = `${product.nameEn} · ${product.tagline}`;

    drawerPanel.style.setProperty('--accent', product.accent);
    drawerPanel.style.setProperty('--accent-soft', product.accentSoft);

    const capabilities = product.features.map((feature, index) => (
      `<li><span>${pad(index + 1)}</span><p>${escapeHtml(feature)}</p></li>`
    )).join('');

    const platforms = product.platforms.map((platform) => `<li>${escapeHtml(platform)}</li>`).join('');
    const stack = product.stack.map(escapeHtml).join(' · ');

    const shots = product.shots.length
      ? `<div class="shot-list">${product.shots.map((shot, index) => `
          <figure>
            <img src="${escapeHtml(shot.src)}" alt="${escapeHtml(shot.alt)}" width="${Number(shot.width)}" height="${Number(shot.height)}" loading="lazy" decoding="async">
            <figcaption>${escapeHtml(shot.alt)} · ${pad(index + 1)}</figcaption>
          </figure>
        `).join('')}</div>`
      : `
        <div class="shot-empty">
          <span>NO PREVIEW YET</span>
          <p>暂无界面截图</p>
          <small>原型阶段暂未收录，可前往仓库查看项目说明与截图。</small>
        </div>
      `;

    drawerBody.innerHTML = `
      <section class="drawer-section">
        <p class="drawer-summary">${escapeHtml(product.summary)}</p>
      </section>

      <section class="drawer-section">
        <h3 class="drawer-label">能力清单 <span>CAPABILITIES</span></h3>
        <ol class="capability-list">${capabilities}</ol>
      </section>

      <div class="drawer-meta-grid">
        <div>
          <h3 class="drawer-label">平台与形态 <span>PLATFORMS</span></h3>
          <ul class="chip-list">${platforms}</ul>
          <p class="drawer-local">${product.localFirst
            ? '本地优先：核心数据默认保存在自己的设备上。'
            : '需要服务端支持：数据与能力由后端服务提供。'}</p>
        </div>
        <div>
          <h3 class="drawer-label">技术栈 <span>STACK</span></h3>
          <p class="stack-line">${stack}</p>
        </div>
      </div>

      <section class="drawer-section">
        <h3 class="drawer-label">界面一览 <span>PREVIEW</span></h3>
        ${shots}
      </section>
    `;

    const primary = product.download.status === 'soon'
      ? `<span class="btn btn-primary is-disabled" aria-disabled="true">${escapeHtml(product.download.label)}</span>`
      : `<a class="btn btn-primary" href="${escapeHtml(product.download.url)}" target="_blank" rel="noopener noreferrer">${escapeHtml(product.download.label)} ↗</a>`;
    const github = `<a class="btn btn-ghost" href="${escapeHtml(product.links.repo)}" target="_blank" rel="noopener noreferrer">GitHub ↗</a>`;
    const demo = product.links.demo
      ? `<a class="btn btn-ghost" href="${escapeHtml(product.links.demo)}" target="_blank" rel="noopener noreferrer">在线原型 ↗</a>`
      : '';

    drawerActions.innerHTML = `
      ${primary}
      ${github}
      ${demo}
      <p class="drawer-action-note">${escapeHtml(product.download.note)}</p>
    `;
  }

  /* ------------------------------------------------------------- drawer */

  function readHash() {
    const raw = window.location.hash.replace(/^#/, '');
    if (!raw.startsWith('product=')) return null;
    try {
      return decodeURIComponent(raw.slice('product='.length)) || null;
    } catch (error) {
      return null;
    }
  }

  function pushHash(slug) {
    const hash = `#product=${encodeURIComponent(slug)}`;
    if (window.location.hash === hash) return;
    try {
      window.history.pushState({ product: slug }, '', hash);
      pushedEntry = true;
    } catch (error) {
      window.location.hash = hash;
      pushedEntry = true;
    }
  }

  function clearHash() {
    if (!readHash()) return;
    try {
      window.history.replaceState(null, '', window.location.pathname + window.location.search);
    } catch (error) {
      window.location.hash = '';
    }
  }

  function openDrawer(slug, options) {
    const product = bySlug.get(slug);
    if (!product || !dialog) return;

    const settings = options || {};
    if (settings.trigger) lastTrigger = settings.trigger;

    renderDrawer(product);
    activeSlug = slug;
    document.body.classList.add('drawer-open');

    if (!dialog.open) dialog.showModal();
    drawerTitle.focus({ preventScroll: true });

    if (settings.push !== false) pushHash(slug);
  }

  function requestClose() {
    if (!dialog || !dialog.open) return;

    if (pushedEntry) {
      pushedEntry = false;
      window.history.back();
      return;
    }

    clearHash();
    dialog.close();
  }

  function syncFromHash() {
    if (!dialog) return;
    const slug = readHash();

    if (slug && bySlug.has(slug)) {
      if (!dialog.open || activeSlug !== slug) {
        openDrawer(slug, { push: false, trigger: dialog.open ? lastTrigger : null });
      }
      return;
    }

    if (dialog.open) dialog.close();
  }

  function bindDrawer() {
    if (!dialog) return;

    drawerClose.addEventListener('click', requestClose);

    dialog.addEventListener('cancel', (event) => {
      event.preventDefault();
      requestClose();
    });

    dialog.addEventListener('click', (event) => {
      if (event.target === dialog) requestClose();
    });

    dialog.addEventListener('close', () => {
      const closingSlug = activeSlug;
      document.body.classList.remove('drawer-open');
      activeSlug = null;
      pushedEntry = false;

      const trigger = lastTrigger;
      lastTrigger = null;

      if (trigger && document.contains(trigger)) {
        trigger.focus({ preventScroll: true });
        return;
      }

      if (closingSlug) {
        const fallback = document.querySelector(`.card-detail[data-open="${closingSlug}"]`);
        if (fallback) fallback.focus({ preventScroll: true });
      }
    });

    window.addEventListener('popstate', syncFromHash);
    window.addEventListener('hashchange', syncFromHash);
  }

  /* ------------------------------------------------------------- filter */

  function bindFilters() {
    const buttons = Array.from(document.querySelectorAll('.filter'));
    const cards = () => Array.from(grid.querySelectorAll('.work-card'));

    buttons.forEach((button) => {
      button.addEventListener('click', () => {
        const filter = button.dataset.filter;
        let shown = 0;

        buttons.forEach((item) => {
          const active = item === button;
          item.classList.toggle('is-active', active);
          item.setAttribute('aria-pressed', String(active));
        });

        cards().forEach((card) => {
          const match = filter === 'all' || card.dataset.category === filter;
          card.hidden = !match;
          if (match) {
            shown += 1;
            card.classList.add('is-visible');
          }
        });

        filterStatus.textContent = `显示 ${shown} 件作品`;
      });
    });
  }

  function renderCounts() {
    const counts = { all: products.length, desktop: 0, mobile: 0, service: 0 };
    products.forEach((product) => {
      if (counts[product.category] !== undefined) counts[product.category] += 1;
    });

    document.querySelectorAll('[data-count]').forEach((node) => {
      node.textContent = String(counts[node.dataset.count] ?? 0);
    });

    const works = document.querySelector('[data-stat="works"]');
    const platforms = document.querySelector('[data-stat="platforms"]');
    const localFirst = document.querySelector('[data-stat="local-first"]');

    const families = new Set();
    products.forEach((product) => {
      product.platforms.forEach((platform) => {
        if (platform === 'API') return;
        families.add(platform.startsWith('Web') ? 'Web' : platform);
      });
    });

    if (works) works.textContent = String(products.length);
    if (platforms) platforms.textContent = String(families.size);
    if (localFirst) localFirst.textContent = String(products.filter((product) => product.localFirst).length);
    if (filterStatus) filterStatus.textContent = `显示 ${products.length} 件作品`;
  }

  /* ---------------------------------------------------------- interactions */

  function bindOpeners() {
    const handleOpen = (event) => {
      const opener = event.target.closest('[data-open]');
      if (!opener) return;
      const slug = opener.dataset.open;
      if (!bySlug.has(slug)) return;

      const trigger = opener.classList.contains('card-hit')
        ? opener.closest('.work-card').querySelector('.card-detail')
        : opener;

      openDrawer(slug, { trigger });
    };

    if (grid) grid.addEventListener('click', handleOpen);
    if (ledgerBody) ledgerBody.addEventListener('click', handleOpen);
  }

  function bindMenu() {
    if (!menuToggle || !siteNav) return;

    const label = menuToggle.querySelector('.sr-only');

    const setMenu = (open) => {
      menuToggle.setAttribute('aria-expanded', String(open));
      siteNav.classList.toggle('is-open', open);
      if (label) label.textContent = open ? '关闭菜单' : '打开菜单';
    };

    const isOpen = () => menuToggle.getAttribute('aria-expanded') === 'true';

    menuToggle.addEventListener('click', () => setMenu(!isOpen()));

    siteNav.addEventListener('click', (event) => {
      if (event.target.closest('a')) setMenu(false);
    });

    document.addEventListener('click', (event) => {
      if (!isOpen()) return;
      if (siteNav.contains(event.target) || menuToggle.contains(event.target)) return;
      setMenu(false);
    });

    document.addEventListener('keydown', (event) => {
      if (event.key !== 'Escape' || !isOpen()) return;
      setMenu(false);
      menuToggle.focus();
    });

    const desktop = window.matchMedia('(min-width: 761px)');
    const onChange = (event) => {
      if (event.matches) setMenu(false);
    };
    if (typeof desktop.addEventListener === 'function') desktop.addEventListener('change', onChange);
  }

  function bindScrollState() {
    if (!header) return;
    let ticking = false;

    const update = () => {
      header.classList.toggle('is-scrolled', window.scrollY > 8);
      ticking = false;
    };

    window.addEventListener('scroll', () => {
      if (ticking) return;
      ticking = true;
      window.requestAnimationFrame(update);
    }, { passive: true });

    update();
  }

  function reveal(node) {
    node.classList.add('is-visible');
  }

  function setupReveals() {
    const nodes = Array.from(document.querySelectorAll('.reveal'));
    if (!nodes.length) return;

    if (reducedMotion || !('IntersectionObserver' in window)) {
      nodes.forEach(reveal);
      return;
    }

    const observer = new IntersectionObserver((entries, instance) => {
      entries.forEach((entry) => {
        if (!entry.isIntersecting) return;
        reveal(entry.target);
        instance.unobserve(entry.target);
      });
    }, { threshold: 0.08, rootMargin: '0px 0px -6% 0px' });

    nodes.forEach((node) => observer.observe(node));

    // Safety net for very fast scrolling: never leave a section behind the reader.
    let ticking = false;
    const sweep = () => {
      ticking = false;
      document.querySelectorAll('.reveal:not(.is-visible)').forEach((node) => {
        if (node.getBoundingClientRect().top < window.innerHeight * 0.94) reveal(node);
      });
    };
    const scheduleSweep = () => {
      if (ticking) return;
      ticking = true;
      window.requestAnimationFrame(sweep);
    };

    window.addEventListener('scroll', scheduleSweep, { passive: true });
    window.addEventListener('resize', scheduleSweep);
    scheduleSweep();
  }

  function setYear() {
    const year = document.getElementById('year');
    if (year) year.textContent = String(new Date().getFullYear());
  }

  renderCards();
  renderLedger();
  renderCounts();
  bindOpeners();
  bindFilters();
  bindDrawer();
  bindMenu();
  bindScrollState();
  setupReveals();
  setYear();
  syncFromHash();
})();

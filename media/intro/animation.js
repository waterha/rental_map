(() => {
  'use strict';
  const $ = id => document.getElementById(id);
  const clamp = (value, lo = 0, hi = 1) => Math.min(hi, Math.max(lo, value));
  const ease = value => { const p = clamp(value); return 1 - (1 - p) ** 3; };
  const smooth = value => { const p = clamp(value); return p * p * (3 - 2 * p); };
  const ramp = (t, start, duration) => clamp((t - start) / duration);
  const svg = (tag, attrs, parent) => {
    const element = document.createElementNS('http://www.w3.org/2000/svg', tag);
    Object.entries(attrs).forEach(([key, value]) => element.setAttribute(key, value));
    if (parent) parent.append(element);
    return element;
  };
  function visible(element, alpha) {
    element.style.opacity = clamp(alpha);
    element.style.visibility = alpha > 0.001 ? 'visible' : 'hidden';
  }
  function section(id, t, start, end, duration = .65) {
    const alpha = ease(ramp(t, start, duration)) * (1 - smooth(ramp(t, end - duration, duration)));
    const element = $(id); visible(element, alpha);
    element.style.transform = `translateY(${(1 - ease(ramp(t, start, .9))) * 24}px)`;
    return alpha;
  }
  for (const [name, features] of Object.entries(window.PROMO_MAPS)) {
    const group = $('geo-' + name);
    for (const f of features) {
      svg('path', { d: f.d || '', class: (name === 'china' && f.code === '330000') || (name === 'zhejiang' && f.code === '330100') || (name === 'hangzhou' && f.code === '330106') ? 'active' : '', 'data-code': f.code }, group);
      if (name !== 'china' || ['110000', '330000', '440000', '510000', '650000', '150000', '540000', '420000', '310000'].includes(f.code)) {
        const text = svg('text', { x: f.x, y: f.y, 'font-size': name === 'china' ? 15 : name === 'zhejiang' ? 19 : 16 }, group);
        text.textContent = f.name;
      }
    }
  }
  let seed = 391;
  const random = () => { seed = (seed * 1664525 + 1013904223) >>> 0; return seed / 4294967296; };
  for (let row = 0; row < 13; row++) for (let col = 0; col < 20; col++) {
    const x = 665 + col * 66 + (row % 2) * 12, y = -80 + row * 93;
    if ((x > 1460 && x < 1650) || (x < 1110 && y > 720) || random() < .14) continue;
    const w = 22 + random() * 27, h = 23 + random() * 38;
    const building = svg('rect', { x, y, width: w, height: h, rx: 2, class: 'building', opacity: .38 + random() * .4, transform: `rotate(-8 ${x} ${y})` }, $('buildings'));
    if (random() > .72) building.setAttribute('fill', '#1c3d50');
  }
  const minorRoads = [];
  for (let i = 0; i < 13; i++) {
    const y = -60 + i * 100;
    minorRoads.push(`M660 ${y}Q1100 ${y - 50} 1440 ${y + 5}T2010 ${y - 30}`);
  }
  for (let i = 0; i < 16; i++) {
    const x = 660 + i * 90;
    minorRoads.push(`M${x} -80Q${x + 80} 300 ${x + 5} 590T${x - 70} 1180`);
  }
  for (const d of minorRoads) svg('path', { d, class: 'road-minor' }, $('roads'));
  const majorRoads = ['M600 350Q1040 318 1260 400T1990 390', 'M620 674Q1000 580 1270 640T2020 750', 'M1060 -50Q1110 350 1170 550T1220 1140', 'M1730 -100Q1610 380 1690 630T1800 1160'];
  for (const d of majorRoads) for (const kind of ['road-major-outline', 'road-major', 'road-major-center']) svg('path', { d, class: kind }, $('roads'));
  const palette = { whole: '#bc92ff', shared: '#ffd66e', sublet: '#ff7087' };
  const pins = [
    { x: 1160, y: 407, kind: 'whole' }, { x: 1160, y: 654, kind: 'shared' }, { x: 1280, y: 780, kind: 'sublet' },
    { x: 1020, y: 515, kind: 'whole' }, { x: 1248, y: 294, kind: 'shared' }, { x: 1390, y: 409, kind: 'sublet' },
    { x: 960, y: 692, kind: 'shared' }, { x: 1085, y: 235, kind: 'whole' }, { x: 1170, y: 875, kind: 'whole' },
    { x: 1010, y: 375, kind: 'sublet' },
    { x: 1545, y: 648, kind: 'sublet', neighbor: true }, { x: 1510, y: 434, kind: 'whole', neighbor: true },
    { x: 1630, y: 296, kind: 'shared', neighbor: true }, { x: 1740, y: 556, kind: 'whole', neighbor: true },
    { x: 1580, y: 790, kind: 'shared', neighbor: true }, { x: 1765, y: 845, kind: 'sublet', neighbor: true },
  ];
  pins.forEach((pin, index) => {
    const group = svg('g', { 'data-pin': index }, $('rental-pins'));
    svg('circle', { cx: pin.x, cy: pin.y, r: 18, fill: palette[pin.kind], opacity: .20, filter: 'url(#point-glow)' }, group);
    pin.halo = svg('circle', { cx: pin.x, cy: pin.y, r: 17, stroke: palette[pin.kind], 'stroke-opacity': .45, class: 'rental-pin-halo' }, group);
    svg('circle', { cx: pin.x, cy: pin.y, r: 5.5, fill: palette[pin.kind], class: 'rental-pin-core' }, group);
    pin.element = group;
  });
  const eastShade = svg('path', { d: 'M1430 -30H2000V1150H1400L1340 790 1420 590Q1390 470 1430 330Z', fill: '#030a13', opacity: .64 }, null);
  $('street-camera').insertBefore(eastShade, $('nearby-rings'));
  $('price-c').style.left = '1190px'; $('price-c').style.top = '817px';
  const sceneLabels = ['租房版图 · 地图找房', '01 / 找到目标区域', '02 / 看清位置关系', '03 / 点开，就是房源', '04 / 找房，不被区界限制', '租房版图 · 位置附近，一眼找房'];
  function renderAt(seconds) {
    const t = clamp(seconds, 0, 36);
    section('opening', t, -.2, 4.9);
    section('geography', t, 4.2, 10.7);
    const streetAlpha = ease(ramp(t, 10, .9)) * (1 - smooth(ramp(t, 31.2, .9)));
    visible($('street-stage'), streetAlpha);
    section('nearby', t, 10, 18.1);
    section('detail', t, 17.4, 24.8);
    section('merge', t, 24.1, 32.1);
    // Keep the completed end card visible on the final frame.
    visible($('ending'), ease(ramp(t, 31.4, .85)));
    $('ending').style.transform = `translateY(${(1 - ease(ramp(t, 31.4, 1))) * 20}px)`;
    const openProgress = ease(ramp(t, .3, 2.5));
    $('opening').querySelector('h1').style.transform = `translateY(${(1 - openProgress) * 18}px)`;
    document.querySelectorAll('.opening-label').forEach((label, index) => {
      label.style.opacity = ease(ramp(t, .9 + index * .35, .9));
      label.style.transform = `translateY(${(1 - ease(ramp(t, .9 + index * .35, 1.2))) * 35 + Math.sin(t * .9 + index * 1.8) * 5}px)`;
    });
    document.querySelectorAll('.opening-orbits>i').forEach((ring, index) => ring.style.opacity = .6 + .25 * Math.sin(t * .8 + index));
    const china = 1 - smooth(ramp(t, 6.1, .75));
    const zhejiang = smooth(ramp(t, 6.1, .75)) * (1 - smooth(ramp(t, 8, .75)));
    const hangzhou = smooth(ramp(t, 8, .75));
    $('geo-china').style.opacity = china;
    $('geo-zhejiang').style.opacity = zhejiang;
    $('geo-hangzhou').style.opacity = hangzhou;
    $('geo-china').style.transform = `scale(${1 + smooth(ramp(t, 5.7, 1.2)) * .12})`;
    $('geo-zhejiang').style.transform = `scale(${.89 + ease(ramp(t, 6.1, 1)) * .11})`;
    $('geo-hangzhou').style.transform = `scale(${.9 + ease(ramp(t, 8, 1)) * .1})`;
    $('geo-caption').textContent = t < 6.5 ? 'CHINA / 中国' : t < 8.4 ? 'ZHEJIANG / 浙江省' : 'HANGZHOU / 杭州市';
    $('crumb-province').style.opacity = ease(ramp(t, 6.3, .5));
    $('crumb-slash').style.opacity = ease(ramp(t, 8.2, .4));
    $('crumb-city').style.opacity = ease(ramp(t, 8.2, .5));
    $('search-text').textContent = '杭州西湖'.slice(0, Math.floor(ramp(t, 7.4, 1.15) * 5));
    document.querySelector('.search-demo>i').style.opacity = Math.floor(t * 2) % 2 ? .9 : .2;
    const cameraShift = ease(ramp(t, 17.5, 1.1)) * -65 + ease(ramp(t, 24.1, 1.1)) * 65;
    $('street-camera').setAttribute('transform', `translate(${cameraShift} 0)`);
    $('work-tag').style.transform = `translateX(${cameraShift}px)`;
    $('work-tag').style.opacity = ease(ramp(t, 10.6, .7));
    $('work-pin').style.opacity = ease(ramp(t, 10.5, .6));
    const halo = $('work-pin').querySelector('.work-halo');
    halo.setAttribute('r', 43 + Math.sin(t * 2.5) * 6);
    const mergeProgress = smooth(ramp(t, 27.1, 1.3));
    eastShade.setAttribute('opacity', .66 * (1 - mergeProgress));
    document.querySelector('.district-seam').style.opacity = .8 - mergeProgress * .6;
    document.querySelector('.east-boundary').style.strokeOpacity = mergeProgress;
    $('neighbor-label').style.opacity = .14 + mergeProgress * .45;
    $('map-scope').textContent = t < 27.7 ? '西湖区' : '西湖区 + 余杭区';
    $('neighbor-chip').style.opacity = .18 + ease(ramp(t, 25.6, .6)) * .82;
    $('neighbor-chip').style.transform = `translateY(${(1 - ease(ramp(t, 25.6, .7))) * 5}px)`;
    $('merge-confirm').style.background = t > 27.1 && t < 27.5 ? '#46c3de' : '#1e6b83';
    $('merge-confirm').style.transform = `scale(${1 - Math.sin(ramp(t, 27.1, .28) * Math.PI) * .025})`;
    $('neighbor-callout').style.opacity = ease(ramp(t, 28.2, .8));
    $('neighbor-callout').style.transform = `translateY(${(1 - ease(ramp(t, 28.2, .8))) * 12}px)`;
    $('nearby-rings').style.opacity = .5 + ease(ramp(t, 11, 2)) * .5;
    $('nearby-rings').style.transformOrigin = '1320px 540px';
    $('nearby-rings').style.transform = `scale(${.88 + ease(ramp(t, 10.6, 2.4)) * .12})`;
    $('commute-routes').style.opacity = ease(ramp(t, 13.2, .9));
    $('commute-routes').querySelectorAll('path').forEach((route, index) => {
      const len = route.getTotalLength();
      route.style.strokeDasharray = index === 1 ? '6 8' : '8 6';
      route.style.strokeDashoffset = -t * 12;
      route.style.opacity = index === 1 ? mergeProgress : (1 - smooth(ramp(t, 17.5, .6))) * .9;
      route.dataset.length = len;
    });
    pins.forEach((pin, index) => {
      const enter = pin.neighbor ? ease(ramp(t, 27.25 + (index - 10) * .13, .7)) : ease(ramp(t, 11.3 + index * .12, .75));
      const filter = t >= 19.1 && t < 24.25 ? (pin.kind === 'whole' ? 1 : 1 - smooth(ramp(t, 19.1, .55))) : 1;
      pin.element.style.opacity = enter * filter;
      pin.halo.setAttribute('r', 13 + (1 + Math.sin(t * 2.3 + index * .8)) * 4);
      pin.halo.setAttribute('stroke-opacity', .25 + Math.sin(t * 2.3 + index) * .12);
    });
    ['price-a', 'price-b', 'price-c'].forEach((id, index) => {
      const p = ease(ramp(t, 12.5 + index * .65, .8));
      $(id).style.opacity = p;
      $(id).style.transform = `translateY(${(1 - p) * 15}px)`;
    });
    const card = ease(ramp(t, 20.5, .85));
    $('listing-card').style.opacity = card;
    $('listing-card').style.transform = `translateX(${(1 - card) * 50}px)`;
    $('click-ring').style.opacity = ramp(t, 20.05, .1) * (1 - ramp(t, 20.5, .45));
    $('click-ring').style.transform = `translateX(${cameraShift}px) scale(${.5 + ramp(t, 20.05, .9) * 1.9})`;
    $('detail-caption').style.opacity = ease(ramp(t, 21.3, .7));
    const cursor = $('cursor');
    let x = 0, y = 0, alpha = 0;
    if (t >= 19.4 && t < 21.5) { const p = ease(ramp(t, 19.4, .8)); x = 1290 + (1095 - 1290) * p; y = 620 + (407 - 620) * p; alpha = ease(ramp(t, 19.4, .3)) * (1 - ramp(t, 21, .5)); }
    if (t >= 26.2 && t < 28.2) { const p = ease(ramp(t, 26.2, .8)); x = 540 + (408 - 540) * p; y = 880 + (790 - 880) * p; alpha = ease(ramp(t, 26.2, .25)) * (1 - ramp(t, 27.7, .5)); }
    cursor.style.transform = `translate(${x}px,${y}px)`; cursor.style.opacity = alpha;
    const index = t < 4.5 ? 0 : t < 10.3 ? 1 : t < 17.8 ? 2 : t < 24.5 ? 3 : t < 31.5 ? 4 : 5;
    $('footer-label').textContent = sceneLabels[index];
    $('time-code').textContent = `00:${Math.floor(t).toString().padStart(2, '0')}`;
    $('progress-bar').style.width = `${t / 36 * 100}%`;
    $('seek').value = t;
    return { seconds: t, scene: index, duration: 36 };
  }
  window.renderAt = renderAt;
  window.PROMO_DURATION = 36;
  const exporting = new URLSearchParams(location.search).has('export');
  if (exporting) document.body.classList.add('exporting');
  function resize() { $('film').style.transform = `scale(${Math.min(innerWidth / 1920, innerHeight / 1080)})`; }
  addEventListener('resize', resize); resize();
  let paused = exporting, time = 0, last = performance.now();
  $('toggle').onclick = () => { paused = !paused; last = performance.now(); };
  $('seek').oninput = event => { time = Number(event.target.value); paused = true; renderAt(time); };
  function tick(now) {
    if (!paused) { time += (now - last) / 1000; if (time >= 36) { time = 36; paused = true; } renderAt(time); }
    last = now; requestAnimationFrame(tick);
  }
  renderAt(exporting ? 0 : .01);
  if (!exporting) requestAnimationFrame(tick);
})();

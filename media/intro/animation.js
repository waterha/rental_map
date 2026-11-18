(() => {
  'use strict';
  const $=id=>document.getElementById(id);
  const clamp=(v,a=0,b=1)=>Math.max(a,Math.min(b,v));
  const smooth=v=>{const p=clamp(v);return p*p*p*(p*(p*6-15)+10)};
  const ramp=(t,start,length)=>smooth((t-start)/length);
  const lerp=(a,b,p)=>a+(b-a)*p;
  const windowAlpha=(t,start,end,fade=.55)=>ramp(t,start,fade)*(1-ramp(t,end-fade,fade));
  const show=(el,opacity)=>{el.style.opacity=clamp(opacity);el.style.visibility=opacity>.001?'visible':'hidden'};
  const svg=(tag,attrs,parent)=>{const el=document.createElementNS('http://www.w3.org/2000/svg',tag);for(const [k,v] of Object.entries(attrs))el.setAttribute(k,v);if(parent)parent.append(el);return el};
  const chapters=[
    {start:0,end:4,label:'位置找房',title:'你想住的地方，<br><em>附近有什么房？</em>',body:'让房源回到地图上。<br>位置、房子，一起看清楚。',note:'沿着同一张地图，找到适合你的生活半径。'},
    {start:4,end:9,label:'搜索地区',title:'先找地区，<br><em>再看附近。</em>',body:'逐级进入省、市、区县，<br>也可以直接搜索目标地区。',note:''},
    {start:9,end:14,label:'附近房源',title:'远近关系，<br><em>一眼就明白。</em>',body:'光点就是房源。拖动、缩放，<br>看清你想住的位置周边。',note:'红色转租 · 紫色整租 · 黄色合租'},
    {start:14,end:21,label:'房源详情',title:'选好位置，<br><em>再了解房子。</em>',body:'按出租类型筛选，点击光点。<br>照片、户型、租金和联系方式一起看。',note:'左右滑动照片，保留地图上的位置感。'},
    {start:21,end:28,label:'合区浏览',title:'好房的距离，<br><em>不被区界限制。</em>',body:'住在区界附近？选择相邻区县，<br>把两边的房源放在一起比较。',note:'同一张地图，扩大选择范围。'},
    {start:28,end:34,label:'地铁图层',title:'点亮地铁，<br><em>再选附近好房。</em>',body:'区县级开启地铁图层。<br>线路、站点与房源同图展示。',note:'线路与站点连线为示意，以实际运营为准。'},
    {start:34,end:38,label:'选点发布',title:'你的好房，<br><em>也亮在这里。</em>',body:'地图选点，添加照片和房源资料，<br>让附近找房的人看见。',note:'1–5 张照片 · 出租类型 · 户型 · 价格 · 联系方式'},
    {start:38,end:40.8,label:'附近好房',title:'想住哪里，<br><em>附近好房看得见。</em>',body:'附近房源 · 合区浏览 · 地铁图层<br>在同一张地图上，找到你的选择。',note:''},
  ];
  const captions=chapters.map((c,i)=>{const el=document.createElement('section');el.className='caption';el.innerHTML=`<div class="eyebrow">${i===0?'RENTAL ATLAS':String(i).padStart(2,'0')+' / '+c.label}</div><h1>${c.title}</h1><p>${c.body}</p><small>${c.note}</small>`;$('captions').append(el);return el});
  chapters.slice(1,7).forEach(c=>{const el=document.createElement('div');el.textContent=c.label;$('journey').append(el)});
  let seed=391;
  const random=()=>{seed=(seed*1664525+1013904223)>>>0;return seed/4294967296};
  for(let row=0;row<17;row++)for(let col=0;col<23;col++){
    const x=560+col*72,y=-170+row*92;
    if((x>1560&&x<1770)||(x<1060&&y>710)||random()<.16)continue;
    svg('rect',{x,y,width:24+random()*28,height:25+random()*35,rx:2,class:'building',opacity:.45+random()*.4,transform:`rotate(-7 ${x} ${y})`},$('buildings'));
  }
  for(let i=0;i<16;i++)svg('path',{d:`M400 ${i*95-190}Q1100 ${i*95-230} 1430 ${i*95-170}T2400 ${i*95-200}`,class:'minor-road'},$('roads'));
  for(let i=0;i<22;i++)svg('path',{d:`M${470+i*90} -300Q${530+i*90} 350 ${470+i*90} 750T${420+i*90} 1400`,class:'minor-road'},$('roads'));
  for(const d of ['M350 356Q1000 300 1360 400T2400 360','M400 705Q1050 595 1410 670T2400 820','M1010 -300Q1080 320 1170 670T1230 1400','M1780 -300Q1690 300 1790 700T1970 1400'])for(const kind of ['major-road-shadow','major-road','major-road-center'])svg('path',{d,class:kind},$('roads'));
  const colors=['#bb9bff','#ffda84','#ff8199'];
  const pins=[
    [1140,410,0,2600],[1110,666,1,1500],[1270,810,2,2200],[960,510,0,2900],[1240,290,1,1600],[1350,382,2,2400],[920,680,1,1400],[1060,240,0,3100],[1190,900,0,2800],[980,355,2,2100],
    [1540,630,2,2300],[1500,430,0,2700],[1620,300,1,1700],[1740,565,0,3000],[1590,800,1,1550],[1790,820,2,2500],
  ].map(([x,y,kind,price],i)=>{
    const group=svg('g',{},$('pins'));
    const halo=svg('circle',{cx:x,cy:y,r:19,fill:colors[kind],opacity:.14},group);
    svg('circle',{cx:x,cy:y,r:6,fill:colors[kind],stroke:'#ddfaff','stroke-width':1},group);
    return {x,y,kind,price,group,halo,neighbor:i>=10};
  });
  const metro=[];
  for(const [color,points] of [['#f783a8',[[850,350],[1040,350],[1140,410],[1300,465],[1500,430],[1750,430]]],['#7be6c1',[[1090,170],[1140,410],[1110,666],[1270,810],[1570,870]]]]){
    const group=svg('g',{},$('metro-lines'));
    const path=svg('path',{d:points.map(([x,y],i)=>`${i?'L':'M'}${x} ${y}`).join(' '),stroke:color,class:'metro-path'},group);
    const stations=points.map(([x,y],i)=>{const station=svg('g',{},group);svg('circle',{cx:x,cy:y,r:8,stroke:color,class:'station'},station);const text=svg('text',{x:x+15,y:y-16,class:'station-name'},station);text.textContent=x===1140&&y===410?'换乘站':`站点 ${String.fromCharCode(65+i)}`;return station});
    metro.push({path,stations,group});
  }
  const mapGroups=['country-context','province-context','city-context'];
  ['china','zhejiang','hangzhou'].forEach((name,index)=>{
    for(const f of window.PROMO_MAPS[name]){
      const el=svg('path',{d:f.d||'','data-code':f.code},$(mapGroups[index]));
      if(name==='china'&&f.code==='330000'||name==='zhejiang'&&f.code==='330100'||name==='hangzhou'&&f.code==='330106')el.classList.add('selected');
      if(name!=='china'){const label=svg('text',{x:f.x,y:f.y},$(mapGroups[index]));label.textContent=f.name}
    }
  });
  const duration=40;
  const cameraKeys=[
    {t:0,x:1310,y:540,z:.90},{t:8.8,x:1280,y:540,z:.93},{t:12,x:1255,y:520,z:1.025},
    {t:15,x:1260,y:520,z:1.025},{t:17.4,x:1260,y:520,z:1.025},{t:20.2,x:1260,y:520,z:1.025},
    {t:24,x:1280,y:540,z:.81},{t:27.4,x:1370,y:540,z:.87},{t:32.6,x:1370,y:540,z:.87},
    {t:35.6,x:1370,y:540,z:.87},{t:38.5,x:1340,y:540,z:.84},{t:40,x:1340,y:540,z:.84},
  ];
  function cameraAt(t){let a=cameraKeys[0],b=cameraKeys.at(-1);for(let i=1;i<cameraKeys.length;i++)if(t<=cameraKeys[i].t){a=cameraKeys[i-1];b=cameraKeys[i];break}const p=ramp(t,a.t,b.t-a.t||1);return {x:lerp(a.x,b.x,p),y:lerp(a.y,b.y,p),z:lerp(a.z,b.z,p)}}
  function renderAt(seconds){
    const t=clamp(seconds,0,duration),camera=cameraAt(t);
    const screen=(x,y)=>[1285+(x-camera.x)*camera.z,540+(y-camera.y)*camera.z];
    $('camera').setAttribute('transform',`translate(1285 540) scale(${camera.z}) translate(${-camera.x} ${-camera.y})`);
    captions.forEach((el,i)=>{const c=chapters[i];const alpha=i===0?1-ramp(t,3.6,.5):i===chapters.length-1?ramp(t,c.start-.4,.55):ramp(t,c.start-.4,.55)*(1-ramp(t,c.end-.4,.55));show(el,alpha);el.style.transform=`translateY(${(1-ramp(t,c.start-.4,.6))*12}px)`});
    const merge=ramp(t,24.6,1.8);
    pins.forEach((pin,i)=>{const enter=pin.neighbor?.12+.88*ramp(t,24.8+(i-10)*.13,1.2):ramp(t,.4+i*.11,1.3);const typeDim=pin.kind===0?1:1-.85*windowAlpha(t,14.9,20.8,.8);show(pin.group,enter*typeDim);pin.halo.setAttribute('r',17+Math.sin(t*1.8+i)*3)});
    $('neighbor-area').style.fillOpacity=.025+merge*.045;
    $('boundary').style.strokeOpacity=.75-merge*.5;
    $('nearby-rings').style.opacity=.6+.4*Math.sin(t*.25)**2;
    $('target').querySelector('circle').setAttribute('r',46+Math.sin(t*1.8)*4);
    const contextAlpha=windowAlpha(t,4.15,9,.55)+windowAlpha(t,21.6,25.8,.6);
    show($('context-card'),contextAlpha);
    $('context-card').style.transform=`translateY(${(1-clamp(contextAlpha))*12}px)`;
    const navigation=1-ramp(t,9,.3);
    const country=1-ramp(t,5.65,.6),province=ramp(t,5.65,.6)*(1-ramp(t,7.1,.6)),city=ramp(t,7.1,.6);
    show($('country-context'),country*navigation);
    show($('province-context'),province*navigation);
    show($('city-context'),t<10?city:1);
    $('city-context').querySelectorAll('path').forEach(el=>el.classList.toggle('selected',el.dataset.code==='330106'||el.dataset.code==='330110'&&t>=23.3));
    $('context-title').textContent=t<10?'省 → 市 → 区县':'合区 · 选择一起查看的区县';
    $('context-status').textContent=t<10?(t<6?'中国':t<7.5?'浙江省':'杭州市 · 西湖区'):(t<23.3?'✓ 西湖区':'✓ 西湖区 + ✓ 余杭区 → 一起看房');
    const search=windowAlpha(t,4.1,9,.5);
    show($('search-box'),search);$('search-query').textContent='杭州西湖区'.slice(0,Math.floor(ramp(t,4.8,1.6)*5));show($('search-result'),ramp(t,6.3,.5));
    const types=windowAlpha(t,13.6,21.2,.6);show($('type-filter'),types);$('type-filter').querySelectorAll('span').forEach((el,i)=>el.classList.toggle('active',i===(t>=15.2?1:0)));
    const detail=windowAlpha(t,16.5,21.2,.65);show($('listing-card'),detail);$('listing-card').style.transform=`translateX(${(1-detail)*28}px)`;
    const slide=ramp(t,18.5,.85);$('room-slides').style.transform=`translateX(${-slide*50}%)`;['photo-dot-a','photo-dot-b'].forEach((id,i)=>$(id).classList.toggle('active',i===(slide>.5?1:0)));
    const selected=screen(1140,410);$('pin-callout').style.left=`${selected[0]-80}px`;$('pin-callout').style.top=`${selected[1]-65}px`;show($('pin-callout'),windowAlpha(t,15.7,17.5,.45));
    metro.forEach(({path,group,stations},i)=>{const p=ramp(t,28.8+i*.18,2);show(group,ramp(t,28.8,.5));const length=path.getTotalLength();path.style.strokeDasharray=length;path.style.strokeDashoffset=length*(1-p);stations.forEach((el,j)=>show(el,ramp(t,29+j*.22,.55)))});
    $('merge-tool').classList.toggle('active',t>=21.8);$('metro-tool').classList.toggle('active',t>=28.8);$('publish-tool').classList.toggle('active',t>=34.5);
    const publish=windowAlpha(t,35.2,38.6,.5);show($('publish-card'),publish);$('publish-card').style.transform=`translateX(${(1-publish)*25}px)`;
    const published=ramp(t,36.9,.7);show($('published-pin'),published);$('publish-submit').textContent=t>=36.9?'✓ 房源已发布':'发布房源 →';
    const before=ramp(t,34.9,.35)*(1-published);$('published-pin').style.opacity=Math.max(published,before*.5);$('published-pin').style.visibility=t>=34.9?'visible':'hidden';
    $('scope').textContent=t<24.8?'西湖区':'西湖区 + 余杭区';
    $('action-note').textContent=t<4?'':t<9?'搜索地区，快速进入目标区县':t<14?'按住左键拖动 · 滚轮缩放':t<21?'仅切换房源信息，地图位置保持不变':t<28?'选区 → 合并 → 两边房源一起看':t<34?'在同一张地图上，对照站点和房源':t<38?'选点 → 添加资料 → 点亮房源':'选位置，再选房。';
    const active=Math.max(0,chapters.findIndex(c=>t>=c.start&&t<c.end)-1);$('journey').querySelectorAll('div').forEach((el,i)=>el.classList.toggle('active',i<=active));
    const events=[{start:6.7,end:7.7,from:[470,620],to:[430,624],click:7.25},{start:15,end:17.4,from:[1200,600],to:selected,click:16.5},{start:21.3,end:22.7,from:[1000,310],to:[790,160],click:21.9},{start:28,end:29.8,from:[1180,390],to:[910,161],click:28.8},{start:34,end:35.6,from:[1100,380],to:[1035,162],click:34.5},{start:35,end:36.3,from:[1035,162],to:screen(1200,705),click:35.65},{start:36.1,end:37.5,from:screen(1200,705),to:[1670,697],click:36.9}];
    let cursorAlpha=0,x=0,y=0,clickAlpha=0,ringScale=1;
    for(const e of events)if(t>=e.start&&t<e.end){const p=ramp(t,e.start,(e.end-e.start)*.6);x=lerp(e.from[0],e.to[0],p);y=lerp(e.from[1],e.to[1],p);cursorAlpha=windowAlpha(t,e.start,e.end,.22);clickAlpha=windowAlpha(t,e.click,e.click+.45,.15);ringScale=lerp(.5,1.7,ramp(t,e.click,.45))}
    $('cursor').style.transform=`translate(${x}px,${y}px)`;$('cursor').style.opacity=cursorAlpha;$('cursor-ring').style.opacity=clickAlpha;$('cursor-ring').style.transform=`scale(${ringScale})`;
    $('clock').textContent=`00:${Math.floor(t).toString().padStart(2,'0')} / 00:40`;$('progress').style.width=`${t/duration*100}%`;$('seek').value=t;
    return {seconds:t,duration,camera,chapter:chapters.findIndex(c=>t>=c.start&&t<c.end)};
  }
  window.renderAt=renderAt;window.PROMO_DURATION=duration;window.PROMO_PLAYBACK_RATE=1;
  const exporting=new URLSearchParams(location.search).has('export');if(exporting)document.body.classList.add('exporting');
  const resize=()=>{$('film').style.transform=`scale(${Math.min(innerWidth/1920,innerHeight/1080)})`};addEventListener('resize',resize);resize();
  let paused=exporting,time=0,last=performance.now();$('toggle').onclick=()=>{paused=!paused;last=performance.now()};$('seek').oninput=e=>{time=Number(e.target.value);paused=true;renderAt(time)};
  function tick(now){if(!paused){time=Math.min(duration,time+(now-last)/1000);renderAt(time);if(time>=duration)paused=true}last=now;requestAnimationFrame(tick)}
  renderAt(0);if(!exporting)requestAnimationFrame(tick);
})();
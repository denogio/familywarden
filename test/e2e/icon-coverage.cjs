// ICON2 local-only visual/transport check; no router, phone, LuCI session or CDN.
const assert = require('node:assert/strict');
const {readFileSync, mkdirSync} = require('node:fs');
const http = require('node:http');
const path = require('node:path');
const {chromium, expect} = require('@playwright/test');

(async () => {
  const repo = path.resolve(__dirname, '../..');
  const output = process.env.FW_ICON_SCREENSHOTS || '/tmp/fw-icon2-screenshots';
  const prototype = process.env.FW_CONTROL_PROTOTYPE;
  mkdirSync(output, {recursive:true});
  let icons = {}, requests = 0, responseGate = null;
  const server = http.createServer(async (req, res) => {
    if (req.url === '/prototype' && prototype) { res.setHeader('Content-Type','text/html; charset=utf-8'); res.end(readFileSync(prototype)); return; }
    if (req.url.startsWith('/web/')) {
      const name = path.basename(req.url);
      res.setHeader('Content-Type', name.endsWith('.css') ? 'text/css' : 'text/javascript');
      res.end(readFileSync(path.join(repo,'web',name))); return;
    }
    if (req.url === '/api/inventory_icons') {
      let body = ''; for await (const data of req) body += data;
      const query = JSON.parse(body);
      assert.equal(query.profile, 'alex'); assert.ok(query.packages.length <= 32);
      const reply = {icons:{},next:query.offset};
      for (let i=query.offset; i<query.packages.length; i++) {
        const pkg = query.packages[i]; if (icons[pkg]) reply.icons[pkg] = icons[pkg];
        reply.next=i+1;
        if (Buffer.byteLength(JSON.stringify(reply))>60*1024) { delete reply.icons[pkg]; reply.next=i; break; }
      }
      assert.ok(reply.next>query.offset);
      assert.ok(Buffer.byteLength(JSON.stringify(reply))<=60*1024);
      requests++; if (responseGate) await responseGate; res.setHeader('Content-Type','application/json'); res.end(JSON.stringify(reply)); return;
    }
    res.setHeader('Content-Type','text/html; charset=utf-8');
    res.end('<!doctype html><html lang="en"><link rel="stylesheet" href="/web/control.css"><body><main class="fwa" style="padding:32px"><h1>Alex · observed apps</h1><p>96px artwork · consistent 34px tiles at DPR 2 · unavailable images keep initials</p><div id="icons" style="display:grid;grid-template-columns:repeat(10,1fr);gap:16px"></div></main></body></html>');
  });
  await new Promise(resolve => server.listen(0,'127.0.0.1',resolve));
  const base = 'http://127.0.0.1:' + server.address().port;
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage({viewport:{width:1440,height:1000},deviceScaleFactor:2});
    await page.goto(base);
    await page.clock.install();
    await page.clock.pauseAt(new Date());
    icons = await page.evaluate(() => {
      const canvas = document.createElement('canvas'); canvas.width=canvas.height=96;
      const ctx=canvas.getContext('2d'), image=ctx.createImageData(96,96);
      let seed=7; for(let i=0;i<image.data.length;i+=4) {
        seed=(Math.imul(seed,1664525)+1013904223)|0;
        image.data[i]=seed&255; image.data[i+1]=(seed>>>8)&255; image.data[i+2]=(seed>>>16)&255; image.data[i+3]=255;
      }
      ctx.putImageData(image,0,0);
      const noisy=canvas.toDataURL('image/png').split(',')[1], result={};
      for(let i=0;i<210;i++) result['com.example.app'+String(i).padStart(3,'0')]=noisy;
      for (let i=0;i<3;i++) {
        ctx.fillStyle=i===1?'#f5f5f5':['#389c68','#f5f5f5','#d9a23f'][i]; ctx.fillRect(0,0,96,96);
        ctx.fillStyle=['white','#4266ab','#faf5e9'][i];
        if(i===0) { ctx.beginPath(); ctx.moveTo(36,24);ctx.lineTo(72,48);ctx.lineTo(36,72);ctx.fill(); }
        if(i===1) { ctx.beginPath();ctx.arc(48,48,30,0,Math.PI*2);ctx.fill(); }
        if(i===2) ctx.fillRect(8,30,80,36);
        result['com.example.app00'+i]=canvas.toDataURL('image/png').split(',')[1];
      }
      result['com.example.invalid']='iVBORw0KGgoAAAA';
      return result;
    });
    await page.evaluate(async () => {
      const {iconLoader}=await import('/web/family.js');
      const render=iconLoader(async (method,query) => (await fetch('/api/'+method,{method:'POST',body:JSON.stringify(query)})).json());
      const root=document.getElementById('icons');
      const poll = () => {
        root.replaceChildren();
        for(let i=0;i<210;i++) {
          const pkg='com.example.app'+String(i).padStart(3,'0'), row=document.createElement('div');
          row.dataset.pkg=pkg; row.append(render(null,'App '+i,pkg,'alex'),document.createTextNode(' App '+i)); root.append(row);
        }
        for(const kind of ['missing','invalid']) { const row=document.createElement('div');row.id=kind;row.append(render(null,kind,'com.example.'+kind,'alex'));root.append(row); }
      };
      poll();
      setInterval(poll, 15000);

    });
    await expect(page.locator('#icons img')).toHaveCount(210);
    await expect.poll(() => page.locator('[data-pkg="com.example.app209"] img').evaluate(img => img.complete && img.naturalWidth===96)).toBe(true);
    await expect(page.locator('#invalid')).toHaveText('I');
    await expect(page.locator('#missing')).toHaveText('M');
    assert.ok(requests>7,'later transport pages were not exercised');
    assert.equal(await page.locator('#icons .has-image').count(),210);
    const initialRequests = requests;
    for (let cycle=0;cycle<2;cycle++) {
      await page.clock.runFor(15000);
      await expect(page.locator('#icons img')).toHaveCount(210);
      assert.equal(requests,initialRequests,'ordinary polls retransferred cached images');
    }
    let release;
    responseGate = new Promise(resolve => { release=resolve; });
    await page.clock.runFor(270000); // cache reaches five minutes at the next poll
    await expect.poll(() => requests).toBeGreaterThan(initialRequests);
    await expect(page.locator('#icons img')).toHaveCount(210); // renewal is still blocked
    assert.equal(await page.locator('#icons .has-image').count(),210,'stale artwork blinked to initials');
    release(); responseGate=null;
    await expect.poll(() => requests).toBe(initialRequests*2);
    await page.screenshot({path:output+'/icon-grid-dpr2.png',fullPage:true});
    // A deliberately hostile host image default must not introduce a pale rim.
    await page.addStyleTag({content:'img{border:1px solid #f5f5f5;padding:2px;max-width:90%}'});
    await page.locator('main').evaluate(node => node.classList.add('is-dark'));
    const tile = page.locator('[data-pkg="com.example.app000"] .fw-app-icon');
    await tile.screenshot({path:output+'/dark-icon-closeup-dpr2.png'});
    const pixels = await page.evaluate(async png => {
      const img = new Image(); img.src='data:image/png;base64,'+png; await img.decode();
      const canvas=document.createElement('canvas');canvas.width=img.width;canvas.height=img.height;
      const ctx=canvas.getContext('2d');ctx.drawImage(img,0,0);
      return [...ctx.getImageData(34,2,1,1).data];
    }, (await tile.screenshot()).toString('base64'));
    assert.deepEqual(pixels,[56,156,104,255],'opaque artwork must reach the top edge on dark backgrounds');
    assert.equal(await tile.locator('img').evaluate(img => getComputedStyle(img).objectFit),'cover');
    await page.evaluate(async () => {
      const {mount}=await import('/web/control.js');
      const profile={id:'alex',name:'Alex',enabled:'1',policy:'allow',apps:'1',suspend_new:'1',screen_minutes:'60',screen_weekday:'',screen_weekend:'',always_allowed:[]};
      const data={configuration_export:{owner:'core',pending:false,configuration:{version:1,revision:'visual',pact:null,current:{settings:{},profiles:[profile],apps:[{id:'a_1',profile:'alex',package:'com.example.app000',minutes:'30',blocked:'0'}],schedules:[],downtime:[]}}},
        configuration_report:{profiles:{alex:{name:'Alex',status:'allowed'}},configuration:{}},configuration_status:{owner:'core'},core_status:{running:true},version:{},devices:{devices:[],pending:[]},
        configuration_dns_status:{format:'dns-state-v1',profiles:{},pending:false},usage:{profiles:[]},requests:{requests:[]},new_apps:{profiles:[]}};
      const root=document.createElement('main');document.body.replaceChildren(root);
      window.visualControl=mount(root,{theme:'dark',call:async(method,query)=> {
        if(method==='inventory_icons') return (await fetch('/api/'+method,{method:'POST',body:JSON.stringify(query)})).json();
        if(method==='admin_installed') return {profile_id:'alex',snapshot:'visual',apps:[0,1,2].map((i)=>({package:'com.example.app00'+i,label:['Video','Alex music','Alex drawing'][i],rule:i===0?'limit':'none',minutes:i===0?30:undefined,seconds:i===0?1320:undefined,on:[0]})),sources:[{reported:true,installed:3,sent:3}],counts:{all:3,limit:1,unlimited:0,blocked:0,pending:0},total:3,pages:1,page:0,start:0};
        return data[method] || {status:'ok'};
      }});
    });
    await page.getByRole('button',{name:'Edit rules',exact:true}).click();
    await expect(page.locator('.fw-rule-apps img')).toHaveCount(3);
    await page.locator('.fw-rule-apps').locator('..').screenshot({path:output+'/control-apps-dark-dpr2.png'});
    await page.locator('.fw-rulebtn').first().click();
    await page.getByRole('dialog').screenshot({path:output+'/control-rule-modal-dark-dpr2.png'});
    await page.getByRole('dialog').getByRole('button',{name:'Cancel',exact:true}).click();
    if (prototype) {
      await page.goto(base+'/prototype');
      await page.screenshot({path:output+'/prototype-family-dpr2.png',fullPage:true});
      await page.getByRole('button',{name:'View Alex’s details',exact:true}).click();
      await page.getByRole('tab',{name:'History',exact:true}).click();
      await page.evaluate(() => window.scrollTo(0,0));
      await page.screenshot({path:output+'/prototype-history-dpr2.png',fullPage:true});
      await page.getByRole('tab',{name:'Rules',exact:true}).click();
      await page.locator('section').filter({has:page.getByRole('heading',{name:'Apps',exact:true})}).screenshot({path:output+'/prototype-apps-dpr2.png'});
      await page.locator('.fw-rulebtn').first().click();
      await page.getByRole('dialog').screenshot({path:output+'/prototype-rule-modal-dpr2.png'});
    } else {
      console.log('Prototype captures skipped: set FW_CONTROL_PROTOTYPE for visual comparison.');
    }
    console.log(`ICON2: 210 visible icons, all bounded pages (${requests} responses), missing/invalid fallbacks; DPR2 screenshots: ${output}`);
  } finally { await browser.close(); await new Promise(resolve=>server.close(resolve)); }
})().catch(error => { console.error(error); process.exit(1); });

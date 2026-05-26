/* Lumen — scripted demo. NOT live.
   Hover the chat input + press any key (or click Send): the next user line
   auto-types itself, sends, ~4s bot "typing", then bot reply + stage view. */

document.documentElement.classList.add('fonts-pending');
document.fonts.ready.then(()=>document.documentElement.classList.remove('fonts-pending'));

const TYPING_MS=4000, CHAR_MS=42;
const msi=n=>`<span class="msi">${n}</span>`;

/* ---------- catalog ---------- */
const P={
  voyager:{name:'Voyager ANC Headphones',brand:'Auralis',price:329,rating:4.7,ic:'headphones',th:'t-audio',type:'Over-ear',water:'—',weight:'250 g',fit:'Headband',battery:'40 h'},
  aero:{name:'AeroPods Pro 2',brand:'Auralis',price:249,rating:4.8,ic:'earbuds',th:'t-audio',type:'True wireless',water:'IPX4',weight:'52 g',fit:'In-ear',battery:'30 h'},
  pulsebeat:{name:'PulseBeat Sport',brand:'Kairo',price:129,rating:4.6,ic:'earbuds',th:'t-power',type:'True wireless',water:'IPX7',weight:'45 g',fit:'Wing-tip',battery:'24 h'},
  openrun:{name:'OpenRun Bone-Conduction',brand:'Boko',price:159,rating:4.7,ic:'hearing',th:'t-wear',type:'Open-ear',water:'IP67',weight:'29 g',fit:'Around-ear',battery:'10 h'},
  flexneck:{name:'FlexNeck Sport',brand:'Voltex',price:89,rating:4.4,ic:'earbuds',th:'t-track',type:'Neckband',water:'IPX5',weight:'35 g',fit:'Neckband',battery:'18 h'},
  nomad:{name:'Nomad 20K Power Bank',brand:'Voltex',price:79,rating:4.6,ic:'battery_charging_full',th:'t-power'},
  watch:{name:'Pulse Smartwatch 5',brand:'Kairo',price:399,rating:4.5,ic:'watch',th:'t-wear'},
  trail:{name:'TrailSpeaker Mini',brand:'Voltex',price:129,rating:4.4,ic:'speaker',th:'t-power'},
  cam:{name:'ClearView Action Cam',brand:'Optiq',price:279,rating:4.7,ic:'photo_camera',th:'t-cam'},
};
const GRID=['voyager','aero','pulsebeat','openrun','flexneck','nomad','watch','trail','cam'];

/* ---------- card ---------- */
function fieldChip(f){return `<div class="field ${f.hot?'hot':''}"><span class="fl">${f.l}</span><span class="fv">${f.v}</span></div>`;}
function card(key,o={}){
  const p=P[key];
  const badge=o.badge?`<span class="badge ${o.badge.cls}">${o.badge.text}</span>`:'';
  const explain=o.explain?`<div class="card-explain">${msi(o.explain.ic)}<span>${o.explain.t}</span></div>`:'';
  const fields=o.fields?`<div class="card-fields">${o.fields.map(fieldChip).join('')}</div>`:'';
  return `<div class="card ${o.pick?'pick':''}">
    <div class="card-img ${p.th}">${badge}<span class="fav ${o.liked?'liked':''}">${msi('favorite')}</span><span class="card-ico msi">${p.ic}</span><button class="add">${msi('add')}</button></div>
    <div class="card-body">
      <div class="card-title">${p.name}</div>
      <div class="card-price-row"><span class="card-price">$${p.price}.00</span><span class="card-rating"><span class="star">★</span> ${p.rating}</span></div>
      <div class="card-brand">${p.brand}</div>
      ${explain}${fields}
    </div>
  </div>`;
}
const rowsOf=(keys,cols=3)=>{let o='';for(let i=0;i<keys.length;i+=cols){o+=`<div class="row" style="grid-template-columns:repeat(${cols},1fr)">${keys.slice(i,i+cols).map(k=>card(k)).join('')}</div>`;}return o;};

/* ---------- stage views ---------- */
const stage=document.getElementById('stage');

function viewGrid(){
  stage.innerHTML=`<div class="cat-header">${msi('search')}<span class="cnt">9 products</span></div>${rowsOf(GRID)}`;
}

const FIND=[
  {k:'aero',explain:{ic:'cable',t:'True wireless — no cable to tangle · IPX4 sweat-resistant'},fields:[{l:'Type',v:'True wireless',hot:1},{l:'Water',v:'IPX4',hot:1},{l:'Weight',v:'52 g'}]},
  {k:'pulsebeat',explain:{ic:'water_drop',t:'Secure wing-tip · IPX7 survives sweat & rain'},fields:[{l:'Type',v:'True wireless',hot:1},{l:'Water',v:'IPX7',hot:1},{l:'Weight',v:'45 g'}]},
  {k:'openrun',explain:{ic:'hearing',t:'Open-ear, nothing inside · IP67 fully waterproof'},fields:[{l:'Type',v:'Open-ear',hot:1},{l:'Water',v:'IP67',hot:1},{l:'Weight',v:'29 g'}]},
  {k:'flexneck',explain:{ic:'fitness_center',t:'Lightweight neckband · IPX5 sweat-proof'},fields:[{l:'Type',v:'Neckband',hot:1},{l:'Water',v:'IPX5',hot:1},{l:'Weight',v:'35 g'}]},
];
function viewFind(){
  const cards=FIND.map(f=>card(f.k,{explain:f.explain,fields:f.fields})).join('');
  stage.innerHTML=`<div class="sec-head"><span class="st">${msi('auto_awesome')}Gym-proof &amp; weatherproof — like the Voyager, minus the wires</span><span class="ss green">4 matches · why they fit</span></div><div class="row" style="grid-template-columns:repeat(2,1fr)">${cards}</div>`;
}

const REFINE=[
  {k:'pulsebeat',explain:{ic:'directions_run',t:'Wing-tip locks in for sprints · IPX7'},fields:[{l:'Price',v:'$129'},{l:'Water',v:'IPX7',hot:1},{l:'Weight',v:'45 g'},{l:'Fit',v:'Wing-tip',hot:1}]},
  {k:'openrun',explain:{ic:'directions_run',t:'Open-ear keeps you aware of traffic · IP67'},fields:[{l:'Price',v:'$159'},{l:'Water',v:'IP67',hot:1},{l:'Weight',v:'29 g'},{l:'Fit',v:'Around-ear',hot:1}]},
  {k:'flexneck',explain:{ic:'directions_run',t:'Featherweight neckband for long runs · IPX5'},fields:[{l:'Price',v:'$89'},{l:'Water',v:'IPX5',hot:1},{l:'Weight',v:'35 g'},{l:'Fit',v:'Neckband',hot:1}]},
];
function viewRefine(){
  const cards=REFINE.map(f=>card(f.k,{liked:f.k!=='flexneck',explain:f.explain,fields:f.fields})).join('');
  stage.innerHTML=`<div class="sec-head"><span class="st">${msi('auto_awesome')}Under $200 · built for running</span><span class="ss green">3 results · fields updated to fit, water &amp; weight</span></div><div class="row" style="grid-template-columns:repeat(3,1fr)">${cards}</div>`;
}

function compareTable(set,tags,hotKeys,winner){
  const cols=set.length;
  const specs=[
    {lab:'Price',g:k=>'$'+P[k].price+'.00',key:'price'},
    {lab:'Rating',g:k=>`<span class="card-rating"><span class="star">★</span> ${P[k].rating}</span>`,key:'rating'},
    {lab:'Water resistance',g:k=>P[k].water,key:'water'},
    {lab:'Weight',g:k=>P[k].weight,key:'weight'},
    {lab:'Fit',g:k=>P[k].fit,key:'fit'},
    {lab:'Battery',g:k=>P[k].battery,key:'battery'},
  ];
  const head=set.map(k=>{const p=P[k];const t=tags[k];
    return `<div class="cmp-cell cmp-prod"><div class="cmp-mini ${p.th}">${msi(p.ic)}</div><span class="cmp-name">${p.name}</span>${t?`<span class="cmp-tag ${t.cls}">${t.text}</span>`:''}</div>`;}).join('');
  const body=specs.map(s=>{
    const hot=hotKeys.includes(s.key);
    const cells=set.map(k=>{const win=hot&&k===winner;return `<div class="cmp-cell ${win?'cmp-winv':''}">${s.g(k)}</div>`;}).join('');
    return `<div class="cmp-row ${hot?'hot':''}"><div class="cmp-cell lab">${s.lab}</div>${cells}</div>`;
  }).join('');
  const add=`<div class="cmp-row"><div class="cmp-cell lab"></div>${set.map(()=>`<div class="cmp-cell"><button class="cmp-add">${msi('add')}</button></div>`).join('')}</div>`;
  return `<div class="compare" style="--cols:${cols}"><div class="cmp-row head"><div class="cmp-cell lab">Product</div>${head}</div>${body}${add}</div>`;
}
const CMP=['pulsebeat','openrun','flexneck'];
function viewCompare(){
  const tags={openrun:{cls:'win',text:'Best for you'},pulsebeat:{cls:'value',text:'Best value'}};
  stage.innerHTML=`<div class="sec-head"><span class="st">${msi('auto_awesome')}Your favorites, compared for running</span><span class="ss green">water, weight &amp; fit highlighted for you</span></div>${compareTable(CMP,tags,['water','weight','fit'],'openrun')}`;
}

function applyHalloween(){
  document.documentElement.dataset.theme='halloween';
  // keep current stage; the section title gets a seasonal touch
  const st=stage.querySelector('.sec-head .st');
  if(st) st.insertAdjacentHTML('beforeend',' 🎃');
}

function viewLanding(){
  stage.innerHTML=`
  <div class="pres">
    <div class="pres-hero">
      <div class="pres-eyebrow">Your running audio, explained</div>
      <div class="pres-h1">Earbuds that survive the gym 🎃</div>
      <p class="pres-lead">No wires to tangle, sweat- and rain-proof, and light enough to forget you're wearing them. Here's how your three picks stack up — and how to choose between them.</p>
      <div class="pres-stats">
        <div class="ps"><span class="psn">3</span><span class="psl">picks for you</span></div>
        <div class="ps"><span class="psn">IP67</span><span class="psl">top water rating</span></div>
        <div class="ps"><span class="psn">29 g</span><span class="psl">lightest</span></div>
        <div class="ps"><span class="psn">$89+</span><span class="psl">from</span></div>
      </div>
    </div>

    <div class="pres-grid">
      <div class="pres-box">
        <h3>Why these three</h3>
        <div class="feat"><span class="fi">${msi('water_drop')}</span><div><div class="feat-t">Weatherproof by design</div><div class="feat-d">IPX5 to IP67 — they shrug off sweat, splashes and a full downpour.</div></div></div>
        <div class="feat"><span class="fi">${msi('directions_run')}</span><div><div class="feat-t">They stay put</div><div class="feat-d">Wing-tips and around-ear hooks that don't bounce on a sprint.</div></div></div>
        <div class="feat"><span class="fi">${msi('cable')}</span><div><div class="feat-t">Nothing to tangle</div><div class="feat-d">True-wireless and open-ear — no cable in your way.</div></div></div>
      </div>
      <div class="pres-new">
        <div class="new-badge">${msi('bolt')} Just published in Canvas</div>
        <h3>${msi('fitness_center')} Fit &amp; Sweat Guide</h3>
        <div class="fitgrid">
          <div class="fitcell">${msi('water_drop')}<div class="ft">IP67</div><div class="fd">OpenRun — fully waterproof</div></div>
          <div class="fitcell">${msi('monitor_weight')}<div class="ft">29 g</div><div class="fd">Lightest of the three</div></div>
          <div class="fitcell">${msi('directions_run')}<div class="ft">Wing-tip</div><div class="fd">PulseBeat — locked-in fit</div></div>
        </div>
      </div>
    </div>

    <div class="pres-box">
      <h3>How to choose</h3>
      <div class="pres-choose">
        <div class="pc-col"><div class="pc-h">${msi('directions_run')} Pick OpenRun if…</div><p>you run outdoors and want to hear traffic — open-ear, IP67, only 29 g.</p></div>
        <div class="pc-col"><div class="pc-h">${msi('fitness_center')} Pick PulseBeat if…</div><p>you train indoors and want isolation — sealed wing-tip, IPX7, best value.</p></div>
        <div class="pc-col"><div class="pc-h">${msi('savings')} Pick FlexNeck if…</div><p>budget matters most — featherweight neckband at just $89.</p></div>
      </div>
    </div>

    <div class="pres-grid">
      <div class="pres-box">
        <h3>What runners say</h3>
        <p class="quote">"Ran a half-marathon in the rain — never skipped, never slipped."</p>
        <p class="quote-by">— Marcus, verified buyer ★★★★★</p>
        <p class="quote">"Open-ear means I still hear cars. Game changer for road running."</p>
        <p class="quote-by">— Priya, verified buyer ★★★★★</p>
      </div>
      <div class="pres-box">
        <h3>In the box &amp; warranty</h3>
        <div class="feat"><span class="fi">${msi('inventory_2')}</span><div><div class="feat-t">Charging case + 3 ear-tip sizes</div><div class="feat-d">Find your fit out of the box.</div></div></div>
        <div class="feat"><span class="fi">${msi('verified')}</span><div><div class="feat-t">2-year sweat warranty</div><div class="feat-d">Covered against moisture damage.</div></div></div>
        <div class="feat"><span class="fi">${msi('local_shipping')}</span><div><div class="feat-t">Free next-day delivery</div><div class="feat-d">Order today, train tomorrow.</div></div></div>
      </div>
    </div>

    <div class="pres-cta">
      <div><div class="pcp">Top pick for you — OpenRun Bone-Conduction</div><div><span class="pcprice">$159</span></div></div>
      <button class="pcbtn">${msi('shopping_cart')} Add to cart</button>
    </div>
  </div>`;
}

/* ---------- chat plumbing ---------- */
const thread=document.getElementById('thread');
const ciField=document.getElementById('ciField');
const ciPlc=document.getElementById('ciPlc');
const ciText=document.getElementById('ciText');
const ciSend=document.getElementById('ciSend');
const timelineEl=document.getElementById('timeline');

function botBubble(text,chip){const el=document.createElement('div');el.className='bw';el.innerHTML=`<div class="bav">${msi('smart_toy')}</div><div class="bubble bot">${text}${chip?`<span class="chip">${msi('filter_list')}${chip}</span>`:''}</div>`;thread.appendChild(el);thread.scrollTop=thread.scrollHeight;}
function userBubble(text){const el=document.createElement('div');el.className='uw';el.innerHTML=`<div class="bubble user">${text}</div>`;thread.appendChild(el);thread.scrollTop=thread.scrollHeight;}
function showTyping(){const el=document.createElement('div');el.className='typing';el.innerHTML=`<div class="bav">${msi('smart_toy')}</div><div class="dots"><i></i><i></i><i></i></div>`;thread.appendChild(el);thread.scrollTop=thread.scrollHeight;return el;}

const tl=[];
function pushTimeline(label){tl.push(label);timelineEl.innerHTML=tl.map((s,i)=>`${i>0?'<div class="tl-conn"></div>':''}<div class="tl-step ${i===tl.length-1?'active':''}"><span class="tl-dot"></span><span class="tl-text">${s}</span></div>`).join('');timelineEl.scrollTop=timelineEl.scrollHeight;}

function autoType(text,cb){ciPlc.style.display='none';ciField.classList.add('typing-on');ciText.textContent='';const chars=[...text];let i=0;const t=setInterval(()=>{ciText.textContent+=chars[i++];if(i>=chars.length){clearInterval(t);setTimeout(cb,320);}},CHAR_MS);}
function resetField(){ciText.textContent='';ciField.classList.remove('typing-on');ciPlc.style.display='';}

/* ---------- script (chat beats) ---------- */
const BEATS=[
  {user:"Find me earbuds like these, but that won't tangle at the gym and survive sweat & rain",
   bot:"Got it — for the gym you want no dangling cable and a real water rating. Here are 4 that fit, each with why.",
   chip:"4 matches · why they fit", view:viewFind, tl:"Find similar"},
  {user:"Make it under $200 and built for running",
   bot:"Updated. Dropped the pricier pair and switched the cards to show fit, water rating and weight — what matters on a run.",
   chip:"3 results · fields updated", view:viewRefine, tl:"Refine"},
  {user:"I like PulseBeat and OpenRun — compare these for me",
   bot:"Here they are side by side, with water resistance, weight and fit highlighted for you. OpenRun is the best fit for running.",
   chip:"compared on what matters to you", view:viewCompare, tl:"Compare"},
  {user:"Switch the theme to Halloween 🎃",
   bot:"Done — restyled the whole storefront live. Same widgets, new look.",
   chip:"theme applied on the fly", view:applyHalloween, tl:"Halloween theme"},
  {user:"Make a landing page that explains all this",
   bot:"Built a landing that explains the picks — and it already includes the Fit & Sweat Guide widget you just added.",
   chip:"landing built · new widget included", view:viewLanding, tl:"Landing page", needWidget:true},
];

let stepIndex=0, busy=false, widgetPublished=false;

function storeVisible(){return !document.getElementById('store').classList.contains('hidden') && document.getElementById('admin').classList.contains('hidden');}

function advanceChat(){
  if(busy||stepIndex>=BEATS.length||!storeVisible()) return;
  const beat=BEATS[stepIndex];
  if(beat.needWidget && !widgetPublished){ flashAdmin(); return; }
  busy=true; ciSend.disabled=true;
  autoType(beat.user,()=>{
    userBubble(beat.user); resetField();
    const t=showTyping();
    setTimeout(()=>{
      t.remove(); botBubble(beat.bot,beat.chip); beat.view(); pushTimeline(beat.tl);
      stepIndex++; busy=false;
      ciSend.disabled = stepIndex>=BEATS.length;
      if(stepIndex<BEATS.length && BEATS[stepIndex].needWidget && !widgetAdded) flashAdmin();
    },TYPING_MS);
  });
}
ciSend.addEventListener('click',advanceChat);

/* keystroke trigger: hover/focus input + any key */
let fieldHover=false;
ciField.addEventListener('mouseenter',()=>{fieldHover=true;if(canType())ciField.classList.add('armed');});
ciField.addEventListener('mouseleave',()=>{fieldHover=false;ciField.classList.remove('armed');});
function canType(){return storeVisible() && !busy && stepIndex<BEATS.length && !(BEATS[stepIndex].needWidget&&!widgetPublished);}
document.addEventListener('keydown',e=>{
  if(e.metaKey||e.ctrlKey||e.altKey) return;
  if((fieldHover||document.activeElement===ciField) && canType()){ e.preventDefault(); advanceChat(); }
});

/* ---------- admin · KeepstarCanvas ---------- */
const PRESETS=[
  {name:'product_card',status:'published'},
  {name:'smart_picks',status:'published'},
  {name:'comparison_table',status:'published'},
  {name:'gallery',status:'published'},
  {name:'bundle_builder',status:'published'},
];
const COMPS=[{name:'price_badge',cat:'atom'},{name:'rating_stars',cat:'atom'},{name:'spec_row',cat:'molecule'},{name:'fit_meter',cat:'molecule'}];
const cvPresets=document.getElementById('cvPresets');
const cvComps=document.getElementById('cvComps');
const cvTitle=document.getElementById('cvTitle');
const cvStatus=document.getElementById('cvStatus');
const cvActions=document.getElementById('cvActions');
const cvCanvas=document.getElementById('cvCanvas');
const cvInsp=document.getElementById('cvInsp');
const cvAiField=document.getElementById('cvAiField');
const cvAiPlc=document.getElementById('cvAiPlc');
const cvAiText=document.getElementById('cvAiText');
const cvGenerate=document.getElementById('cvGenerate');

let selectedPreset=null, generating=false;

function renderPresets(){
  cvPresets.innerHTML=PRESETS.map(p=>`<button class="cv-preset ${selectedPreset===p.name?'sel':''}" data-n="${p.name}"><span class="cv-pname">${p.name}</span><span class="cv-pill ${p.status}">${p.status}</span></button>`).join('');
  cvPresets.querySelectorAll('.cv-preset').forEach(b=>b.addEventListener('click',()=>selectPreset(b.dataset.n)));
  cvComps.innerHTML=COMPS.map(c=>`<div class="cv-comp-item"><span class="cv-cname">${c.name}</span><span class="cv-ccat">${c.cat}</span></div>`).join('');
}
function fitTile(status){
  return `<div class="cv-tile">
    <div class="cv-tile-bar">${msi('fitness_center')}<b>fit_sweat_guide</b><span class="cv-pill ${status}">${status}</span></div>
    <div class="cv-tile-body"><div class="cv-fitgrid">
      <div class="cv-fitcell">${msi('water_drop')}<div class="t">IP67</div><div class="d">Water rating</div></div>
      <div class="cv-fitcell">${msi('monitor_weight')}<div class="t">29 g</div><div class="d">Weight</div></div>
      <div class="cv-fitcell">${msi('directions_run')}<div class="t">Wing-tip</div><div class="d">Secure fit</div></div>
    </div></div>
  </div>`;
}
function selectPreset(name){
  selectedPreset=name;
  const p=PRESETS.find(x=>x.name===name);
  renderPresets();
  cvTitle.textContent=name;
  cvStatus.className=`cv-status ${p.status}`; cvStatus.textContent=p.status; cvStatus.classList.remove('hidden');
  cvActions.innerHTML = p.status==='draft' ? `<button class="cv-pub" id="cvPublish">${msi('publish')} Publish</button><button class="cv-del">Delete</button>` : `<button class="cv-del">Delete</button>`;
  const pub=document.getElementById('cvPublish');
  if(pub) pub.addEventListener('click',publishPreset);
  if(name==='fit_sweat_guide'){
    cvCanvas.style.alignItems='flex-start';cvCanvas.style.justifyContent='center';cvCanvas.style.padding='28px';
    cvCanvas.innerHTML=fitTile(p.status);
    cvInsp.innerHTML=`
      <div class="cv-fld"><span class="cv-fld-l">Name</span><div class="cv-fld-v">fit_sweat_guide</div></div>
      <div class="cv-fld"><span class="cv-fld-l">Category</span><div class="cv-fld-v">comparison</div></div>
      <div class="cv-fld"><span class="cv-fld-l">Entity type</span><div class="cv-fld-v">product</div></div>
      <div class="cv-fld"><span class="cv-fld-l">Description</span><div class="cv-fld-v">Water rating, weight and secure-fit at a glance.</div></div>
      ${p.status==='published'?`<div class="cv-insp-note">${msi('check_circle')}Now usable in the assistant</div>`:''}`;
  }
}
function publishPreset(){
  const p=PRESETS.find(x=>x.name===selectedPreset); if(!p) return;
  p.status='published'; widgetPublished=true;
  document.getElementById('openAdmin').classList.remove('pulse');
  selectPreset(p.name);
}
function genWidget(){
  if(generating || widgetPublished) return;
  generating=true; cvGenerate.disabled=true;
  cvAiPlc.style.display='none';
  document.getElementById('cvAiField').parentElement.classList.add('gen');
  const prompt="A widget that shows water rating, weight and secure-fit at a glance";
  let i=0;
  const t=setInterval(()=>{
    cvAiText.textContent+=prompt[i++];
    if(i>=prompt.length){
      clearInterval(t);
      setTimeout(()=>{
        document.querySelector('.cv-ai').classList.remove('gen');
        cvCanvas.innerHTML=`<div class="cv-gen-spin"><div class="ring"></div><span>Generating widget…</span></div>`;
        setTimeout(()=>{
          PRESETS.push({name:'fit_sweat_guide',status:'draft'});
          generating=false;
          selectPreset('fit_sweat_guide');
        },2600);
      },350);
    }
  },CHAR_MS);
}
cvGenerate.addEventListener('click',genWidget);

function flashAdmin(){document.getElementById('openAdmin').classList.add('pulse');}
document.getElementById('openAdmin').addEventListener('click',()=>{renderPresets();document.getElementById('store').classList.add('hidden');document.getElementById('admin').classList.remove('hidden');});
document.getElementById('closeAdmin').addEventListener('click',()=>{document.getElementById('admin').classList.add('hidden');document.getElementById('store').classList.remove('hidden');});

/* ---------- welcome -> store ---------- */
let welcomeStep=0;
function goWelcome(){
  if(welcomeStep===0){
    welcomeStep=1;
    document.getElementById('foExplain').classList.add('hidden');
    document.getElementById('foProduct').classList.remove('hidden');
    document.getElementById('foTitle').innerHTML='Looking at the Voyager ANC Headphones';
    document.getElementById('foSub').textContent='Want alternatives, a deeper look, or a more gym-friendly pick?';
    document.getElementById('foPlc').textContent='Ask about this product…';
  } else {
    enterStore();
  }
}
function enterStore(){
  document.getElementById('firstOpen').classList.add('hidden');
  document.getElementById('store').classList.remove('hidden');
  viewGrid();
  botBubble("I see you're viewing the Voyager ANC Headphones. Want alternatives, or something more gym-friendly?");
}
document.getElementById('foBtn').addEventListener('click',e=>{e.stopPropagation();goWelcome();});
document.getElementById('foSearch').addEventListener('click',goWelcome);
document.querySelectorAll('.cap').forEach(c=>c.addEventListener('click',goWelcome));

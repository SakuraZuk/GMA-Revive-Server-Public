"use strict";
const get = id => document.getElementById(id);
const text = value => JSON.stringify(value, null, 2);
let activities = [];
function toast(message){const el=get("toast");el.hidden=false;el.textContent=message;clearTimeout(toast.timer);toast.timer=setTimeout(()=>{el.hidden=true},3200)}
function when(sec){if(!sec)return "—";const d=new Date(sec*1000);const p=n=>String(n).padStart(2,"0");return d.getFullYear()+"-"+p(d.getMonth()+1)+"-"+p(d.getDate())+" "+p(d.getHours())+":"+p(d.getMinutes())}
function tag(status){const on=["进行中","排期中","已登录"].includes(status),off=["已关闭","已结束","排期结束","排期关闭"].includes(status);return `<span class="tag ${on?"on":off?"off":"wait"}">${status||"—"}</span>`}
async function api(path,body,method){
 const response=await fetch(path,{method:method||(body?"POST":"GET"),headers:{"Content-Type":"application/json"},credentials:"same-origin",body:body?JSON.stringify(body):undefined});
 const raw=await response.text();let data={};try{data=raw?JSON.parse(raw):{}}catch{data={error:raw}}
 if(!response.ok)throw Error(data.error||raw||response.status);
 return data;
}
function showApp(user){get("login").hidden=true;get("app").hidden=false;get("who").textContent=user||"admin"}
get("login-form").addEventListener("submit",async event=>{
 event.preventDefault();get("login-error").textContent="";
 try{const data=await api("/admin/session",{user:get("user").value.trim(),password:get("password").value});showApp(data.user);openTab("home")}catch(error){get("login-error").textContent=error.message}
});
get("logout").addEventListener("click",async()=>{await api("/admin/logout",{});location.reload()});
document.querySelectorAll("nav button").forEach(button=>button.addEventListener("click",()=>openTab(button.dataset.tab)));
function openTab(name){
 document.querySelectorAll("nav button").forEach(button=>button.classList.toggle("active",button.dataset.tab===name));
 document.querySelectorAll(".view").forEach(view=>{view.hidden=view.id!==name});
 const load={home:loadHome,online:loadOnline,activity:loadActivities,users:()=>{},notice:loadNotices,mail:loadMails,gacha:loadGacha}[name];
 if(load)load().catch(error=>toast(error.message));
}
async function boot(){try{const data=await api("/admin/session");showApp(data.user);openTab("home")}catch{}}
async function loadHome(){
 const data=await api("/admin/overview");
 const cards=[["在线人数",data.在线人数],["进行中活动",data.活动进行中],["公告",data.公告],["全服邮件",data.全服邮件]];
 get("stats").innerHTML=cards.map(([name,value])=>`<div class="stat"><span>${name}</span><b>${value??0}</b></div>`).join("");
}
async function loadOnline(){
 const data=await api("/admin/online");
 get("online-count").textContent="当前 "+data.count+" 条连接";
 get("online-rows").innerHTML=data.players.map(p=>`<tr><td>${escape(p.nickname||"—")}</td><td>${p.uid||"—"}</td><td>${p.hostnum||"—"}</td><td>${p.level||"—"}</td><td>${escape(p.phase)}</td><td>${p.battle?"战斗中":"—"}</td><td>${escape(p.device||"—")}</td></tr>`).join("")||`<tr><td colspan="7">现在没有在线馆主</td></tr>`;
}
get("refresh-online").addEventListener("click",()=>loadOnline().catch(error=>toast(error.message)));
async function loadActivities(){
 const data=await api("/admin/activities");activities=data.activities||[];renderActivities();
}
function renderActivities(){
 const q=get("activity-q").value.trim();
 const rows=activities.filter(a=>!q||String(a.activity_id).includes(q)||String(a.name).includes(q));
 get("activity-rows").innerHTML=rows.map(a=>`<tr><td>${a.activity_id}</td><td>${escape(a.name||"未命名活动")}</td><td>${tag(a.状态)}</td><td>${a.permanent&&a.begin?"常驻自 "+when(a.begin):when(a.begin)}</td><td>${a.permanent?"常驻":when(a.end)}</td><td>${a.need_power||0}</td></tr>`).join("");
}
get("activity-q").addEventListener("input",renderActivities);
get("find").addEventListener("click",async()=>{
 try{const data=await api("/admin/players?q="+encodeURIComponent(get("search").value));
  get("players").innerHTML=`<table><tbody>${data.players.map(av=>`<tr class="click" data-oid="${av.avatar_oid}" data-name="${escape(av.nickname)}"><td>${escape(av.nickname)}</td><td>UID ${av.uid}</td><td>Lv.${av.level}</td><td>${av.hostnum}</td></tr>`).join("")}</tbody></table>`;
  get("players").querySelectorAll("tr").forEach(row=>row.addEventListener("click",()=>{get("oid").value=row.dataset.oid;get("nickname").value=row.dataset.name}))
 }catch(error){toast(error.message)}
});
function template(){
 const mode=get("action").value;
 get("profile-fields").hidden=mode!=="profile";get("asset-fields").hidden=!["material_add","card_grant"].includes(mode);get("payload-fields").hidden=!["mail","rune"].includes(mode);
 if(mode==="mail")get("payload").value=text({mid:1,title:"馆务邮件",content:"请查收",sender:"阿克夏馆务处",attachments:{"12":1},expires_at:0});
 if(mode==="rune")get("payload").value=text({suit:1101,pos:1,star:5,level:1});
}
get("action").addEventListener("change",template);template();
get("submit").addEventListener("click",async()=>{
 const button=get("submit");button.disabled=true;
 try{const mode=get("action").value;const req={avatar_oid:get("oid").value.trim(),receipt:get("receipt").value.trim(),operator:get("who").textContent,reason:get("reason").value.trim()};let path="/admin/player/update";
  if(mode==="mail"||mode==="rune"){req[mode==="mail"?"mail":"spec"]=JSON.parse(get("payload").value);path=mode==="mail"?"/admin/mail/issue":"/admin/runes/grant"}
  else{req.operation=mode;if(mode==="profile"){req.nickname=get("nickname").value;req.level=Number(get("level").value)}if(mode==="material_add"){req.material_id=Number(get("asset-id").value);req.amount=Number(get("asset-count").value)}if(mode==="card_grant"){req.card_id=Number(get("asset-id").value);req.count=Number(get("asset-count").value)}}
  get("result").textContent=text(await api(path,req));toast("已保存");
 }catch(error){get("result").textContent=error.message;toast(error.message)}finally{button.disabled=false}
});
async function loadNotices(){const data=await api("/admin/notices");get("notice-rows").innerHTML=(data.notices||[]).map(n=>`<tr><td><b>${n.pinned?"置顶 · ":""}${escape(n.title)}</b><div class="help">${escape(n.kind)} · ${escape(n.body).slice(0,80)}</div></td><td>${n.enabled?"展示中":"已下架"}</td><td><button class="ghost" data-id="${n.id}">删除</button></td></tr>`).join("")||`<tr><td colspan="3">还没有公告</td></tr>`;
 get("notice-rows").querySelectorAll("button").forEach(button=>button.addEventListener("click",async()=>{await api("/admin/notices/remove",{id:button.dataset.id});toast("已删除");loadNotices()}))}
get("notice-form").addEventListener("submit",async event=>{event.preventDefault();const form=new FormData(event.target);try{await api("/admin/notices",{title:form.get("title"),body:form.get("body"),kind:form.get("kind"),pinned:form.get("pinned")==="on",enabled:form.get("enabled")==="on",begin:0,end:0});event.target.reset();toast("公告已发布");loadNotices()}catch(error){toast(error.message)}});
async function loadMails(){const data=await api("/admin/broadcasts");get("mail-rows").innerHTML=(data.mails||[]).map(m=>`<tr><td>${escape(m.title)}<div class="help">${escape(m.receipt)}</div></td><td>${m.sent}</td><td>${when(m.time)}</td></tr>`).join("")||`<tr><td colspan="3">还没有全服邮件</td></tr>`}
get("mail-form").addEventListener("submit",async event=>{event.preventDefault();const form=new FormData(event.target);const body={};for(const [key,value] of form.entries())body[key]=["material_id","amount"].includes(key)?Number(value):value;try{const data=await api("/admin/broadcasts",body);toast(data.重复请求?"这张收据已经发过":"已发送给 "+data.sent+" 位馆主");loadMails()}catch(error){toast(error.message)}});
async function loadGacha(){const data=await api("/admin/gacha");get("gacha-rows").innerHTML=data.pools.map(p=>{const s=p.schedule||{};return `<tr class="click" data-id="${p.pool_id}"><td>#${p.pool_id}<div class="help">${p.cards} 张幻书 · 活动 ${p.activity||"—"}</div></td><td>${p.cost_material}×${p.cost_count}</td><td>${p.guarantee||"—"}</td><td>${tag(p.状态)}</td><td>${s.begin?when(s.begin)+" → "+when(s.end):"—"}</td></tr>`}).join("");
 get("gacha-rows").querySelectorAll("tr").forEach(row=>row.addEventListener("click",()=>{get("gacha-form").pool_id.value=row.dataset.id}))}
get("gacha-form").addEventListener("submit",async event=>{event.preventDefault();const form=event.target;const begin=Math.floor(new Date(form.begin.value).getTime()/1000),end=Math.floor(new Date(form.end.value).getTime()/1000);try{await api("/admin/gacha",{pool_id:Number(form.pool_id.value),begin,end,note:form.note.value,enabled:form.enabled.checked});toast("排期已保存");loadGacha()}catch(error){toast(error.message)}});
function escape(value){return String(value??"").replace(/[&<>"']/g,ch=>({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"}[ch]))}
boot();

"use strict";
const get=id=>document.getElementById(id);
let events=[],timer=null;
const text=value=>JSON.stringify(value,null,2);
async function request(path,body){
 const response=await fetch(path,{method:body?"POST":"GET",headers:{"Content-Type":"application/json",Authorization:"Bearer "+get("token").value},body:body?JSON.stringify(body):undefined});
 const result=await response.text();if(!response.ok)throw Error(result+"（"+response.status+"）");return JSON.parse(result);
}
function report(fn){return async()=>{try{await fn()}catch(error){get("result").textContent="未完成："+error.message}}}
get("lock").addEventListener("click",()=>{get("token").value="";get("result").textContent="令牌已清除"});
function template(){
 const mode=get("action").value;
 get("profile-fields").hidden=mode!=="profile";get("material-fields").hidden=!mode.startsWith("material_");get("card-fields").hidden=mode!=="card_grant";get("payload-fields").hidden=!["mail","rune"].includes(mode);
 get("offer-fields").hidden=mode!=="recommendation_issue";
 if(mode==="recommendation_issue"&&!get("offer-expired").value){const expiry=new Date(Date.now()+86400000);get("offer-expired").value=new Date(expiry.getTime()-expiry.getTimezoneOffset()*60000).toISOString().slice(0,16)}
 if(mode==="mail")get("payload").value=text({mid:1,title:"管理邮件",content:"邮件正文",sender:"管理员",attachments:{"12":1},expires_at:0});
 if(mode==="rune")get("payload").value=text({suit:1101,pos:1,star:5,level:1});
 const hints={profile:"昵称和等级按原生规则校验；正在战斗的玩家须结束后再修改。",material_add:"增加普通库存材料，同时记录累计获得数量。",material_set:"减少余额会保留累计获得数量；增加余额会增加累计获得数量。",card_grant:"每份生成独立的初始幻书实例，最多50份。",recommendation_issue:"报价必须符合原表折扣区间；开始展示后按分钟计时，购买仍校验解锁与终身限购。",rune:"按原生套装、位置和星级生成契印。",mail:"邮件编号须在邮箱内唯一，附件领取后入账；有效期0表示永久。"};get("help").textContent=hints[mode];
}
get("action").addEventListener("change",template);template();
get("find").addEventListener("click",report(async()=>{const data=await request("/admin/players?q="+encodeURIComponent(get("search").value));get("players").replaceChildren();for(const av of data.players){const button=document.createElement("button");button.textContent=av.nickname+" · 编号"+av.uid+" · 等级"+av.level;button.addEventListener("click",()=>{get("oid").value=av.avatar_oid;get("nickname").value=av.nickname;get("level").value=av.level;get("gender").value=av.gender||1});get("players").append(button)}get("result").textContent="读取"+data.players.length+"个玩家（最多100个）"}));
get("detail").addEventListener("click",report(async()=>{const data=await request("/admin/players/"+encodeURIComponent(get("oid").value.trim()));get("result").textContent=text(data);events=data.progress.server_battle?.event_log||[];get("timeline").max=Math.max(events.length-1,0);get("timeline").value=0;draw()}));
get("submit").addEventListener("click",report(async()=>{
 const button=get("submit");button.disabled=true;
 try{const mode=get("action").value;const req={avatar_oid:get("oid").value.trim(),receipt:get("receipt").value.trim(),operator:get("operator").value.trim(),reason:get("reason").value.trim()};let path="/admin/player/update";
 if(mode==="mail"||mode==="rune"){req[mode==="mail"?"mail":"spec"]=JSON.parse(get("payload").value);path=mode==="mail"?"/admin/mail/issue":"/admin/runes/grant"}
 else{req.operation=mode;if(mode==="profile"){req.nickname=get("nickname").value;req.level=Number(get("level").value);req.gender=Number(get("gender").value)}if(mode.startsWith("material_")){req.material_id=Number(get("material").value);const n=Number(get("amount").value);if(!Number.isSafeInteger(n)||n<0)throw Error("请输入有效非负整数数量");req.amount=n}if(mode==="card_grant"){req.card_id=Number(get("card").value);req.count=Number(get("count").value)}}
  if(mode==="recommendation_issue"){req.offer={recommend_id:Number(get("offer-id").value),commodity_id:Number(get("offer-commodity").value),price:Number(get("offer-price").value),show_duration:Number(get("offer-duration").value),expired_time:Math.floor(new Date(get("offer-expired").value).getTime()/1000)};if(!Number.isSafeInteger(req.offer.expired_time))throw Error("请填写有效的报价失效时间")}
  get("result").textContent=text(await request(path,req));
 }finally{button.disabled=false}
}));
function draw(){
 const i=Number(get("timeline").value),event=events[i];get("event-label").textContent=event?(i+1)+"/"+events.length+" · "+event.kind:"没有保留观察事件";get("event").textContent=event?text(event):"";
 const ctx=get("battle").getContext("2d");ctx.clearRect(0,0,950,420);let units=[];
 for(let n=0;n<=i;n++){const data=events[n]?.data;if(Array.isArray(data?.units))units=data.units}
 units.forEach((unit,index)=>{const pos=unit.hex||unit.coord||unit.position;const x=Array.isArray(pos)?100+(Number(pos[0])||0)*35:75+(index%10)*85;const y=Array.isArray(pos)?100+(Number(pos[1])||0)*35:80+Math.floor(index/10)*85;ctx.fillStyle="#175779";ctx.beginPath();ctx.arc(Math.max(25,Math.min(920,x)),Math.max(25,Math.min(390,y)),17,0,Math.PI*2);ctx.fill();ctx.fillStyle="#183041";ctx.fillText(String(unit.eid||index)+" · "+String(unit.role_id||unit.role||""),x-20,y+34)})
}
get("timeline").addEventListener("input",draw);get("play").addEventListener("click",()=>{if(timer){clearInterval(timer);timer=null;get("play").textContent="播放";return}get("play").textContent="暂停";timer=setInterval(()=>{const next=Number(get("timeline").value)+1;if(next>=events.length){clearInterval(timer);timer=null;get("play").textContent="播放";return}get("timeline").value=next;draw()},700)});draw();

# -*- coding: utf-8 -*-
"""去掉技术说明中的临时代理交付措辞，保留源码、证据与未完成边界。"""
from pathlib import Path
import sys
sys.stdout.reconfigure(encoding='utf-8')
root=Path(__file__).resolve().parents[2]
path=root/'SERVER.md'
source=path.read_text(encoding='utf-8')
changes={
 '由根保留可选未实现':'保留为可选未实现',
 'RunePool未决问题由根保留':'契印池政策仍未决',
 '外层固定标记由根包':'外层固定标记由组合生成器包裹',
 '根接口：':'Service内部接口：',
 '根已接':'已接',
 '根邮件领取':'邮件领取',
 '供体验代理商品/所属shop':'供商品/所属shop',
 '根gate':'gate',
 '根保存':'保存',
 '调用根 grantAvatarExp':'调用grantAvatarExp',
 '根 applyBattleCardExp':'applyBattleCardExp',
 '根普通副本':'普通副本',
 '体验代理商店':'商店',
 '根组合顺序':'组合顺序',
 '根接真实PG排名':'实现真实PG排名',
 '体验代理grantNativeItem':'grantNativeItem',
 '体验代理追加':'已追加',
 '并报告TestShopPASS':'TestShop专项已通过',
 '根collection':'collection',
 'RPC8已由根接':'RPC8已接',
 '根共享源码当时':'共享源码当时',
 'root追加':'源码追加',
 '活动代理援护':'活动助战',
}
for before,after in changes.items():source=source.replace(before,after)
path.write_text(source,encoding='utf-8',newline='\n')
print('已清除技术说明中的临时代理措辞')

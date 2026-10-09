"""活动规则施工的取证、备份与专项验证；不连接生产服务。"""
import hashlib, json, pathlib, shutil, subprocess, sys, time
root = pathlib.Path(__file__).resolve().parents[2]
if sys.argv[1] == 'backup':
    target = root / 'out/backups/activity-rules-20261008'
    target.mkdir(parents=True, exist_ok=True)
    rows = json.loads((target / 'manifest.json').read_text(encoding='utf-8')) if (target / 'manifest.json').exists() else []
    for name in sys.argv[2:]:
        source = root / name
        destination = target / (name.replace('/', '__') + '.bak')
        if destination.exists():
            raise RuntimeError('备份已存在，禁止覆盖：' + str(destination))
        shutil.copy2(source, destination)
        rows.append({'path': name, 'sha256': hashlib.sha256(source.read_bytes()).hexdigest()})
    (target / 'manifest.json').write_text(json.dumps(rows, ensure_ascii=False, indent=2), encoding='utf-8')
    print(json.dumps(rows, ensure_ascii=False))
elif sys.argv[1] == 'native':
    sys.path.insert(0, str(root / 'out/tools'))
    from mem_marshal_extract import Loader
    from neox_dis import disasm
    module, *names = sys.argv[2:]
    inventory = json.loads((root / 'out/npk_scripts/android_inventory.json').read_text(encoding='utf-8'))
    entry = next(x for x in inventory['modules'] if x['hash'] == module)
    raw = (root / 'out/npk_scripts' / entry['marshal_file']).read_bytes()
    print(entry['filename'], hashlib.sha256(raw).hexdigest())
    def visit(c):
        name = c['name'].decode('utf-8') if isinstance(c['name'], bytes) else c['name']
        if not names:
            print(name, c.get('varnames', []))
        elif any(x in name for x in names):
            disasm(c)
        for child in c['consts']:
            if isinstance(child, dict) and child.get('type') == 'code':
                visit(child)
    visit(Loader(raw).r_object())
elif sys.argv[1] in ('refs', 'refs-extra'):
    sys.path.insert(0, str(root / 'out/tools'))
    from mem_marshal_extract import Loader
    inventory = json.loads((root / 'out/npk_scripts/android_inventory.json').read_text(encoding='utf-8'))
    wanted = sys.argv[2:]
    report = []
    def decoded(v):
        if isinstance(v, bytes):
            try:
                return v.decode('utf-8')
            except UnicodeDecodeError:
                return '[原生二进制常量：' + hashlib.sha256(v).hexdigest() + ']'
        return str(v)
    for entry in inventory['modules']:
        raw = (root / 'out/npk_scripts' / entry['marshal_file']).read_bytes()
        def visit(c, path=''):
            name = path + '/' + decoded(c['name'])
            texts = [decoded(x) for x in c['names']] + [decoded(x) for x in c['consts'] if isinstance(x, (str,bytes))]
            hits = [x for x in texts if any(token in x for token in wanted)]
            if hits:
                report.append({'module': entry['hash'], 'source': entry['filename'], 'sha256': hashlib.sha256(raw).hexdigest(), 'function': name, 'hits': hits})
            for child in c['consts']:
                if isinstance(child, dict) and child.get('type') == 'code':
                    visit(child, name)
        visit(Loader(raw).r_object())
    destination = 'out/activity-rules-native-refs.json' if sys.argv[1] == 'refs' else 'out/activity-rules-remaining-refs.json'
    (root / destination).write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
    for row in report:
        print(row['module'], row['source'], row['function'], ','.join(row['hits']))
elif sys.argv[1] in ('test', 'testall', 'testpg'):
    mode = sys.argv[1]
    prefix = {'test': 'activity-rules-local-test', 'testall': 'activity-all-local-test', 'testpg': 'activity-pg-compile-test'}[mode]
    log = root / ('out/' + prefix + '.log')
    history = root / 'out/backups/activity-rules-20261008'
    history.mkdir(parents=True, exist_ok=True)
    if log.exists():
        shutil.copy2(log, history / ('test-' + str(time.time_ns()) + '.log'))
    package = './internal/game/dbstore' if mode == 'testpg' else './internal/game'
    pattern = 'TestPostgresActivity' if mode == 'testpg' else ('TestActivity' if mode == 'testall' else 'TestActivityRules')
    cmd = [__import__('os').environ.get('HS_GO', 'go'), 'test', package, '-run', pattern, '-count=1', '-v']
    started = time.time()
    result = subprocess.run(cmd, cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    log.write_bytes(result.stdout)
    (root / ('out/' + prefix + '.json')).write_text(json.dumps({'command': cmd, 'started_unix': started, 'ended_unix': time.time(), 'exit_code': result.returncode, 'log_sha256': hashlib.sha256(result.stdout).hexdigest()}, ensure_ascii=False, indent=2), encoding='utf-8')
    sys.stdout.buffer.write(result.stdout)
    sys.exit(result.returncode)
elif sys.argv[1] == 'mail':
    activities = json.loads((root / 'internal/game/activity_catalog.json').read_text(encoding='utf-8'))['表']
    social = json.loads((root / 'internal/game/social_catalog.json').read_text(encoding='utf-8'))['tables']
    report = []
    for table in ['monster_nian_daily_rank', 'mountain_game_rank', 'monster_nian_rank']:
        print(table)
        value = activities.get(table, {})
        entries = value.get('键值条目', [])
        print(entries)
        for entry in entries:
            bid = entry['值']['bonus_id']
            bonus = activities['bonus'][str(bid)]
            mid = bonus['mail_template_id']
            print(bid, mid, social['mail_template'].get(str(mid)), bonus['fixed_items'])
            report.append({'table': table, 'range': entry['值']['rank_range'], 'bonus_id': bid, 'mail_template_id': mid, 'template': social['mail_template'].get(str(mid)), 'fixed_items': bonus['fixed_items']})
    sources = {name: hashlib.sha256((root / name).read_bytes()).hexdigest() for name in ['internal/game/activity_catalog.json', 'internal/game/social_catalog.json']}
    (root / 'out/activity-rules-native-mail.json').write_text(json.dumps({'sources': sources, 'rows': report}, ensure_ascii=False, indent=2), encoding='utf-8')
elif sys.argv[1] == 'manifest':
    target = root / 'out/backups/activity-rules-20261008'
    rows = [{'path': path.name[:-4].replace('__', '/'), 'sha256': hashlib.sha256(path.read_bytes()).hexdigest()} for path in sorted(target.glob('*.bak'))]
    (target / 'manifest.json').write_text(json.dumps(rows, ensure_ascii=False, indent=2), encoding='utf-8')
    print(json.dumps(rows, ensure_ascii=False))
elif sys.argv[1] == 'final':
    files = ['internal/game/activity_business.go', 'internal/game/activity_metrics.go', 'internal/game/mountain_nian_business.go', 'internal/game/activity_rank.go', 'internal/game/activity_calendar.go', 'internal/game/mountain_guard.go', 'internal/game/daily_business.go', 'internal/game/house_frage_business.go', 'internal/game/activity_rules_test.go', 'internal/game/activity_calendar_test.go', 'internal/game/house_frage_business_test.go', 'internal/game/dbstore/activity_rank.go', 'internal/game/dbstore/activity_rank_test.go', 'internal/game/dbstore/activity_calendar_test.go']
    modules = [('98236F87', ['query_rank_list', 'on_query_rank_list', 'cal_pvp_rank_info']), ('CFFEDD3B', ['query_mountain_sea_own_rank']), ('EFCB6F9A', ['get_process_per', 'get_card_effect_value']), ('E3E5EBF7', ['on_update_attr', 'on_update_skill']), ('2CC3F05D', ['setup_change_attr_data']), ('A091551F', ['create_avatar']), ('6D5412CA', ['fix_magic_field_skill_level']), ('2BCB4D1C', ['get_node_buff_list', 'get_correction_level']), ('991031A8', ['check_unlock_all_in']), ('F1606E31', ['update', 'on_update']), ('6822602C', ['to_string'])]
    evidence = bytearray()
    for module, names in modules:
        result = subprocess.run([sys.executable, '-X', 'utf8', str(pathlib.Path(__file__)), 'native', module, *names], cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        evidence.extend(('\n取证命令：' + module + ' ' + ','.join(names) + '\n').encode('utf-8'))
        evidence.extend(result.stdout)
        if result.returncode:
            raise RuntimeError('原生取证命令失败：' + module)
    native_log = root / 'out/activity-rules-native-disasm.log'
    native_log.write_bytes(evidence)
    report = {
        '时间': time.time(), '源码冻结': True, '没有部署': True, '没有修改MD': True,
        '文件': [{'路径': name, 'SHA256': hashlib.sha256((root / name).read_bytes()).hexdigest()} for name in files],
        '原文件备份': json.loads((root / 'out/backups/activity-rules-20261008/manifest.json').read_text(encoding='utf-8')),
        '专项': json.loads((root / 'out/activity-all-local-test.json').read_text(encoding='utf-8')),
        '真实PG': {'执行者': '根统一连接隔离真实PostgreSQL', '本地只编译且全部SKIP': True, '用例': ['TestPostgresActivityRankingFullCountStableAndReload', 'TestPostgresActivityCalendarAtomicDailyMountainReloadAndRetry'], '编译日志': 'out/activity-pg-compile-test.log'},
        '已落': [
            '年兽UTC+8每日3次进度/奖励位复位；跨零点成绩只更新个人最大/总伤害，不进有效排行；后续较低但同日有效成绩可以独立更新排行。',
            '山海难度降序/AP升序与年兽有效伤害降序；同分OID稳定序；真实SQL COUNT全部同服参榜者，显示页最多1000；COUNT与页面同一repeatable-read快照。',
            'Fixture1105同服玩家测试个人1105名不因显示2页/1000条截断；真实PG用例1107角色包括1105参榜、不同服和未参榜。',
            '山海守护属性base+process*充能/1000及结界[ID,1+int(process*充能/1000),base]按原生公式冻结；通过change_attr_data.buff/mf_skill交原生引擎消费，客户端自报change_attr_data清除。',
            '年兽日榜23:59:59及山海每挑战七日窗口22点：全服原始排行快照、排名收据与原生固定附件邮件同一UpdateAllSocial事务；邮箱500失败全服回滚，重试防重。',
            '首次晚初见不补历史奖、零名次不发奖、漏周/漏日或截止后best覆盖历史时标缺历史；周截止独立于当天截止；年兽只冻结三分榜，未知总公式不结周奖。',
            '关联shop活动按ID排序；共享配置的开放资格取关联活动并集。实际145条本版activity_type无共享shop_ids，共享用例是合成回归，不称原版共享事实。',
            '残页梦境401/402真实扣骰子1后同事务累计无参1008一次，普通双骰模式仍一次；不足骰子失败全事务回滚；209001/2/3原生1/30/666目标。'
        ],
        '原生证据': {'全量引用扫描': 'out/activity-rules-native-refs.json', '模块数': 3099, '反汇编': str(native_log.relative_to(root)).replace('\\', '/'), '反汇编SHA256': hashlib.sha256(evidence).hexdigest(), '七条实际邮件': 'out/activity-rules-native-mail.json', '说明': '仅本版Android原生marshal与datas。没有将PC参考当本版事实。'},
        '最小剩余政策': [
            {'项目': '年兽总榜与周奖', '已知': '三个原生DID分榜名次、周三23:59:59截止清榜、每小时刷新显示说明。', '缺': '三名次综合公式、缺一榜惩罚、总榜同分决胜与刷新口径；不能用MAX伤害假造总榜。'},
            {'项目': 'Cthulhu critical2/12与all-in', '已知': 'GUI大成功SAN+20、大失败SAN-10；普通失败可满足解锁后一次all-in重掷花SAN20；2d6与属性修正。', '缺': '原服big_enable/forced2/12优先级与判定结果枚举、all-in后dice/抵扣/状态账本确切规则。'},
            {'项目': '初音惊喜与手册buff', '已知': '本版receive_miku_surprise_bonus代理与handbook_item.battle_addition_effects配置。', '缺': 'surprise触发、奖励ID、限定范围/回执状态与手册battle_addition_effects如何装配进battle参数；全3099扫描未发现服务端消费实现。'},
            {'项目': '汪言属性', '已知': '未占领影响节点buff_added；当前combat_power/关卡战力条件选择correction_level。', '缺': 'correction应用于哪方及extra字段/结算发动端；buff user_property与确切camp装配。'},
            {'项目': '山海守护掉落加成与常驻重赛', '已知': 'GUI材料加成比例base+process*充能/1000，七日rank_record_day及22点。', '缺': '资产整数舍入/多槽叠加实际发奖公式；常驻是否循环赛季与历史名次迁移。当前不猜奖励加成，不自动重复新赛季。'},
            {'项目': '夏活与鱼长', '缺': '封弊者60%加成真实buff/过滤范围装配；鱼长度分布/取整及原服生成机制。'},
            {'项目': '宿舍SSR与动画候选', '缺': '累计SSR保底/动态概率公式，以及动画真假候选组成/顺序/去重机制；不能仅凭静态参数补猜。'}
        ],
        '验收边界': '16项本地Activity专项PASS不等于真实PG或Android验收。真实PG需根统一执行，Android活动战斗/结奖UI尚需联调。'
    }
    path = root / 'out/activity-rules-final-report.json'
    path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
    print(str(path), hashlib.sha256(path.read_bytes()).hexdigest())
elif sys.argv[1] == 'remaining':
    modules = [('FF2A17AE', ['get_real_ssr_weight', 'random_site', 'get_bonus_materials', 'get_random_materials']), ('514DD3DE', ['get_ssr_add_prob', 'get_facility_mood_profit', 'get_dormitory_cards_num_profit']), ('4BE7D89A', ['<lambda>']), ('7036F614', ['add_fish']), ('90B7C06A', ['start_fishing']), ('5DA4EB4F', ['get_fish_info']), ('B0D07550', ['receive_miku_surprise_bonus']), ('F0E40550', ['get_random_reward_card_info', 'player_get_house_random_reward']), ('F0BFB88A', ['update_daily_random_reward']), ('F1606E31', ['update_check_result_second'])]
    evidence = bytearray()
    for module, names in modules:
        result = subprocess.run([sys.executable, '-X', 'utf8', str(pathlib.Path(__file__)), 'native', module, *names], cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        if result.returncode:
            raise RuntimeError('原生补证失败：' + module)
        evidence.extend(('\n原生补证：' + module + ' ' + ','.join(names) + '\n').encode('utf-8'))
        evidence.extend(result.stdout)
    log = root / 'out/activity-rules-remaining-native.log'
    log.write_bytes(evidence)
    tables = {}
    for name in ['facility_base', 'dormitory_attrs', 'house_daily_random_reward', 'house_daily_random_reward_rule', 'system_unlock', 'house_frage_params', 'card_house_waypoint_weight', 'summer_fish', 'summer_general_args', 'handbook_item']:
        path = root / ('out/client_catalogs/tables/' + name + '.json')
        source = json.loads(path.read_text(encoding='utf-8'))
        table = source['数据']
        if name == 'facility_base':
            values = {'6': table['6']}
        elif name == 'dormitory_attrs':
            values = {key: {'cards_num_profit': value['cards_num_profit'], 'room_system_name': value['room_system_name']} for key, value in table.items()}
        elif name == 'system_unlock':
            values = [x for x in table['键值条目'] if x['键'][0] in ['house_dormitory3', 'house_daily_random_reward', 'house_daily_reward']]
        elif name == 'house_daily_random_reward_rule':
            values = {'配置条目数': len(table), '4401': table['4401'], '4409': table['4409']}
        elif name == 'handbook_item':
            values = {'配置条目数': len(table), '含战斗效果条目': {key: value for key, value in table.items() if value.get('battle_addition_effects')}}
        elif name == 'summer_fish':
            values = {'配置条目数': len(table), '首两项': dict(list(table.items())[:2])}
        else:
            values = table
        tables[name] = {'路径': str(path.relative_to(root)).replace('\\', '/'), 'SHA256': hashlib.sha256(path.read_bytes()).hexdigest(), '原生模块': source.get('模块哈希'), '原生SHA256': source.get('来源SHA256'), '配置': values}
    report = {
        '冻结源码后只读取证': True, '反汇编': str(log.relative_to(root)).replace('\\', '/'), '反汇编SHA256': hashlib.sha256(evidence).hexdigest(), '配置': tables,
        '新确定点': [
            '量子骰子facility_base6原生lambda=float(get_dormitory_cards_num_profit)；get_ssr_add_prob调用该lambda，取设施实际房间入住N的cards_num_profit[N-1]，空入住/超表长度为0。get_bonus_materials的ssr_add_prob只作用SSR残页概率。',
            '居民礼物原生直传player_get_house_random_reward(rid,card_id)，回on_player_get_house_random_reward(ret,bonus_box)。house_daily_random_reward[rid][card_id][0]状态必须CARD_CAN_GET_REWARD，幻书实际入住目标房间并在rid每日时段；领取回包触发special_guide1。',
            '居民礼物解锁[[[1,613]]]；固定收藏室日奖解锁[[[1,604]]]；房3解锁[[[1,104]]]。原生两个礼物时段06:00..13:59:59、16:00..23:59:59，各random_num1；不能混作同一系统。',
            '鱼图鉴add_fish已证实数量+1与max旧长/新长。start_fishing仅代理回fish_id/fish_length，没有原服长度分布代码；get_fish_length在GUI是结果成员，不是生成函数。',
            '残页get_real_ssr_weight已证按per_count阈值选factor；但per_count输入生成未知，现Go直接w.Total是不能证明的原型策略；SSR累计与设施入住加成必须区分。'
        ],
        '仍缺': ['per_count从Total/SSR等统计派生及重置口径', '居民当日随机选择哪张幻书/何时冻结及reward_info生成口径', '初音surprise触发与奖励ID、手册battle_addition_effects装配端', '夏活封弊者加成buff/过滤与装配', '鱼长生成分布', '残页动画假候选生成和顺序机制']
    }
    path = root / 'out/activity-rules-remaining-evidence.json'
    path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
    print(str(path), hashlib.sha256(path.read_bytes()).hexdigest())

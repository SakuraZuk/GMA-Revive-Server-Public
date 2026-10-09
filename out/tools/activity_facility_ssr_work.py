"""本版设施入住典藏残页加成的备份、专项与施工报告。"""
import hashlib, json, pathlib, shutil, subprocess, sys, time

root = pathlib.Path(__file__).resolve().parents[2]
target = root / 'out/backups/activity-facility-ssr-20261008'
if sys.argv[1] == 'backup':
    target.mkdir(parents=True, exist_ok=True)
    rows = []
    for name in sys.argv[2:]:
        source = root / name
        dest = target / (name.replace('/', '__') + '.bak')
        if dest.exists():
            raise RuntimeError('已有备份禁止覆盖：' + str(dest))
        shutil.copy2(source, dest)
        rows.append({'路径': name, 'SHA256': hashlib.sha256(source.read_bytes()).hexdigest()})
    (target / 'manifest.json').write_text(json.dumps(rows, ensure_ascii=False, indent=2), encoding='utf-8')
    print(json.dumps(rows, ensure_ascii=False))
elif sys.argv[1] == 'test':
    log = root / 'out/activity-facility-ssr-test.log'
    if log.exists():
        shutil.copy2(log, target / ('test-' + str(time.time_ns()) + '.log'))
    cmd = [r__import__('os').environ.get('HS_GO', 'go'), 'test', './internal/game', '-run', 'TestActivityHouse', '-count=1', '-v']
    start = time.time()
    result = subprocess.run(cmd, cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    log.write_bytes(result.stdout)
    report = {'命令': cmd, '开始时间': start, '结束时间': time.time(), '退出码': result.returncode, '日志SHA256': hashlib.sha256(result.stdout).hexdigest()}
    (root / 'out/activity-facility-ssr-test.json').write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
    sys.stdout.buffer.write(result.stdout)
    sys.exit(result.returncode)
elif sys.argv[1] == 'final':
    files = ['internal/game/house_frage_business.go', 'internal/game/house_frage_facility.go', 'internal/game/house_frage_facility_test.go', 'internal/game/dbstore/activity_house_facility_test.go']
    report = {
        '源码已冻结': True, '文件': [{'路径': name, 'SHA256': hashlib.sha256((root / name).read_bytes()).hexdigest()} for name in files],
        '改前备份': json.loads((target / 'manifest.json').read_text(encoding='utf-8')),
        '专项': json.loads((root / 'out/activity-facility-ssr-test.json').read_text(encoding='utf-8')),
        '原生依据': 'out/activity-rules-remaining-evidence.json 和 out/activity-rules-remaining-native.log：514DD3DE/4BE7D89A/FF2A17AE链。',
        '已接': '非fixed卡格的典藏残页概率=cw/(cw+ow)*(1+Up/100+设施6实际房间入住bonus)；N=该房dormitory_card_mgr对应Slots长度，N<=表长取cards_num_profit[N-1]，空入住/超表长为0。其他稀有度和固定祈愿不叠加该概率。',
        '边界': '没有修改累计per_count=w.Total的现有原型策略，没有改棋盘稀有度权重、骰子数量/总榜公式或居民礼物。',
        '验证': '固定在原生基础概率与入住后概率之间的随机边界，真实ctrl_move产生典藏残页，空入住产生原生代币；实际资源不足失败回滚；概率UP为加法合并，固定祈愿仍必得；JSON存档重载后加成继续由真实入住读取。',
        '真实PG': {'用例': 'TestPostgresActivityHouseFacilitySSRActualRewardReloadAndRollback', '本地只编译且SKIP': True, '日志': 'out/activity-pg-compile-test.log', '执行': '由根统一连接真实PostgreSQL验收'}
    }
    path = root / 'out/activity-facility-ssr-report.json'
    path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
    print(str(path), hashlib.sha256(path.read_bytes()).hexdigest())

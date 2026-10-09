"""活动真实PG协议夹具修复的改前备份与编译记录。"""
import hashlib, json, pathlib, shutil, subprocess, sys, time

root = pathlib.Path(__file__).resolve().parents[2]
target = root / 'out/backups/activity-pg-fixture-20261008'
files = ['internal/game/dbstore/activity_calendar_test.go', 'internal/game/dbstore/activity_house_facility_test.go']
if sys.argv[1] == 'backup':
    target.mkdir(parents=True, exist_ok=True)
    rows = []
    for name in files:
        source = root / name
        dest = target / (name.replace('/', '__') + '.bak')
        if dest.exists():
            raise RuntimeError('改前备份禁止覆盖：' + str(dest))
        shutil.copy2(source, dest)
        rows.append({'路径': name, 'SHA256': hashlib.sha256(source.read_bytes()).hexdigest()})
    (target / 'manifest.json').write_text(json.dumps(rows, ensure_ascii=False, indent=2), encoding='utf-8')
    print(json.dumps(rows, ensure_ascii=False))
elif sys.argv[1] == 'test':
    cmd = [r__import__('os').environ.get('HS_GO', 'go'), 'test', './internal/game/dbstore', '-run', 'TestPostgresActivity', '-count=1', '-v']
    log = root / 'out/activity-pg-fixture-compile.log'
    if log.exists():
        shutil.copy2(log, target / ('test-' + str(time.time_ns()) + '.log'))
    start = time.time()
    result = subprocess.run(cmd, cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    log.write_bytes(result.stdout)
    report = {'命令': cmd, '开始时间': start, '结束时间': time.time(), '退出码': result.returncode, '日志SHA256': hashlib.sha256(result.stdout).hexdigest()}
    (root / 'out/activity-pg-fixture-compile.json').write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
    sys.stdout.buffer.write(result.stdout)
    sys.exit(result.returncode)
elif sys.argv[1] == 'final':
    changed = files + ['internal/game/dbstore/activity_pg_helpers_test.go']
    report = {
        '改前备份': json.loads((target / 'manifest.json').read_text(encoding='utf-8')),
        '仅修改PG测试': True, '文件': [{'路径': name, 'SHA256': hashlib.sha256((root / name).read_bytes()).hexdigest()} for name in changed],
        '原因': '真实PG首轮活动calendar/SSR构造game.New(store,nil)，真实quick_login必查询Hotfix，nil Catalog.Query造成panic；不是生产协议问题。',
        '修复': '活动专用PG夹具加载实际deploy/data/hotfix.json，验证Connected→quick_login成功与十二字节角色→Authenticated→BecomePlayer→Playing→set_reconnect_auth_msg→on_refresh_login。重登录始终新Connection；不放宽生产RPC。',
        'Ranking复核': 'activity_rank_test.go直接使用Store SQL，没有构造Service或登录，不存在nil Hotfix目录。原SQL COUNT/同分/回滚断言保留。',
        '资产断言': '日奖/七日奖附件与收据、邮箱500失败全服回滚、重建Store防重；设施入住SSR/空入住边界与扣骰子回滚均原样保留。',
        '编译': json.loads((root / 'out/activity-pg-fixture-compile.json').read_text(encoding='utf-8')),
        '真实PG': '根负责重构建后连接真实PG执行；本地无HS_TEST_DATABASE_URL则全部SKIP，不称真实PG通过。'
    }
    path = root / 'out/activity-pg-fixture-report.json'
    path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
    print(str(path), hashlib.sha256(path.read_bytes()).hexdigest())

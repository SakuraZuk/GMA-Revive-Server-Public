"""在独立回环端口启动管理页预览；使用开发夹具与一次性本地令牌。"""
import json,os,secrets,socket,subprocess,sys
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
sys.stdout.reconfigure(encoding='utf-8')
for port in (19900,19901):
    with socket.socket() as probe:probe.bind(('127.0.0.1',port))
binary=ROOT/'out/bin/admin-preview.exe'
result=subprocess.run([r__import__('os').environ.get('HS_GO', 'go'),'build','-o',str(binary),'./cmd/gameserver'],cwd=ROOT,capture_output=True,text=True,encoding='utf-8')
if result.returncode:print(result.stderr);raise SystemExit(result.returncode)
env=os.environ.copy();token=secrets.token_urlsafe(36)
env.update(HS_GAME_BIND='127.0.0.1:19900',HS_GAME_ADMIN_BIND='127.0.0.1:19901',HS_GAME_ADMIN_TOKEN=token,HS_GAME_DEBUG_BIND='',HS_DATABASE_URL='',HS_GAME_RSA_KEY='',HS_DATA_DIR=str(ROOT/'deploy/data'))
(ROOT/'out/admin-preview-state.json').write_text(json.dumps({'token':token,'url':'http://127.0.0.1:19901/admin','说明':'仅本机开发夹具，一次性预览令牌，不是生产令牌'},ensure_ascii=False),encoding='utf-8')
with (ROOT/'out/admin-preview.log').open('wb') as log:
    process=subprocess.Popen([str(binary)],cwd=ROOT,env=env,stdout=log,stderr=subprocess.STDOUT,creationflags=subprocess.CREATE_NO_WINDOW)
    print('中文管理页预览：http://127.0.0.1:19901/admin',flush=True)
    try:process.wait()
    finally:
        if process.poll() is None:process.terminate();process.wait(timeout=10)

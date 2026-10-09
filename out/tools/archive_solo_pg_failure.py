"""保存本批真实PG失败的原始报告和日志。"""
from pathlib import Path
import datetime,shutil
root=Path(__file__).resolve().parents[2]
backup=root/'out/backups'/('solo-pg-failure-'+datetime.datetime.now().strftime('%Y%m%d-%H%M%S'))
backup.mkdir(parents=True)
for name in ('remote-database-verification.json','database-test.log','completion-build.json'):
 shutil.copy2(root/'out'/name,backup/name)

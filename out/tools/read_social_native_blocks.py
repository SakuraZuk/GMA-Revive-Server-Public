"""按完整方法名选取原生反汇编代码块，不执行客户端模块。"""
import sys
from pathlib import Path
sys.stdout.reconfigure(encoding="utf-8")
path=Path(sys.argv[1])
names=set(sys.argv[2:])
printing=False
for line in path.read_text(encoding="utf-8").splitlines():
    if line.startswith("code "):
        name=line.split("  (",1)[0][5:]
        printing=name in names or name.rsplit("/",1)[-1] in names
    if printing:
        print(line)

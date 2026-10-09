import socket, struct

def ask(name, server='127.0.0.1'):
    parts = name.split('.')
    q = b'\xab\xcd\x01\x00\x00\x01\x00\x00\x00\x00\x00\x00'
    for p in parts:
        q += bytes([len(p)]) + p.encode()
    q += b'\x00\x00\x01\x00\x01'
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.settimeout(5)
    s.sendto(q, (server, 53))
    r, _ = s.recvfrom(512)
    an = struct.unpack('>H', r[6:8])[0]
    return 'AN=%d %s' % (an, '.'.join(str(b) for b in r[-4:]))

for n in ('h62.update.netease.com', 'analytics.mpay.netease.com', 'game.r18sex.net'):
    try:
        print(n, ask(n))
    except Exception as e:
        print(n, 'FAIL', e)

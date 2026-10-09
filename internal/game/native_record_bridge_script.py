# -*- coding: utf-8 -*-
# 本版原生battle_record四元组完整文件采集；保存原cPickle默认protocol0，zlib原样上传。
def _install_revival_native_record_bridge():
    import os
    import json
    import zlib
    import base64
    import hashlib
    import time
    import cPickle
    import game3d
    import gworld
    import version
    from battle_logic import client_battle
    shadow = client_battle.shadow_battle
    if getattr(shadow, '_revival_bridge_version', 0) not in (12, 13, 14, 15):
        raise RuntimeError('原生录像采集需要兼容战斗桥版本12至15')
    if getattr(shadow, '_revival_native_record_bridge', 0) == 1:
        return
    def function(method):
        return getattr(method, 'im_func', method)
    old_notify = function(shadow.notify_battle_finish)
    root = os.path.join(game3d.get_doc_dir(), 'hs_native_records')
    if not os.path.isdir(root):
        os.makedirs(root)
    busy = [False]
    serial = [0]
    def write_meta(filename, meta):
        temporary = filename + '.tmp'
        handle = open(temporary, 'wb')
        try:
            handle.write(json.dumps(meta, ensure_ascii=True))
            handle.flush()
        finally:
            handle.close()
        os.rename(temporary, filename)
    def upload_next():
        if busy[0]:
            return
        try:
            player = gworld.get_player()
            if player is None:
                return
            owner = str(player.eid)
            for name in sorted(os.listdir(root)):
                if not name.endswith('.meta'):
                    continue
                meta_path = os.path.join(root, name)
                handle = open(meta_path, 'rb')
                try:
                    meta = json.loads(handle.read())
                finally:
                    handle.close()
                if meta['owner'] != owner:
                    continue
                if meta.get('retry_after', 0) > time.time():
                    continue
                handle = open(os.path.join(root, meta['battle_uuid'] + '.record'), 'rb')
                try:
                    data = handle.read()
                finally:
                    handle.close()
                if len(data) != meta['size'] or hashlib.sha256(data).hexdigest() != meta['sha256']:
                    raise ValueError('原生录像暂存文件哈希不一致')
                index = meta.get('next', 0)
                total = (len(data) + 49151) // 49152
                if index >= total:
                    os.rename(meta_path, meta_path + '.done')
                    continue
                chunk = {'battle_uuid': meta['battle_uuid'], 'version': meta['version'],
                         'sha256': meta['sha256'], 'size': len(data), 'count': meta['count'],
                         'index': index, 'total': total,
                         'data': base64.b64encode(data[index * 49152:(index + 1) * 49152])}
                serial[0] += 1
                request = serial[0]
                busy[0] = request
                def timeout(request=request):
                    if busy[0] == request:
                        busy[0] = False
                game3d.delay_exec(10000, timeout)
                def acknowledged(ok, reason, meta=meta, meta_path=meta_path, request=request):
                    if busy[0] != request:
                        return
                    busy[0] = False
                    if ok:
                        meta['next'] = meta.get('next', 0) + 1
                        meta.pop('last_error', None)
                        meta.pop('retry_after', None)
                        write_meta(meta_path, meta)
                    else:
                        meta['retry_after'] = time.time() + 60
                        meta['last_error'] = str(reason)[:500]
                        write_meta(meta_path, meta)
                        print('HS_NATIVE_RECORD_UPLOAD_REJECT %s' % reason)
                player.call_server('upload_native_battle_record', chunk, callback=acknowledged)
                return
        except Exception as error:
            busy[0] = False
            print('HS_NATIVE_RECORD_UPLOAD_WAIT %s' % error)
    def loop():
        upload_next()
        game3d.delay_exec(1500, loop)
    def notify(self):
        result = old_notify(self)
        if self.winner_eid_list is None or getattr(self, '_revival_native_record_saved_uuid', None) == str(self.id):
            return result
        # 4/16只用于异步竞技；同步真人的恢复仍使用共同命令日志。
        if not getattr(self, 'extra_info', {}).get('asyn_pvp_eid'):
            return result
        try:
            recorder = self.battle_record
            records = recorder.records
            names = set(row[1] for row in records)
            if not set(('set_last_fighting_cards', 'prepare', 'add_fighting_cards', 'battle_end_notice')).issubset(names):
                raise ValueError('原生录像未采集完整战斗')
            if len(records) > 100000:
                raise ValueError('完整原生录像超过条目保护上限')
            raw = cPickle.dumps(records)
            if len(raw) > 67108864:
                raise ValueError('完整原生录像超过展开大小保护上限')
            data = zlib.compress(raw)
            if len(data) > 786432:
                raise ValueError('完整原生录像超过传输大小保护上限')
            uuid = str(self.id)
            handle = open(os.path.join(root, uuid + '.record'), 'wb')
            try:
                handle.write(data)
                handle.flush()
            finally:
                handle.close()
            meta = {'owner': str(gworld.get_player().eid), 'battle_uuid': uuid,
                    'version': str(version.VERSION), 'sha256': hashlib.sha256(data).hexdigest(),
                    'size': len(data), 'count': len(records), 'next': 0}
            write_meta(os.path.join(root, uuid + '.meta'), meta)
            self._revival_native_record_saved_uuid = uuid
            print('HS_NATIVE_RECORD_STAGED %s %s' % (uuid, len(records)))
        except Exception as error:
            print('HS_NATIVE_RECORD_CAPTURE_ERROR %s' % error)
        return result
    shadow.notify_battle_finish = notify
    shadow._revival_native_record_bridge = 1
    loop()
def _revival_start_native_record_bridge():
    try:
        _install_revival_native_record_bridge()
        print('HS_NATIVE_RECORD_BRIDGE_READY 1')
    except Exception as error:
        print('HS_NATIVE_RECORD_BRIDGE_WAIT %s' % error)
        try:
            import game3d
            game3d.delay_exec(1500, _revival_start_native_record_bridge)
        except Exception as retry_error:
            print('HS_NATIVE_RECORD_BRIDGE_RETRY_ERROR %s' % retry_error)
_revival_start_native_record_bridge()

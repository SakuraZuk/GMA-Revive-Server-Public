-- 幻书账号角色持久化定义，按客户端已证的登录界面契约保存字段。
CREATE TABLE IF NOT EXISTS accounts (
    account        TEXT PRIMARY KEY,            -- 客户端 quick_login 账号（设备号）或 SDK 账号
    password_hash  TEXT NOT NULL,               -- bcrypt 哈希，禁止写入明文
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at  TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS avatars (
    avatar_oid     BYTEA PRIMARY KEY CHECK (octet_length(avatar_oid) = 12), -- 客户端实体标识
    account        TEXT NOT NULL REFERENCES accounts(account),
    hostnum        INTEGER NOT NULL CHECK (hostnum > 0), -- 服务器编号
    nickname       TEXT NOT NULL,
    level          INTEGER NOT NULL DEFAULT 1,
    head_id        INTEGER NOT NULL DEFAULT 1,
    head_box_id    INTEGER NOT NULL DEFAULT 1,
    custom_head_image_url TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_avatars_account ON avatars(account);
-- 最小登录业务每账号每服一个角色，并发建角由此约束兜底。
CREATE UNIQUE INDEX IF NOT EXISTS idx_avatars_account_hostnum ON avatars(account, hostnum);

-- 现有角色自动分配稳定 UID，登录时不再全员使用 1。
ALTER TABLE avatars ADD COLUMN IF NOT EXISTS uid BIGSERIAL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_avatars_uid ON avatars(uid);
ALTER TABLE avatars ADD COLUMN IF NOT EXISTS gender INTEGER NOT NULL DEFAULT 1 CHECK(gender IN (1,2));
ALTER TABLE avatars ADD COLUMN IF NOT EXISTS nickname_set BOOLEAN NOT NULL DEFAULT false;
CREATE UNIQUE INDEX IF NOT EXISTS idx_avatars_chosen_nickname ON avatars(hostnum,nickname) WHERE nickname_set;

CREATE TABLE IF NOT EXISTS avatar_progress (
    avatar_oid BYTEA PRIMARY KEY REFERENCES avatars(avatar_oid) ON DELETE CASCADE,
    state JSONB NOT NULL CHECK (jsonb_typeof(state)='object'),
    revision BIGINT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 客户端收尾凭证摘要绑定设备；游戏进程重启后仍可恢复同一角色。
CREATE TABLE IF NOT EXISTS avatar_reconnect (
    avatar_oid BYTEA PRIMARY KEY REFERENCES avatars(avatar_oid) ON DELETE CASCADE,
    device_id TEXT NOT NULL,
    token_hash BYTEA NOT NULL CHECK (octet_length(token_hash)=32),
    expires_at TIMESTAMPTZ NOT NULL
);

-- 评论由独立表保存；JSONB玩家行锁与评论记录在同事务更新。
CREATE TABLE IF NOT EXISTS card_comments (
 comment_oid BYTEA PRIMARY KEY CHECK (octet_length(comment_oid)=12),
 avatar_oid BYTEA NOT NULL REFERENCES avatars(avatar_oid) ON DELETE CASCADE,
 card_id INTEGER NOT NULL CHECK(card_id>0),
 created_at BIGINT NOT NULL,
 like_count INTEGER NOT NULL DEFAULT 0 CHECK(like_count>=0),
 deleted BOOLEAN NOT NULL DEFAULT false,
 state JSONB NOT NULL
);
CREATE INDEX IF NOT EXISTS card_comments_time_idx ON card_comments(card_id,created_at DESC,comment_oid) WHERE deleted=false;
CREATE INDEX IF NOT EXISTS card_comments_likes_idx ON card_comments(card_id,like_count DESC,created_at DESC,comment_oid) WHERE deleted=false;

-- 游戏进程在线租约，连接各自持有token；崩溃后按租约失效，不伪造永久在线。
CREATE TABLE IF NOT EXISTS hs_game_presence (
 session_id TEXT PRIMARY KEY CHECK(length(session_id)=24),
 avatar_oid BYTEA NOT NULL REFERENCES avatars(avatar_oid) ON DELETE CASCADE,
 expires_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS hs_game_presence_online_idx ON hs_game_presence(expires_at,avatar_oid);

-- 完整原生录像分块及完成文件；未完成分块不得用于播放，SHA和版本不可覆盖。
CREATE TABLE IF NOT EXISTS native_battle_records (
 battle_uuid TEXT PRIMARY KEY CHECK(length(battle_uuid)=24),
 avatar_oid BYTEA NOT NULL REFERENCES avatars(avatar_oid) ON DELETE CASCADE,
 state JSONB NOT NULL CHECK(jsonb_typeof(state)='object'),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS native_battle_records_owner_idx ON native_battle_records(avatar_oid,updated_at DESC);

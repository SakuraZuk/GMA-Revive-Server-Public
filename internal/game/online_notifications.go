package game

import "encoding/hex"

// 注册表只保存在线连接；传输断开时必须Detach，避免保留离线会话。
func (s *Service) attachPlayer(c *Connection) {
	key := hex.EncodeToString(selectedOID(c))
	s.onlineMu.Lock()
	defer s.onlineMu.Unlock()
	if s.online == nil {
		s.online = map[string]map[*Connection]bool{}
	}
	if s.online[key] == nil {
		s.online[key] = map[*Connection]bool{}
	}
	s.online[key][c] = true
	c.onlineOID = key
	c.lastDailyCheckDay = socialDay(s.Now())
	// Handle已持连接锁，通知好友延迟至它的解锁defer，避免同时登入互锁。
	c.pendingSocialOIDs = append(c.pendingSocialOIDs, socialFriendOIDs(c.SelectedAvatarUnsafe().Progress.Social)...)
}

func (s *Service) Detach(c *Connection) {
	c.mu.Lock()
	friends := socialFriendOIDs(c.SelectedAvatarUnsafe().Progress.Social)
	s.onlineMu.Lock()
	delete(s.online[c.onlineOID], c)
	if len(s.online[c.onlineOID]) == 0 {
		delete(s.online, c.onlineOID)
	}
	c.onlineOID = ""
	c.pendingMailbox = nil
	c.pendingMailboxNotify = false
	c.pendingPlayerReload = false
	c.phase = Closed
	s.onlineMu.Unlock()
	c.mu.Unlock()
	s.removeHumanPresence(c)
	// detach必须先释放自身连接锁，再令好友连接等待刷新；其他同角色连接仍计在线。
	for _, oid := range friends {
		s.publishPlayerRefresh(oid)
	}
}

// 只合并邮箱字段。版本较旧的通知不能覆盖随后成功领取的状态。
func (s *Service) publishMailbox(oid []byte, p Progress) { s.publishMailboxChange(oid, p, true) }

func (s *Service) publishMailboxChange(oid []byte, p Progress, notify bool, skip ...*Connection) {
	s.onlineMu.Lock()
	connections := []*Connection{}
	for c := range s.online[hex.EncodeToString(oid)] {
		connections = append(connections, c)
	}
	s.onlineMu.Unlock()
	for _, c := range connections {
		if len(skip) > 0 && c == skip[0] {
			continue
		}
		c.mu.Lock()
		if c.phase == Playing && p.MailRevision >= c.SelectedAvatarUnsafe().Progress.MailRevision && (c.pendingMailbox == nil || p.MailRevision >= c.pendingMailbox.MailRevision) {
			snapshot := CloneProgress(Progress{ShortMailInfo: p.ShortMailInfo, MailRevision: p.MailRevision})
			c.pendingMailbox = &snapshot
			c.pendingMailboxNotify = c.pendingMailboxNotify || notify
		}
		c.mu.Unlock()
	}
}

func (s *Service) flushMailbox(c *Connection) []Push {
	if c.pendingMailbox == nil || c.phase != Playing {
		return nil
	}
	snapshot := c.pendingMailbox
	notify := c.pendingMailboxNotify
	c.pendingMailbox = nil
	c.pendingMailboxNotify = false
	if snapshot.MailRevision < c.SelectedAvatarUnsafe().Progress.MailRevision {
		return nil
	}
	for i, av := range c.identity.Avatars {
		if av.Hostnum == c.hostnum {
			c.identity.Avatars[i].Progress.ShortMailInfo = snapshot.ShortMailInfo
			c.identity.Avatars[i].Progress.MailRevision = snapshot.MailRevision
		}
	}
	result := []Push{push("Avatar", "client_prop_changed", []any{"short_mail_info", mailProperties(snapshot.ShortMailInfo, s.Now().Unix())})}
	if notify {
		result = append(result, push("Avatar", "notify_new_mail"))
	}
	return result
}

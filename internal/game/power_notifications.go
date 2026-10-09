package game

func powerPush(c *Connection) Push {
	return push("Avatar", "client_prop_changed", []any{"power", c.SelectedAvatarUnsafe().Progress.Power})
}

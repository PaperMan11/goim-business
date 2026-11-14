package account

// SelectKicked 按每用户/每平台上限选出需要踢出的会话（最旧优先，当前会话除外）。
// 传入的 active 必须已按创建时间升序（最旧在前）。
func SelectKicked(active []*Session, currentSID int64, platform int8, maxUser, maxPlatform int) []int64 {
	platformCount := 0
	for _, item := range active {
		if item.Platform == platform {
			platformCount++
		}
	}
	userOver := len(active) - maxUser
	platformOver := platformCount - maxPlatform

	var kicked []int64
	for _, item := range active {
		if userOver <= 0 && platformOver <= 0 {
			break
		}
		if item.ID == currentSID {
			continue
		}
		needUser := userOver > 0
		needPlatform := item.Platform == platform && platformOver > 0
		if !needUser && !needPlatform {
			continue
		}
		kicked = append(kicked, item.ID)
		if needUser {
			userOver--
		}
		if needPlatform {
			platformOver--
		}
	}
	return kicked
}

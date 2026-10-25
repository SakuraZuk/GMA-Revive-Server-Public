package game

// 原生global_vo允许0表示单卡设置；global_set_vo仍须保存最近的有效全局语种。
// 旧存档没有该字段时优先沿用已保存的全局选择，再回退原生日语默认值。
func effectiveGlobalVoice(p Progress) int {
	for _, voice := range []int{p.GlobalSetVO, p.GlobalVO} {
		if voice == 1 || voice == 2 {
			return voice
		}
	}
	return 2
}

// 原生后缀表把card_vo=0映射到日文事件；缺省单卡设置必须继承有效全局选择。
// 单卡显式选择优先，其次全局选择；只使用该卡原生外观表列出的可用语种。
func effectiveCardVoice(p Progress, id int) int {
	voices := androidCardAppearances[id].Voices
	for _, voice := range []int{p.CardVoices[id], effectiveGlobalVoice(p), 2, 1} {
		if (voice == 1 || voice == 2) && containsInt(voices, voice) {
			return voice
		}
	}
	return 2
}

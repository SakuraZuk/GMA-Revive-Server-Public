package game

import (
	"testing"
	"time"
)

// 只比较候选资格末步；无关材料模拟大存档，不能当线上延迟验收。
func BenchmarkAssistCandidateValidation(b *testing.B) {
	card := newCard(3202, 1, time.Unix(1, 0))
	provider := Avatar{Progress: Progress{Cards: []Card{card}, Materials: map[int]Material{}}}
	for i := 1; i <= 3000; i++ {
		provider.Progress.Materials[i] = Material{ID: i, Count: 10, Total: 20}
	}
	b.Run("原全档快照校验", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := snapshotFriendAssist(provider, card.UUID); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("仅指定卡校验", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := availableAssistCard(provider, card.UUID); err != nil {
				b.Fatal(err)
			}
		}
	})
}

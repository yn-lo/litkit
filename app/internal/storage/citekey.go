package storage

import (
	"errors"
	"fmt"
	"math/rand/v2"
)

// cite_key 生成参数（FR-LIB-06：3 字母 a-zA-Z，空间耗尽升 4/5）。
const (
	citeKeyMinLen   = 3
	citeKeyMaxLen   = 5
	citeKeyAttempts = 50000 // 每长度尝试次数，52^3=14 万空间下 5 万次碰撞概率极低
)

const citeKeyAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// newCiteKey 生成库内唯一的 cite_key。
//
// 每轮先查库去重再返回；3 字母空间耗尽时升 4/5 字母。
func (s *Store) newCiteKey() (string, error) {
	for length := citeKeyMinLen; length <= citeKeyMaxLen; length++ {
		for range citeKeyAttempts {
			k := randomKey(length)
			var n int
			if err := s.db.QueryRow("SELECT COUNT(*) FROM papers WHERE cite_key = ?", k).Scan(&n); err != nil {
				return "", fmt.Errorf("storage cite key check: %w", err)
			}
			if n == 0 {
				return k, nil
			}
		}
	}
	return "", errors.New("storage: cite_key 空间耗尽（52^5 全被占用）")
}

// randomKey 生成 n 位 a-zA-Z 随机串（非加密用途，gosec G404 已在配置豁免）。
func randomKey(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = citeKeyAlphabet[rand.IntN(len(citeKeyAlphabet))]
	}
	return string(b)
}

package gssc

// EstimateTokens 估算一段文本的 token 数量
//
// 使用字符级启发式算法，适配中英文混合场景：
//   - CJK 字符（中日韩统一表意文字）：约 2 tokens/字
//   - 其他字符（英文、数字、标点等）：约 0.25 tokens/字（4 字符 ≈ 1 token）
func EstimateTokens(text string) int64 {
	var count float64
	for _, r := range text {
		if isCJK(r) {
			count += 2
		} else {
			count += 0.25
		}
	}
	return int64(count)
}

func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Unified Ideographs Extension A
		(r >= 0x20000 && r <= 0x2A6DF) || // CJK Unified Ideographs Extension B
		(r >= 0x2A700 && r <= 0x2B73F) || // CJK Unified Ideographs Extension C
		(r >= 0x2B740 && r <= 0x2B81F) || // CJK Unified Ideographs Extension D
		(r >= 0x2B820 && r <= 0x2CEAF) || // CJK Unified Ideographs Extension E
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility Ideographs
		(r >= 0x2F800 && r <= 0x2FA1F) // CJK Compatibility Ideographs Supplement
}

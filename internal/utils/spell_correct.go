package utils

func levenshtein(a, b string) int {
    la, lb := len([]rune(a)), len([]rune(b))
    dp := make([][]int, la+1)
    for i := range dp {
        dp[i] = make([]int, lb+1)
        dp[i][0] = i
    }
    for j := 0; j <= lb; j++ { dp[0][j] = j }
    for i := 1; i <= la; i++ {
        for j := 1; j <= lb; j++ {
            if []rune(a)[i-1] == []rune(b)[j-1] {
                dp[i][j] = dp[i-1][j-1]
            } else {
                dp[i][j] = 1 + min(dp[i-1][j], dp[i][j-1], dp[i-1][j-1])
            }
        }
    }
    return dp[la][lb]
}

// CorrectWord — cari kata paling mirip di kamus, max edit distance 2
func CorrectWord(word string, dictionary []string) string {
    best, bestDist := word, 3 // threshold max
    for _, candidate := range dictionary {
        d := levenshtein(word, candidate)
        if d < bestDist {
            best, bestDist = candidate, d
        }
    }
    return best
}

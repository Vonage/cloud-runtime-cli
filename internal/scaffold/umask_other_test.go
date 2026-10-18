//go:build !unix

package scaffold_test

func setUmask(int) int { return 0 }

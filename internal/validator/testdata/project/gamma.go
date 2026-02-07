package main

// GammaFunc depends on BetaFunc from beta.go (transitive dependency on AlphaFunc).
func GammaFunc() string {
	return "gamma-" + BetaFunc()
}

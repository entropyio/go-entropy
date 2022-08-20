package tests

import (
	"testing"
)

func TestBlockchain(t *testing.T) {
	t.Parallel()

	bt := new(testMatcher)
	// General state tests are 'exported' as blockchain tests, but we can run them natively.
	// For speedier CI-runs, the line below can be uncommented, so those are skipped.
	// For now, in hardfork-times (Berlin), we run the tests both as StateTests and
	// as blockchain tests, since the latter also covers things like receipt root
	bt.skipLoad(`^GeneralStateTests/`)

	// Skip random failures due to selfish mining test
	bt.skipLoad(`.*bcForgedTest/bcForkUncle\.json`)

	// Slow tests
	bt.slow(`.*bcExploitTest/DelegateCallSpam.json`)
	bt.slow(`.*bcExploitTest/ShanghaiLove.json`)
	bt.slow(`.*bcExploitTest/SuicideIssue.json`)
	bt.slow(`.*/bcForkStressTest/`)
	bt.slow(`.*/bcGasPricerTest/RPC_API_Test.json`)
	bt.slow(`.*/bcWalletTest/`)

	// Very slow test
	bt.skipLoad(`.*/stTimeConsuming/.*`)
	// test takes a lot for time and goes easily OOM because of sha3 calculation on a huge range,
	// using 4.6 TGas
	bt.skipLoad(`.*randomStatetest94.json.*`)
	bt.walk(t, blockTestDir, func(t *testing.T, name string, test *BlockTest) {
		if err := bt.checkFailure(t, test.Run(false)); err != nil {
			t.Errorf("test without snapshotter failed: %v", err)
		}
		if err := bt.checkFailure(t, test.Run(true)); err != nil {
			t.Errorf("test with snapshotter failed: %v", err)
		}
	})
	// There is also a LegacyTests folder, containing blockchain tests generated
	// prior to Istanbul. However, they are all derived from GeneralStateTests,
	// which run natively, so there's no reason to run them here.
}

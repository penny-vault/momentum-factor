package mom_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/momentum-factor/mom"
	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

var _ = Describe("MomentumFactor", func() {
	var (
		ctx       context.Context
		snap      *data.SnapshotProvider
		nyc       *time.Location
		startDate time.Time
		endDate   time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		nyc, err = time.LoadLocation("America/New_York")
		Expect(err).NotTo(HaveOccurred())

		snap, err = data.NewSnapshotProvider("testdata/snapshot.db")
		Expect(err).NotTo(HaveOccurred())

		startDate = time.Date(2024, 1, 1, 0, 0, 0, 0, nyc)
		endDate = time.Date(2024, 4, 1, 0, 0, 0, 0, nyc)
	})

	AfterEach(func() {
		if snap != nil {
			snap.Close()
		}
	})

	runBacktest := func() portfolio.Portfolio {
		// Default qmom configuration: FIP filter on (FipQuantile=0.5) and
		// market-cap floor at $1B. IndexName is overridden to SPX so the
		// snapshot fixture stays small (NDX is not available locally and
		// us-tradable explodes the fixture beyond a reasonable size).
		strategy := &mom.MomentumFactor{
			IndexName:    "SPX",
			TopHoldings:  50,
			FipQuantile:  0.50,
			MinMarketCap: 1_000_000_000,
		}
		acct := portfolio.New(
			portfolio.WithCash(100000, startDate),
			portfolio.WithAllMetrics(),
		)

		eng := engine.New(strategy,
			engine.WithDataProvider(snap),
			engine.WithAssetProvider(snap),
			engine.WithAccount(acct),
		)

		result, err := eng.Backtest(ctx, startDate, endDate)
		Expect(err).NotTo(HaveOccurred())
		return result
	}

	It("rebalances monthly at month-end", func() {
		result := runBacktest()
		txns := result.Transactions()

		rebalanceDates := map[string]bool{}
		for _, t := range txns {
			if t.Type == asset.BuyTransaction || t.Type == asset.SellTransaction {
				rebalanceDates[t.Date.In(nyc).Format("2006-01-02")] = true
			}
		}

		Expect(rebalanceDates).To(HaveKey("2024-01-31"))
		Expect(rebalanceDates).To(HaveKey("2024-02-29"))
		Expect(rebalanceDates).To(HaveKey("2024-03-28"))
	})

	It("holds approximately TopHoldings stocks per rebalance", func() {
		result := runBacktest()
		txns := result.Transactions()

		firstRebalanceTickers := map[string]bool{}
		for _, t := range txns {
			d := t.Date.In(nyc).Format("2006-01-02")
			if d == "2024-01-31" && t.Type == asset.BuyTransaction {
				firstRebalanceTickers[t.Asset.Ticker] = true
			}
		}

		Expect(len(firstRebalanceTickers)).To(BeNumerically(">=", 45))
		Expect(len(firstRebalanceTickers)).To(BeNumerically("<=", 50))
	})

	It("selects high-momentum stocks that survive the FIP filter", func() {
		result := runBacktest()
		txns := result.Transactions()

		firstRebalanceTickers := map[string]bool{}
		for _, t := range txns {
			d := t.Date.In(nyc).Format("2006-01-02")
			if d == "2024-01-31" && t.Type == asset.BuyTransaction {
				firstRebalanceTickers[t.Asset.Ticker] = true
			}
		}

		// NVDA was the top SPX momentum stock in Jan 2024 (~153% 12-1) and its
		// 2023 ascent was smooth, so it survives the FIP filter.
		Expect(firstRebalanceTickers).To(HaveKey("NVDA"))
	})

	It("generates a meaningful number of trades over the quarter", func() {
		result := runBacktest()
		txns := result.Transactions()

		var tradeCount int
		for _, t := range txns {
			if t.Type == asset.BuyTransaction || t.Type == asset.SellTransaction {
				tradeCount++
			}
		}

		// 3 monthly rebalances with up to ~50 buys + ~50 sells each.
		Expect(tradeCount).To(BeNumerically(">=", 100))
		Expect(tradeCount).To(BeNumerically("<=", 300))
	})
})

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
		endDate = time.Date(2025, 1, 1, 0, 0, 0, 0, nyc)
	})

	AfterEach(func() {
		if snap != nil {
			snap.Close()
		}
	})

	runBacktest := func() portfolio.Portfolio {
		strategy := &mom.MomentumFactor{IndexName: "SPX"}
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

	It("produces expected returns and final value", func() {
		result := runBacktest()

		summary, err := result.Summary()
		Expect(err).NotTo(HaveOccurred())
		Expect(summary.TWRR).To(BeNumerically("~", 0.1818, 0.01))
		Expect(result.Value()).To(BeNumerically("~", 118185, 500))
	})

	It("rebalances monthly at month-end", func() {
		result := runBacktest()
		txns := result.Transactions()

		rebalanceDates := map[string]bool{}
		for _, t := range txns {
			if t.Type == asset.BuyTransaction || t.Type == asset.SellTransaction {
				rebalanceDates[t.Date.In(nyc).Format("2006-01-02")] = true
			}
		}

		// Should rebalance every month-end in 2024
		Expect(rebalanceDates).To(HaveKey("2024-01-31"))
		Expect(rebalanceDates).To(HaveKey("2024-06-28"))
		Expect(rebalanceDates).To(HaveKey("2024-12-31"))
	})

	It("holds approximately 50 stocks per rebalance", func() {
		result := runBacktest()
		txns := result.Transactions()

		// Count unique tickers bought on first rebalance
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

	It("selects high-momentum stocks", func() {
		result := runBacktest()
		txns := result.Transactions()

		firstRebalanceTickers := map[string]bool{}
		for _, t := range txns {
			d := t.Date.In(nyc).Format("2006-01-02")
			if d == "2024-01-31" && t.Type == asset.BuyTransaction {
				firstRebalanceTickers[t.Asset.Ticker] = true
			}
		}

		// NVDA was the top momentum stock in Jan 2024 (153% 12-1 momentum)
		Expect(firstRebalanceTickers).To(HaveKey("NVDA"))
		// META had 138% momentum (older snapshots may carry the predecessor FB ticker)
		Expect(firstRebalanceTickers).To(SatisfyAny(
			HaveKey("META"),
			HaveKey("FB"),
		))
		// AMD had 96% momentum
		Expect(firstRebalanceTickers).To(HaveKey("AMD"))
	})

	It("generates a meaningful number of trades over the year", func() {
		result := runBacktest()
		txns := result.Transactions()

		var tradeCount int
		for _, t := range txns {
			if t.Type == asset.BuyTransaction || t.Type == asset.SellTransaction {
				tradeCount++
			}
		}

		// 12 monthly rebalances with ~40 trades each
		Expect(tradeCount).To(BeNumerically(">=", 400))
		Expect(tradeCount).To(BeNumerically("<=", 700))
	})
})

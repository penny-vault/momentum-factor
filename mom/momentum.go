// Copyright 2021-2026
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mom

import (
	"context"
	_ "embed"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
)

//go:embed README.md
var description string

// MomentumFactor implements 12-1 momentum stock selection with optional
// Frog-In-the-Pan (FIP) smoothness filtering and a market-cap floor. The
// vanilla configuration (FipQuantile=1.0, MinMarketCap=0) reduces to the
// classic Jegadeesh & Titman (1993) strategy. The default configuration
// (FipQuantile=0.5, MinMarketCap=1e9) implements the Gray & Vogel (2016)
// "Quantitative Momentum" formulation.
type MomentumFactor struct {
	IndexName    string  `pvbt:"index" desc:"Stock index universe to select from" default:"us-tradable" suggest:"us-tradable=us-tradable|SPX=SPX|NDX=NDX"`
	TopHoldings  int     `pvbt:"top-holdings" desc:"Final number of stocks to hold (after the FIP filter halves the momentum cut)" default:"50" suggest:"SP500=50|NASDAQ100=10"`
	FipQuantile  float64 `pvbt:"fip-quantile" desc:"Of the momentum-ranked stocks, smoothest fraction by FIP to hold; 1.0 disables the FIP filter" default:"0.50" suggest:"classic=1.0|qmom=0.50"`
	MinMarketCap float64 `pvbt:"min-market-cap" desc:"Minimum market capitalization (USD); set negative to disable" default:"1000000000" suggest:"classic=-1|qmom=1000000000"`
	TrendFilter  bool    `pvbt:"trend-filter" desc:"When enabled, hold the cash ticker whenever VFINX 1-3-6 risk-adjusted momentum is non-positive" default:"false"`
	CashTicker   string  `pvbt:"cash-ticker" desc:"Defensive ticker held when the trend filter is bearish" default:"BIL"`
}

func (s *MomentumFactor) Name() string {
	return "Momentum Factor"
}

func (s *MomentumFactor) Setup(_ *engine.Engine) {}

func (s *MomentumFactor) Describe() engine.StrategyDescription {
	return engine.StrategyDescription{
		ShortCode:   "mom",
		Description: description,
		Source:      "https://doi.org/10.1111/j.1540-6261.1993.tb04702.x",
		Version:     "1.1.0",
		VersionDate: time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC),
		Schedule:    "@monthend",
		Benchmark:   "SPY",
	}
}

func (s *MomentumFactor) Compute(ctx context.Context, eng *engine.Engine, _ portfolio.Portfolio, batch *portfolio.Batch) error {
	topHoldings := s.TopHoldings
	if topHoldings < 1 {
		topHoldings = 50
	}

	fipQuantile := s.FipQuantile
	if fipQuantile <= 0 || fipQuantile > 1 {
		fipQuantile = 0.50
	}
	// Momentum-rank cut size: pick enough names that the FIP halving lands at
	// topHoldings. With FipQuantile=0.50 and TopHoldings=50 this is 100; with
	// FipQuantile=1.0 it equals TopHoldings (FIP off).
	momentumCutTarget := int(math.Round(float64(topHoldings) / fipQuantile))
	if momentumCutTarget < topHoldings {
		momentumCutTarget = topHoldings
	}

	// Optional trend filter: when VFINX's 1-3-6 risk-adjusted momentum score
	// is at or below zero, allocate to the defensive cash ticker and skip the
	// momentum rebalance. Targets the unhedged-momentum-crash failure mode
	// (e.g. 2008). Off by default; pays a small whipsaw cost in mostly-bull
	// regimes for substantial protection in deep, sustained bear markets.
	if s.TrendFilter {
		bearish, err := s.evaluateTrendFilter(ctx, eng, batch)
		if err != nil {
			return err
		}

		if bearish {
			cash := eng.Asset(s.CashTicker)
			alloc := portfolio.Allocation{
				Date:          eng.CurrentDate(),
				Members:       map[asset.Asset]float64{cash: 1.0},
				Justification: fmt.Sprintf("trend filter: VFINX 1-3-6 momentum non-positive, holding %s", s.CashTicker),
			}

			return batch.RebalanceTo(ctx, alloc)
		}
	}

	indexUniverse := eng.IndexUniverse(s.IndexName)

	// One daily window covers both the monthly 12-1 momentum signal (after
	// downsampling) and the per-stock daily FIP analysis when enabled.
	dailyDF, err := indexUniverse.Window(ctx, portfolio.Months(13), data.MetricClose)
	if err != nil {
		return fmt.Errorf("failed to fetch index prices: %w", err)
	}

	// Optional market-cap filter: Gray/Vogel exclude microcaps before ranking
	// because momentum on illiquid microcaps degenerates into noise.
	qualifyingByMarketCap := map[asset.Asset]bool{}

	useMarketCapFilter := s.MinMarketCap > 0
	if useMarketCapFilter {
		mcDF, err := indexUniverse.At(ctx, data.MarketCap)
		if err != nil {
			return fmt.Errorf("failed to fetch market caps: %w", err)
		}

		for _, stock := range mcDF.AssetList() {
			mc := mcDF.Value(stock, data.MarketCap)
			if !math.IsNaN(mc) && mc >= s.MinMarketCap {
				qualifyingByMarketCap[stock] = true
			}
		}
	}

	monthly := dailyDF.Downsample(data.Monthly).Last()
	if monthly.Len() < 13 {
		return nil
	}

	// 12-1 momentum = (1 + ret12) / (1 + ret1) - 1, the return from month
	// t-12 to t-1, dropping the most recent month to avoid short-term
	// reversal contamination.
	ret12 := monthly.Pct(12)
	ret1 := monthly.Pct(1)
	momentum := ret12.AddScalar(1).Div(ret1.AddScalar(1)).AddScalar(-1).Last()

	if momentum.Len() == 0 {
		return nil
	}

	useFipFilter := fipQuantile < 1.0

	// FIP needs the second-to-last monthly bar's date to bound the formation
	// window when enabled. When disabled, this branch is skipped.
	var (
		cutoffIdx  int
		dailyTimes []time.Time
	)

	if useFipFilter {
		monthlyTimes := monthly.Times()
		if len(monthlyTimes) < 2 {
			return nil
		}

		fipCutoff := monthlyTimes[len(monthlyTimes)-2]
		dailyTimes = dailyDF.Times()

		cutoffIdx = lastIdxOnOrBefore(dailyTimes, fipCutoff)
		if cutoffIdx < 2 {
			return nil
		}
	}

	type stockScore struct {
		stock asset.Asset
		mom   float64
		fip   float64
	}

	ranked := make([]stockScore, 0, len(momentum.AssetList()))
	for _, stock := range momentum.AssetList() {
		if useMarketCapFilter && !qualifyingByMarketCap[stock] {
			continue
		}

		score := momentum.Value(stock, data.MetricClose)
		if !math.IsNaN(score) {
			ranked = append(ranked, stockScore{stock: stock, mom: score})
		}
	}

	if len(ranked) == 0 {
		return nil
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].mom > ranked[j].mom
	})

	momentumCount := momentumCutTarget
	if momentumCount > len(ranked) {
		momentumCount = len(ranked)
	}

	momentumWinners := ranked[:momentumCount]

	// Final selection. With FIP enabled we re-rank the momentum winners by
	// smoothness and keep the top half; otherwise momentum winners are the
	// final selection.
	var selected []stockScore

	if useFipFilter {
		for idx := range momentumWinners {
			prices := dailyDF.Column(momentumWinners[idx].stock, data.MetricClose)
			if cutoffIdx >= len(prices) {
				momentumWinners[idx].fip = math.NaN()
				continue
			}

			momentumWinners[idx].fip = computeFIP(prices[:cutoffIdx+1], momentumWinners[idx].mom)
		}

		valid := make([]stockScore, 0, len(momentumWinners))
		for _, w := range momentumWinners {
			if !math.IsNaN(w.fip) {
				valid = append(valid, w)
			}
		}

		if len(valid) == 0 {
			return nil
		}

		// Lower (more negative) FIP = smoother return path = better persistence.
		sort.Slice(valid, func(i, j int) bool {
			return valid[i].fip < valid[j].fip
		})

		fipCount := int(math.Round(float64(len(valid)) * fipQuantile))
		if fipCount < 1 {
			fipCount = 1
		}

		if fipCount > len(valid) {
			fipCount = len(valid)
		}

		selected = valid[:fipCount]
	} else {
		selected = momentumWinners
	}

	weight := 1.0 / float64(len(selected))

	members := make(map[asset.Asset]float64, len(selected))
	for _, sm := range selected {
		members[sm.stock] = weight
	}

	mcSuffix := ""
	if useMarketCapFilter {
		mcSuffix = fmt.Sprintf(" (min mc $%.0fM)", s.MinMarketCap/1e6)
	}

	var justification string
	if useFipFilter {
		justification = fmt.Sprintf(
			"top %d/%d by smoothest FIP within top %d/%d 12-1 momentum from %s%s",
			len(selected), len(momentumWinners), momentumCount, len(ranked), s.IndexName, mcSuffix,
		)
	} else {
		justification = fmt.Sprintf(
			"top %d/%d by 12-1 momentum from %s%s",
			len(selected), len(ranked), s.IndexName, mcSuffix,
		)
	}

	batch.Annotate("universe-size", fmt.Sprintf("%d", len(ranked)))
	batch.Annotate("momentum-winners", fmt.Sprintf("%d", momentumCount))
	batch.Annotate("justification", justification)

	allocation := portfolio.Allocation{
		Date:          eng.CurrentDate(),
		Members:       members,
		Justification: justification,
	}

	if err := batch.RebalanceTo(ctx, allocation); err != nil {
		return fmt.Errorf("rebalance failed: %w", err)
	}

	return nil
}

// evaluateTrendFilter returns true when VFINX's 1-3-6 risk-adjusted momentum
// score (the average of 1, 3, and 6 month RiskAdjustedPct) is at or below
// zero. VFINX is the strategy's benchmark and serves as the broad-equity
// trend signal.
func (s *MomentumFactor) evaluateTrendFilter(ctx context.Context, eng *engine.Engine, batch *portfolio.Batch) (bool, error) {
	trendAsset := eng.Asset("VFINX")
	trendUniv := eng.Universe(trendAsset)

	// Need 7 monthly bars to compute 6-month momentum.
	trendDF, err := trendUniv.Window(ctx, portfolio.Months(7), data.AdjClose)
	if err != nil {
		return false, fmt.Errorf("trend filter fetch: %w", err)
	}

	monthly := trendDF.Downsample(data.Monthly).Last()
	if monthly.Len() < 7 {
		return false, nil
	}

	mom1 := monthly.RiskAdjustedPct(1)
	mom3 := monthly.RiskAdjustedPct(3)
	mom6 := monthly.RiskAdjustedPct(6)

	score := mom1.Add(mom3).Add(mom6).DivScalar(3).Drop(math.NaN()).Last()
	if score.Len() == 0 {
		return false, nil
	}

	scoreVal := score.Value(trendAsset, data.AdjClose)
	if math.IsNaN(scoreVal) {
		return false, nil
	}

	bearish := scoreVal <= 0
	batch.Annotate("trend-filter", map[bool]string{true: "bearish", false: "bullish"}[bearish])
	batch.Annotate("trend-score", fmt.Sprintf("%.4f", scoreVal))

	return bearish, nil
}

// computeFIP returns the Frog-In-the-Pan score from Da, Gurun & Warachka
// (2014): sign(PRET) * (%neg - %pos), where %pos and %neg are the fractions
// of trading days in the formation window with positive and negative
// returns. For a winner (PRET > 0), a smooth uptrend has many positive days
// and few negative ones, driving the score very negative. A jumpy winner
// driven by a few large positive days has roughly balanced %pos and %neg
// and therefore a near-zero score. Lower is better.
func computeFIP(prices []float64, pret float64) float64 {
	if len(prices) < 3 {
		return math.NaN()
	}

	pos, neg, total := 0, 0, 0

	for i := 1; i < len(prices); i++ {
		prev, curr := prices[i-1], prices[i]
		if math.IsNaN(prev) || math.IsNaN(curr) || prev == 0 {
			continue
		}

		ret := (curr - prev) / prev
		switch {
		case ret > 0:
			pos++
		case ret < 0:
			neg++
		}

		total++
	}

	if total == 0 {
		return math.NaN()
	}

	pPos := float64(pos) / float64(total)
	pNeg := float64(neg) / float64(total)
	sign := 0.0

	switch {
	case pret > 0:
		sign = 1
	case pret < 0:
		sign = -1
	}

	return sign * (pNeg - pPos)
}

// lastIdxOnOrBefore returns the largest index i such that times[i] <= target,
// or -1 if no such index exists. Times are assumed strictly increasing.
func lastIdxOnOrBefore(times []time.Time, target time.Time) int {
	last := -1

	for i, t := range times {
		if t.After(target) {
			break
		}

		last = i
	}

	return last
}

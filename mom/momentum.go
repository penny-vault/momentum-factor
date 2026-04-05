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

// MomentumFactor implements the classic Jegadeesh & Titman (1993) 12-1 momentum
// strategy. It ranks stocks by their 12-month return excluding the most recent
// month, then buys the top holdings equal-weighted.
type MomentumFactor struct {
	IndexName   string `pvbt:"index" desc:"Stock index universe to select from" default:"SPX" suggest:"SPX=SPX|NDX=NDX"`
	TopHoldings int    `pvbt:"top-holdings" desc:"Number of top momentum stocks to hold" default:"50" suggest:"SP500=50|NASDAQ100=10"`
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
		Version:     "1.0.0",
		VersionDate: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
		Schedule:    "@monthend",
		Benchmark:   "VFINX",
	}
}

func (s *MomentumFactor) Compute(ctx context.Context, eng *engine.Engine, strategyPortfolio portfolio.Portfolio, batch *portfolio.Batch) error {
	// 1. Get the index universe for the current date.
	indexUniverse := eng.IndexUniverse(s.IndexName)

	// 2. Fetch 13-month window of monthly close prices for all members.
	//    We need 13 months because Pct(12) requires 13 data points to compute
	//    a 12-period return, and we also skip the most recent month.
	priceDF, err := indexUniverse.Window(ctx, portfolio.Months(13), data.MetricClose)
	if err != nil {
		return fmt.Errorf("failed to fetch index prices: %w", err)
	}

	// 3. Downsample to monthly frequency.
	monthly := priceDF.Downsample(data.Monthly).Last()

	// Need at least 13 rows: 12 months of history + current month.
	if monthly.Len() < 13 {
		return nil
	}

	// 4. Compute 12-1 momentum: return from t-12 to t-1 (skip most recent month).
	//    This is the 12-month return as of last month, not as of today.
	//    ret12 = price[t-1] / price[t-12] - 1
	//    We drop the last row (current month) before computing Pct(11) on the remaining data,
	//    or equivalently: compute Pct(11) on data lagged by 1.
	//
	//    Simpler approach: compute the 12-month return, then the 1-month return,
	//    and derive 12-1 as: (1 + ret12) / (1 + ret1) - 1
	ret12 := monthly.Pct(12)
	ret1 := monthly.Pct(1)

	// 12-1 momentum = (1 + ret12) / (1 + ret1) - 1
	// Take only the last row; individual NaN scores are filtered in the ranking loop.
	momentum := ret12.AddScalar(1).Div(ret1.AddScalar(1)).AddScalar(-1).Last()

	if momentum.Len() == 0 {
		return nil
	}

	// 5. Rank all stocks by 12-1 momentum descending, select top holdings.
	type stockMomentum struct {
		stock asset.Asset
		score float64
	}

	var ranked []stockMomentum

	for _, stock := range momentum.AssetList() {
		score := momentum.Value(stock, data.MetricClose)
		if !math.IsNaN(score) {
			ranked = append(ranked, stockMomentum{stock: stock, score: score})
		}
	}

	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})

	topCount := s.TopHoldings
	if topCount > len(ranked) {
		topCount = len(ranked)
	}

	if topCount == 0 {
		return nil
	}

	selected := ranked[:topCount]

	// 6. Equal weight across selected stocks.
	weight := 1.0 / float64(topCount)
	members := make(map[asset.Asset]float64, topCount)

	for _, sm := range selected {
		members[sm.stock] = weight
	}

	justification := fmt.Sprintf("top %d/%d by 12-1 momentum from %s", topCount, len(ranked), s.IndexName)

	batch.Annotate("universe-size", fmt.Sprintf("%d", len(ranked)))
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

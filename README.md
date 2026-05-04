# Momentum Factor

The **Momentum Factor** strategy ranks stocks by 12-1 momentum (the trailing 12-month return excluding the most recent month) and holds the top names equal-weighted. The default configuration also applies a **Frog-In-the-Pan (FIP)** smoothness filter and a **market-cap floor**, implementing the Gray & Vogel (2016) "Quantitative Momentum" formulation. Both filters can be disabled to recover the original Jegadeesh & Titman (1993) strategy.

The two refinements on top of vanilla 12-1 momentum:

1. **Market-cap floor**: exclude stocks below `MinMarketCap` (default $1B). Microcap momentum is mostly noise; filtering it out gives the FIP filter a cleaner pool to select from.
2. **FIP smoothness filter**: from Da, Gurun, and Warachka (2014). The intuition is behavioral: returns that arrived in many small steps persist better than returns that arrived in a few large jumps, because investors are slow to react to drip-fed news. Score is `sign(PRET) * (%neg - %pos)` over daily returns in the formation window — lower (more negative) means smoother.

## Rules

1. On the last trading day of the month, fetch the universe of stocks meeting the market-cap floor (if enabled).
2. For each qualifying stock, compute 12-1 momentum: the return from month *t-12* to month *t-1*, dropping the most recent month to avoid short-term reversal.
3. Rank stocks by 12-1 momentum descending. Keep the top `TopHoldings / FipQuantile` names. With the defaults that's the top 100.
4. **If FIP is enabled** (`FipQuantile < 1.0`): re-rank the momentum winners by FIP score ascending and keep the smoothest `FipQuantile` fraction. With the defaults that's the top 50 of the 100 momentum names.
5. Equal weight across the final selection.
6. Hold all positions until the close of the following month, then re-rank and rebalance.

## Parameters

- **Index**: stock universe to draw from (default: `us-tradable`, the broad US liquid equity universe; `SPX` and `NDX` are also available).
- **Top Holdings**: final number of stocks to hold (default: `50`).
- **FIP Quantile**: smoothest fraction of momentum-ranked stocks to actually hold (default: `0.50`). Set to `1.0` to disable the FIP filter and reduce to vanilla Jegadeesh-Titman momentum. The momentum cut is sized as `TopHoldings / FipQuantile` so the FIP halving lands back at `TopHoldings`.
- **Min Market Cap**: minimum market cap in USD for a stock to qualify (default: `1000000000` = $1B). Set negative to disable; use `--preset classic` for the classic configuration.
- **Trend Filter**: when enabled, holds the cash ticker whenever VFINX's 1-3-6 risk-adjusted momentum score is at or below zero (default: `false`). Targets deep, sustained bear markets; pays a small whipsaw cost in mostly-bull regimes.
- **Cash Ticker**: defensive ticker held when the trend filter is bearish (default: `BIL`).

## Presets

- `--preset qmom` (default behavior): `FipQuantile=0.50, MinMarketCap=1e9`. Implements Gray & Vogel's "Quantitative Momentum" formulation.
- `--preset classic`: `FipQuantile=1.0, MinMarketCap=0`. Reduces to vanilla Jegadeesh-Titman 12-1 momentum.

## References

- Gray, W. and Vogel, J. (2016). *Quantitative Momentum: A Practitioner's Guide to Building a Momentum-Based Stock Selection System*. Wiley.
- Da, Z., Gurun, U., and Warachka, M. (2014). "Frog in the Pan: Continuous Information and Momentum." *Review of Financial Studies*, 27(7), 2171-2218.
- Jegadeesh, N. and Titman, S. (1993). "Returns to Buying Winners and Selling Losers: Implications for Stock Market Efficiency." *Journal of Finance*, 48(1), 65-91.
- Asness, C., Moskowitz, T., and Pedersen, L. (2013). "Value and Momentum Everywhere." *Journal of Finance*, 68(3), 929-985.

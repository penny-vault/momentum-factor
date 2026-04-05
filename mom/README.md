# Momentum Factor (12-1)

The **Momentum Factor** strategy is based on the seminal research by Jegadeesh and Titman (1993). It is the most studied equity anomaly in finance. The strategy ranks stocks by their 12-month return excluding the most recent month (to avoid short-term reversal effects), then buys the top decile equal-weighted.

## Rules

1. On the last trading day of the month, compute the 12-1 momentum for each stock in the universe:
   - 12-1 Momentum = total return over the past 12 months, excluding the most recent month
   - This is equivalent to the return from month t-12 to month t-1
2. Rank all stocks by 12-1 momentum descending.
3. Select the top decile (top 10% of stocks by count) or a fixed number of top holdings.
4. Equal weight across selected stocks.
5. Hold all positions until the close of the following month, then re-rank and rebalance.

The 1-month skip avoids the well-documented short-term reversal effect where last month's winners tend to reverse in the near term.

## Parameters

- **Index**: Which stock universe to draw from (default: S&P 500)
- **Top Holdings**: Number of stocks to hold (default: 50, roughly top decile of S&P 500)

## References

- Jegadeesh, N. and Titman, S. (1993). "Returns to Buying Winners and Selling Losers: Implications for Stock Market Efficiency." *Journal of Finance*, 48(1), 65-91.
- Asness, C., Moskowitz, T., and Pedersen, L. (2013). "Value and Momentum Everywhere." *Journal of Finance*, 68(3), 929-985.

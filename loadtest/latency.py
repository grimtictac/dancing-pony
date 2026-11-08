#!/usr/bin/env python3
"""Server-side latency between two /metrics snapshots.

    latency.py before.txt after.txt

Reads pony_http_request_duration_seconds from both and prints, per route,
the requests served in between and their mean, p50 and p95 latency. This is
the time inside the app (from the first middleware to the response), so it
leaves out the client's network, which dominates when load is generated far
from the server. Percentiles are interpolated within histogram buckets, so
they are estimates.
"""
import re
import sys
from collections import defaultdict

METRIC = "pony_http_request_duration_seconds"
# Greedy up to the last "} ": route labels contain braces ({slug}).
LINE = re.compile(METRIC + r'_(bucket|sum|count)\{(.*)\} (\S+)$')


def parse(path):
    buckets = defaultdict(dict)
    totals = defaultdict(dict)
    with open(path) as f:
        for line in f:
            m = LINE.match(line)
            if not m:
                continue
            kind, labels, value = m.group(1), m.group(2), float(m.group(3))
            lab = dict(kv.split("=", 1) for kv in re.findall(r'\w+="[^"]*"', labels))
            lab = {k: v.strip('"') for k, v in lab.items()}
            key = (lab.get("method", ""), lab.get("route", ""))
            if kind == "bucket":
                buckets[key][float(lab["le"])] = value
            else:
                totals[key][kind] = value
    return buckets, totals


def quantile(q, cumulative):
    """cumulative: sorted [(upper_bound, count)], last bound is +Inf."""
    total = cumulative[-1][1]
    if total == 0:
        return float("nan")
    target = q * total
    lower_bound, lower_count = 0.0, 0.0
    for bound, count in cumulative:
        if count >= target:
            if bound == float("inf"):
                return lower_bound
            share = (target - lower_count) / (count - lower_count) if count > lower_count else 0
            return lower_bound + share * (bound - lower_bound)
        lower_bound, lower_count = bound, count
    return lower_bound


def main(before_path, after_path):
    b0, t0 = parse(before_path)
    b1, t1 = parse(after_path)
    rows = []
    for key, after in b1.items():
        before = b0.get(key, {})
        cumulative = sorted((le, after[le] - before.get(le, 0.0)) for le in after)
        n = t1[key].get("count", 0) - t0.get(key, {}).get("count", 0)
        if n <= 0:
            continue
        mean = (t1[key].get("sum", 0) - t0.get(key, {}).get("sum", 0)) / n
        rows.append((key, n, mean, quantile(0.5, cumulative), quantile(0.95, cumulative)))
    rows.sort(key=lambda r: -r[1])
    print(f"{'route':<52} {'requests':>9} {'mean':>8} {'p50':>8} {'p95':>8}")
    for (method, route), n, mean, p50, p95 in rows:
        name = route if route.startswith(method) else f"{method} {route}"
        print(f"{name:<52} {int(n):>9} {mean*1000:>6.1f}ms {p50*1000:>6.1f}ms {p95*1000:>6.1f}ms")


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    main(sys.argv[1], sys.argv[2])

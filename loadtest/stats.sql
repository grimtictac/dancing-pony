-- Where Postgres spent its time since the last reset, by statement.
-- make load-stats (after make load-test); make load-test resets it first.
SELECT calls,
       round(total_exec_time::numeric, 0) AS exec_ms,
       round(total_plan_time::numeric, 0) AS plan_ms,
       round(((total_exec_time + total_plan_time) / calls)::numeric, 3) AS ms_per_call,
       left(regexp_replace(query, '\s+', ' ', 'g'), 90) AS query
FROM pg_stat_statements
WHERE calls > 50
ORDER BY total_exec_time + total_plan_time DESC
LIMIT 12;

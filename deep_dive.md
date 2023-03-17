Debugging slow queries

# Simplifying queries

## Smaller queries

Consider this underperforming query:

```sql
With cte1 as (
  Select edible, fuzzy, count(*) as cnt
  from animals
  Group by edible, fuzzy
), cte2 as (
  Select edible, furry, count(*) as cnt
  from food
  Group by edible, fuzzy
), cte3 as (
  Select * from cte1
  Where exists (
    Select * from cte2 where cte1.cnt = cte2.cnt
  ) and
  cte1.fuzzy = false and
  cte2. fuzzy = true
)
select a.name, f.name, f.tasty
from animals a
join food f
on a.fuzzy = f.furry
join cte3 on
  a.fuzzy = cte3.fuzzy and
  a.edible = cte3.edible
where
  f.tasty = true and
  cte1.tasty = true;

```

Most queries can be decomposed into simpler parts. Removing fluff makes it easier to dig into performance bottlenecks.

First, this query has several CTE blocks. Removing CTEs often improves readability, so lets see whether any CTE is a standalone source of slowness:

```sql
> Select edible, fuzzy, count(*) as cnt
  from animals
  Group by edible, fuzzy;
> Select edible, furry, count(*) as cnt
  from food
  Group by edible, fuzzy;
> With cte1 as (
  Select edible, fuzzy, count(*) as cnt
  from animals
  Group by edible, fuzzy
), cte2 as (
  Select edible, furry, count(*) as cnt
  from food
  Group by edible, fuzzy
)
  select count(*) from cte1
  where exists (
    select * from cte2 where cte1.cnt = cte2.cnt
  ) and
  cte1.fuzzy = false and
  cte2. fuzzy = true;
)
```

All of them are fast! We can't quite delete them yet, because they are anchored to the join. But we can "blackbox" cte3 as a non-indexable source of rows that isn't particularly expensive to create.

Let's follow up with the counts of the other two tables:

```sql
> select count(*) from animals;
+----------+
| count(*) |
+----------+
| 10000000 |
+----------+
> select count(*) from food;
+----------+
| count(*) |
+----------+
| 10000000 |
+----------+
```

The tables are about the same size, but they are big! Joins between big tables give room for O(n^2) joins.

Lets see how fast pair-wise joins run:

```sql
> Select *
  From animals a join cte3 on
    a.fuzzy = cte3.fuzzy and
    a.edible = cte3.edible;
```

This one is fast! What about the other:

```sql
> select a.name, f.name, f.tasty
  from animals a
  join food f
  on f.fuzzy = a. fuzzy;
```

This one is slow! We might have found the culprit.

## Shorter runtimes

We carved out at least one subcomponent of the original query that is slow. Rather than debugging the query at its full form, it can be helpful to shorten the runtime of the query without compromising the original performance bottleneck. It is easier to debug a 1 second query than a 100 second query as long as a 10x improvement in the first generalizes to the second.

Here is the slow query we extracted in the last section:


```sql
> select a.name, f.name, f.tasty
  from animals a join food f
  on f.fuzzy = a. fuzzy;
```

Sometime limits can short-circuit an otherwise equivalent query after an arbitrary number of return rows:

```sql
> select a.name, f.name, f.tasty
  from animals a join food f
  on f.fuzzy = a. fuzzy
  limit 2;
```

Limits don't work for queries with distinct or sorting clauses, or queries that return zero rows. Each of these computes the entire result set before applying a limit. We can break those restrictions by removing `DISTINCT` and `ORDER BY` clauses, or by expanding filters to return more result rows.

Another option can add filters to reduce the rows returned from one or more tables:

```sql
> select a.name, f.name, f.tasty
  from animals a join food f
  on f.fuzzy = a. fuzzy
  where a.name = 'walrus';
```

We want more than a few rows, but not many. We also must preserve the original performance bottleneck when simplifying queries. For example, adding filters can change how a range scan is performed. Comparing EXPLAIN outputs and CPU profiles can reassure us that the the faster query is still subject to the original performance bottleneck.

A limit should suffice work for the selected query.

```

=>

```

# EXPLAIN entry point

The output of `EXPLAIN <query>` describes how our optimizer chooses to execute a specific query. This includes details like which columns are read from tables, where filters are placed, which join strategies are used, whether subqueries are cacheable or not. Walking line by line down our simplified and LIMITed plan above (the query passes rows in the reverse, bottom-up):

1. Terminate the query after 2 rows are returned.

2. Project only the `animal.name`, `food.name`, and `food.tasty` columns as outputs.

3. Use a lookup join to find all `animal` X `food` row pairings where `a.fuzzy` = `f.fuzzy`.

4. The join performs a tablescan into the `animal` table.

5. The join performs an index lookup into `food` table parameterized by an `animal.fuzzy` -> `food.fuzzy` key conversion.

At this point we lean on our performance optimization pattern reference.

## Join Order

Is join order constraining this query? If we joined the tables in the other order, all of the access patterns would be inverted but equal. The tables have similar indexes, and though we return one more row from `food`. We can still test to make sure:

```sql
> select \*+ JOIN_ORDER(f,a) *\ a.name, f.name, f.tasty
  from animals a join food f
  on f.fuzzy = a. fuzzy
  limit 2;
```

## Join Operator

The original query used an index lookup to join `animals` and `food`, but we could also use a HASH_JOIN or MERGE_JOIN. Let's try both:

```
> select \*+ MERGE_JOIN(f,a) *\ a.name, f.name, f.tasty
  from animals a join food f
  on f.fuzzy = a. fuzzy
  limit 2;
> select \*+ HASH_JOIN(f,a) *\ a.name, f.name, f.tasty
  from animals a join food f
  on f.fuzzy = a. fuzzy
  limit 2;
```


## Covering lookups

A covering index lookup retrieves all of the values we need projected from a table. The primary index covers all fields. But secondary indexes only cover the indexed fields plus the primary key fields. Covering index scans are always preferred to non-covering lookups, which actually do two lookups.

Our example join performs a non-covering lookup into the `food` table. The lookup into the `fuzzy` index returns key:value pairs that look like `(fuzzy, name): ()`. We need to do a second lookup from a secondary entry into the primary index to retrieve the `tasty` field.

We can add a new index to our table to see how a covering index performs:

```sql
> alter table food add index (fuzzy, tasty);
> select a.name, f.name, f.tasty
  from animals a join food f
  on f.fuzzy = a. fuzzy
  limit 2;
```


# Profile entry point

- Run a profile with `dolt --prof cpu sql -q "<query>"`. Eyeballing a plan does not always tell us why a query is slow. A profile that reports which executors we spent the most time running can shine a light on unsuspected bottlenecks.

A more sophisticated way to debug a slow query uses a profiler.

This is simplified query that a customer was having performance trouble with recently: 

```sql
select * from parts where category = 'auto';
```

The customer initial report included an additional clue: `select * from parts` was fast. This prompted us to look at the distribution of customer parts:

```sql
> select distinct
    category,
    (select count(*) from parts inner
     where inner.category = outer.category) as cnt
  from parts outer
  order by cnt desc;
+------------+--------+
| category   | cnt    |
+------------+--------+
| auto       | 750000 |
| appliance  | 1200   |
| electrical | 50     |
...
```

We push the filter into an index lookup on the `parts` table, but basically ~90% of the table is auto parts. We would call this a particularly "low selectivity" filter because it discards so few rows.

Really we should expect the index scan to do the same thing as a table scan. We can discard the index and rerun the query to compare.

```sql
> alter table parts drop index category;
```

Here is the before/after plans:

```
=>
```

And here is the new performance:

```
> select * from parts where category = 'auto';
```

The read about the same set of rows, but indexed version is worse! Referencing our query optimization reference for [index vs table scan](./README.md#indexscan-vs-tablescan) tells us that lookup introduces an overhead for each read compared to an unordered table scan. But the difference is fairly extreme.

We can run a profile to look at which functions the query spends the most time (after adding back the `category` index):

```bash
> dolt --prof cpu sql -q "select * from parts where category = 'auto';"
```

Here is one visualization:

[profile](./)

We spend a lot of time doing a collation-sensitive string comparison to validate `'auto' = 'auto'`. Not great! We have already fixed this [here], and this is the profile afterwards:

[after profile](./)

And here is the performance:

```sql
> dolt --prof cpu sql -q "select * from parts where category = 'auto';"
```

The EXPLAIN output pointed us in the right direction, but sometimes you need to know what Dolt is doing on the inside.

## Aside

Removing the index helps us find all parts, but makes it more difficult to find every other category:

```sql
select * from parts where category = 'electrical';
```

Alternatively, we could split out an `auto_parts` table if that is a hot path. In which case our originally query would look like:

```sql
> select * from auto_parts;
```

But now DDL and DML is more complicated. We need to make a new table, and then bisect any updates between the two tables. Reads to both need to UNION the results:

```sql
select * from parts
union
select * from auto_parts;
```

Alternatively, we could take advantage of how integer operators are usually faster than string comparisons by splitting category into its own table:
```sql
> select * from categories;
+----+------------+
| id | name       |
+----+------------+
| 2  | appliance  |
| 1  | electrical |
| 0  | auto       |
+----+------------+
```

It is now a bit faster to run `select * from parts where category = 0`, but you have to join the `categories` table to convert between the name and integer ids.

# Summary

The two queries we looked at were only subject to one performance bottleneck each. In the real world, queries usually have several issues that morph during simplification/debugging. The basic workflow for messy queries does not change, but we iterate from the the top each time we fix one bottleneck. And in practice, we hard code fixes to upfront to 1) provide immediate workarounds for customers and 2) see how deep the rabbit holes go before formalizing solution strategies. We sometimes work through 3-4 bug iterations before deciding a query latency is "good enough."

We always prefer adding internal optimizations > adding query hints > adding table indexes > rearranging queries, in that order. But no two queries are the same! The bugs that make it to us coevolve in complexity as Dolt gets smarter.

Refer to the [optimization reference](./README.md) for more details on common optimization patterns.

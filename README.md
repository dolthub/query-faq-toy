# Common performance optimizations

## Indexscan vs Tablescan

It is common to push a permissive filter into a tablescan.
The result is that we scan the entire table, just in a more
expensive way.

The plans below compare a tablescan that will read every row
with the index range scan machinery, versus reading every row:

```
Project
 ├─ columns: [x:0!null, y:1!null, z:2!null]
 └─ IndexedTableAccess(xy)
     ├─ index: [xy.y]
     ├─ static: [{(-1, ∞)}]
     └─ columns: [x y z w]
=>
Project
 ├─ columns: [x:0!null, y:1!null, z:2!null]
 └─ Table
     ├─ name: xy
     └─ columns: [x y z w]
```

Using the index range scan machinery introduces a non-negligible overhead:

```
BenchmarkIndexScan/index_scan_pre-opt
BenchmarkIndexScan/index_scan_pre-opt-12         	     830	   1421220 ns/op
BenchmarkIndexScan/index_scan_post-opt
BenchmarkIndexScan/index_scan_post-opt-12        	    1239	    886823 ns/op
```

## Decorrelate Subqueries

Subqueries can either be cacheable, or non-cacheable. The later are also
called "correlated" subqueries, because they need to be wholesale executed
once for each row in the outer scope.

The first plan below executes the EXISTS subquery once for each `xy` row.
It is not cacheable because its execution depends on `xy.x`; it is correlated
to the outer scope. The second plan hoists the filter, freeing the subquery
into a cacheable form:

```
Filter
 ├─ EXISTS Subquery
 │   ├─ cacheable: false
 │   └─ Filter
 │       ├─ (x = 0)
 │       └─ Table
 │           └─ name: uv
 └─ Table
     ├─ name: xy
     └─ columns: [x y z w]
=>
Filter
 ├─ EXISTS Subquery
 │   ├─ cacheable: true
 │   └─ Table
 │       └─ name: uv
 └─ Filter
     ├─ Eq
     │   ├─ x:0!null
     │   └─ 0 (bigint)
     └─ Table
         ├─ name: xy
         └─ columns: [x y z w]
```

The cacheable form executes the subquery once, rather than 100 times:

```
BenchmarkDecorrelate/uncorrelated_subquery_pre-opt
BenchmarkDecorrelate/uncorrelated_subquery_pre-opt-12         	      46	  22258375 ns/op
BenchmarkDecorrelate/uncorrelated_subquery_post-opt
BenchmarkDecorrelate/uncorrelated_subquery_post-opt-12        	    5415	    193153 ns/op
```

## Covering Index Lookup

Different indexes can be used to read data from disk. A "covering" index
scan is one that provides all columns projected from the scan. A "noncovering"
scan has to read from the lookup index, and then do a second read into the primary
key to fill out additional rows.

The first query uses the `xy.y` index to read `(y): (x)`, (`y` is the secondary key,
`x` is the primary key), but is still missing `z`. So the first query must
do a lookup into the primary key to fetch the remaining field.

The second query is keyed on `(x, z) : ()`, and already has the fields needed
to satisfy the `x,z` projection

```
Project
 ├─ columns: [x:0!null, z:1!null]
 └─ IndexedTableAccess(xy)
     ├─ index: [xy.y]
     ├─ static: [{(0, ∞)}]
     └─ columns: [x z]
=>
Project
 ├─ columns: [x:0!null, z:1!null]
 └─ IndexedTableAccess(xy)
     ├─ index: [xy.x,xy.z]
     ├─ static: [{(0, ∞), [NULL, ∞)}]
     └─ columns: [x z]
```

The second is about twice as fast, because it performs half as many index lookups:

```
BenchmarkCovering/covering_lookup_pre-opt
BenchmarkCovering/covering_lookup_pre-opt-12         	   13773	     92543 ns/op
BenchmarkCovering/covering_lookup_post-opt
BenchmarkCovering/covering_lookup_post-opt-12        	   23208	     44561 ns/op
```

## Joins

The most significant performance choices for joins are:
- What strategy (physical operator) do we use to execute the join?
- What order do we join tables?

The examples below show how even with two tables, operator and order explain
10x complexity differences for the same query. It is important to keep in mind that
join complexity scales with the number of tables in the join tree. A 10x perf hit
for each of 4 tables in a join explodes a 10-second query into a 30-minute query.

### Join Operators

The five join physical operators below implement the same logically equivalent
join between two 100-row tables with 0-99 integer entries. Not every physical
operator is available for every join. It is often possible to add indexes to
support a particular join strategy.

```
BenchmarkJoinOp/inner_join-opt-12         	                  63	    17367096 ns/op
BenchmarkJoinOp/exists_subquery-opt-12        	                 139	    9470630 ns/op
BenchmarkJoinOp/lookup_join-opt-12        	                3631	    321932 ns/op
BenchmarkJoinOp/hash_join-opt-12         	                5151	    195707 ns/op
BenchmarkJoinOp/merge_join-opt-12        	               10000	    112757 ns/op
```

The inner join is slowest because it forces us to read `uv` 100 times,
once for each row in `xy`.

The exists subquery is similar to the inner join, where we will read `uv`
100 times for each `xy` row. The difference here is that we will stop as
soon as we find a single match, rather than finishing scanning `uv`. On
average we will stop after 50% of the table has been compared.

The lookup join is predictably fast, because we use an index to find 
the matching `uv.u` for a given `xy.x` without reading any non-matching
rows.

The hash join is even faster because `uv.u` is small, and the cost
of building a hash map in memory is less expensive than the lookup roundtrips
to disk.

Performing two interleaved table scans on `xy` and `uv` is
the fastest. The key here is to use the comparison direction on `x = u`
to always increment the smaller cursor. A merge join requires neither index
lookup machinery nor a hash map probe table. Lookups outperform merge joins
when reading the entire right table is expensive.

Inner join:
```
InnerJoin
 ├─ Eq
 │   ├─ x:0!null
 │   └─ u:2!null
 ├─ Table
 │   ├─ name: xy
 │   └─ columns: [x y z w]
 └─ Table
     ├─ name: uv
     └─ columns: [u v r s]
<=>
Filter
 ├─ EXISTS Subquery
 │   ├─ cacheable: false
 │   └─ Filter
 │       ├─ (x = u)
 │       └─ Table
 │           └─ name: uv
 └─ Table
     ├─ name: xy
     └─ columns: [x y z w]  
<=>
LookupJoin
 ├─ Eq
 │   ├─ x:0!null
 │   └─ u:2!null
 ├─ Table
 │   ├─ name: xy
 │   └─ columns: [x y z w]
 └─ IndexedTableAccess(uv)
     ├─ index: [uv.u]
     └─ columns: [u v r s]
<=>
HashJoin
 ├─ Eq
 │   ├─ x:0!null
 │   └─ u:2!null
 ├─ Table
 │   ├─ name: xy
 │   └─ columns: [x y z w]
 └─ HashLookup
     ├─ source: x:0!null
     ├─ target: u:0!null
     └─ CachedResults
         └─ Table
             ├─ name: uv
             └─ columns: [u v r s]
<=>
MergeJoin
 ├─ cmp: Eq
 │   ├─ x:0!null
 │   └─ u:2!null
 ├─ IndexedTableAccess(xy)
 │   ├─ index: [xy.x]
 │   ├─ static: [{[NULL, ∞)}]
 │   └─ columns: [x y z w]
 └─ IndexedTableAccess(uv)
     ├─ index: [uv.u]
     ├─ static: [{[NULL, ∞)}]
     └─ columns: [u v r s]
```

### Join Order

In the example below, the tables have sizes xy: 1_000, uv: 10_000.

With a simple join planner, we will always place the smallest table first,
expecting a lookup join of `uv X xy` to cost ~1000 units of CPU, vs `xy X uv`
to cost ~10_000 units.

But the filter on `uv` has a selectivity of 99.9999%, reducing the output
cardinality of `uv` to 1 (a single row). So in reality, ``uv X xy` costs ~1 unit
of computation:

```
LookupJoin
 ├─ Eq
 │   ├─ x:0!null
 │   └─ u:2!null
 ├─ Table
 │   ├─ name: xy
 │   └─ columns: [x y z w]
 └─ Filter
     ├─ Eq
     │   ├─ u:0!null
     │   └─ 0 (bigint)
     └─ IndexedTableAccess(uv)
         ├─ index: [uv.u]
         └─ columns: [u v r s]
=>
LookupJoin
 ├─ Eq
 │   ├─ u:0!null
 │   └─ x:2!null
 ├─ Filter
 │   ├─ Eq
 │   │   ├─ u:0!null
 │   │   └─ 0 (bigint)
 │   └─ Table
 │       ├─ name: uv
 │       └─ columns: [u v r s]
 └─ IndexedTableAccess(xy)
     ├─ index: [xy.x]
     └─ columns: [x y z w]
```

If we cost the join order using filter selectivity, the second query performs 1
iteration vs ~1000 for the default: 

```
BenchmarkJoinOrder/lookup_join_order_pre-opt
BenchmarkJoinOrder/lookup_join_order_pre-opt-12         	    2409	    419858 ns/op
BenchmarkJoinOrder/lookup_join_order_post-opt
BenchmarkJoinOrder/lookup_join_order_post-opt-12        	  200960	      5220 ns/op
```

## Pushdown

"Pushing" filters moves row elimination lower in the tree. The most extreme form
of this moves a filter from the execution tree, transforming a table scan into
a point lookup:

```
Filter
 ├─ Eq
 │   ├─ x:0!null
 │   └─ 0 (bigint)
 └─ Table
     ├─ name: xy
     └─ columns: [x y]
=>
IndexedTableAccess(xy)
 ├─ index: [xy.x]
 ├─ static: [{[0, 0]}]
 └─ columns: [x y]
```

The first query reads the entire table from disk, and then removes all but one.
The second only reads rows from disk where `xy.x` = 0:

```
BenchmarkPushdown/pushdown_filter_pre-opt
BenchmarkPushdown/pushdown_filter_pre-opt-12         	   87451	     13173 ns/op
BenchmarkPushdown/pushdown_filter_post-opt
BenchmarkPushdown/pushdown_filter_post-opt-12        	  520180	      2663 ns/op
```

## Pruning Projections

### Tablescan

Projection pruning is the process of limiting the number of rows
returned by a child relation.

The first benchmark removes a projection by pushing the field
selection into a table scan. Rather than reading four fields
from disk, we will read one:

```
Filter
 ├─ Eq
 │   ├─ x:0!null
 │   └─ 1 (bigint)
 └─ Project
     ├─ columns: [x:0!null]
     └─ Table
         ├─ name: xy
         └─ columns: [x y z w]
=>
Filter
 ├─ Eq
 │   ├─ x:0!null
 │   └─ 1 (bigint)
 └─ Table
     ├─ name: xy
     └─ columns: [x]
```

The benefit is small, but adds up for tables with many columns
or deep joins that pass a lot of data:

```
BenchmarkPrune/prune_projection_pre-opt
BenchmarkPrune/prune_projection_pre-opt-12         	    3462	    298673 ns/op
BenchmarkPrune/prune_projection_post-opt
BenchmarkPrune/prune_projection_post-opt-12        	    5977	    258016 ns/op
```

### Join

The second benchmark performs the same optimization on a join, selecting
only `xy.x` and `uv.u` before the join:

```
Project
 ├─ columns: [x:0!null, u:3!null]
 └─ InnerJoin
     ├─ Eq
     │   ├─ x:0!null
     │   └─ u:3!null
     ├─ Table
     │   ├─ name: xy
     │   └─ columns: [x y z w]
     └─ Table
         ├─ name: uv
         └─ columns: [u v r s]
=>
InnerJoin
 ├─ Eq
 │   ├─ x:0!null
 │   └─ u:1!null
 ├─ Table
 │   ├─ name: xy
 │   └─ columns: [x]
 └─ Table
     ├─ name: uv
     └─ columns: [u]
```

The second selects fewer rows from disk, builds smaller join intermediates,
and removes a projection function call:

```
BenchmarkPrune/pruned_join_pre-opt
BenchmarkPrune/pruned_join_pre-opt-12              	      60	  24666361 ns/op
BenchmarkPrune/pruned_join_post-opt
BenchmarkPrune/pruned_join_post-opt-12             	      68	  19723276 ns/op
```

## Text vs Varchar

Here we have identical tables, but `xy` has TEXT types while `uv` has
VARCHAR types:

```sql
create table xy (
    x int primary key,
    y text,
    z text,
    w text
);
create table uv (
    u int primary key,
    v varchar(100),
    r varchar(100),
    s varchar(100)
);
```

TEXT types are stored as BLOBs out of band, and are considerably more expensive
to read and write.

```
BenchmarkText/text_vs_varchar_pre-opt
BenchmarkText/text_vs_varchar_pre-opt-12         	    2734	    365846 ns/op
BenchmarkText/text_vs_varchar_post-opt
BenchmarkText/text_vs_varchar_post-opt-12        	    4256	    254042 ns/op
```

Plans below:

```
Table
 ├─ name: xy
 └─ columns: [x y z w]
=>
Table
 ├─ name: uv
 └─ columns: [u v r s]
 ```
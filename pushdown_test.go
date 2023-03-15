package query_faq_toy

import (
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/expression"
	"github.com/dolthub/go-mysql-server/sql/plan"
	"github.com/dolthub/go-mysql-server/sql/types"
	"log"
	"strings"
	"testing"
)

func BenchmarkPushdown(b *testing.B) {
	e, ctx := setupMemDB()

	setup := `
use test;
create table xy (x int primary key, y int);
insert into xy values
  (1,0),
  (2,1),
  (0,2),
  (3,3);
create table uv (u int primary key, v int);
insert into uv values
  (0,1),
  (1,1),
  (2,2),
  (3,2);
`

	for _, q := range strings.Split(setup, ";") {
		sch, iter, err := e.Query(ctx, q)
		if err != nil {
			log.Fatalf("setup analyzing query '%s': %s\n", q, err)
		}
		_, err = sql.RowIterToRows(ctx, sch, iter)
		if err != nil {
			log.Fatalf("setup executing query '%s': %s\n", q, err)
		}
	}

	xy, db, err := e.Analyzer.Catalog.Table(ctx, "test", "xy")
	if err != nil {
		log.Fatalf("%s\n", err)
	}

	xyIndexable, ok := xy.(sql.IndexAddressableTable)
	if !ok {
		log.Fatalf("xy not index addressable")
	}
	xyIndexes, err := xyIndexable.GetIndexes(ctx)
	xyPk := xyIndexes[0]

	tests := []struct {
		name string
		pre  sql.Node
		post sql.Node
	}{
		{
			name: "pushdown filter",
			pre: plan.NewFilter(
				expression.NewEquals(
					expression.NewGetField(0, types.Int64, "x", false),
					expression.NewLiteral(0, types.Int64),
				),
				plan.NewResolvedTable(xy, db, nil),
			),
			post: mustStaticIndexedAccessForResolvedTable(
				plan.NewResolvedTable(xy, db, nil),
				sql.IndexLookup{
					Index: xyPk,
					Ranges: sql.RangeCollection{
						sql.Range{sql.ClosedRangeColumnExpr(0, 0, types.Int64)},
					},
				}),
		},
	}

	for _, bb := range tests {
		runBenchmarkComparison(b, ctx, bb.name, bb.pre, bb.post)
	}
}

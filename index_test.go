package query_faq_toy

import (
	"fmt"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/expression"
	"github.com/dolthub/go-mysql-server/sql/plan"
	"github.com/dolthub/go-mysql-server/sql/types"
	"log"
	"strings"
	"testing"
)

func BenchmarkIndexScan(b *testing.B) {
	e, ctx := setupMemDB()

	s := &strings.Builder{}
	s.WriteString("use test;")
	s.WriteString("create table xy (x int primary key, y int, z int, w int, key(x,y), key(y));")
	s.WriteString("create table uv (u int primary key, v int, r int, s int, key (u,v));")
	s.WriteString("insert into xy values\n  ")
	for i := 0; i <= 1000; i++ {
		s.WriteString(fmt.Sprintf("  (%d, %d, %d, %d)", i, i, i, i))
		if i == 1000 {
			s.WriteString(";\n")
		} else {
			s.WriteString(",\n")
		}
	}
	s.WriteString("insert into uv values\n  ")
	for i := 0; i <= 100; i++ {
		s.WriteString(fmt.Sprintf("  (%d, %d, %d, %d)", i, i, i, i))
		if i == 100 {
			s.WriteString(";\n")
		} else {
			s.WriteString(",\n")
		}
	}
	setup := s.String()

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
	yIdx := xyIndexes[2]

	tests := []struct {
		name string
		pre  sql.Node
		post sql.Node
	}{
		{
			name: "index scan",
			pre: plan.NewProject(
				[]sql.Expression{
					expression.NewGetField(0, types.Int64, "x", false),
					expression.NewGetField(1, types.Int64, "y", false),
					expression.NewGetField(2, types.Int64, "z", false),
				},
				mustStaticIndexedAccessForResolvedTable(
					plan.NewResolvedTable(xy, db, nil),
					sql.IndexLookup{
						Index: yIdx,
						Ranges: sql.RangeCollection{
							sql.Range{
								sql.GreaterThanRangeColumnExpr(-1, types.Int64),
							},
						},
					}),
			),
			post: plan.NewProject(
				[]sql.Expression{
					expression.NewGetField(0, types.Int64, "x", false),
					expression.NewGetField(1, types.Int64, "y", false),
					expression.NewGetField(2, types.Int64, "z", false),
				},
				plan.NewResolvedTable(xy, db, nil),
			),
		},
	}

	for _, bb := range tests {
		runBenchmarkComparison(b, ctx, bb.name, bb.pre, bb.post)
	}
}

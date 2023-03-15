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

func BenchmarkJoinOp(b *testing.B) {
	e, ctx := setupMemDB()

	s := &strings.Builder{}
	//s.WriteString("create database test;")
	s.WriteString("use test;")
	s.WriteString("create table xy (x int primary key, y int, z int, w int);")
	s.WriteString("create table uv (u int primary key, v int, r int, s int);")
	s.WriteString("insert into xy values\n  ")
	for i := 0; i <= 100; i++ {
		s.WriteString(fmt.Sprintf("  (%d, %d, %d, %d)", i, i, i, i))
		if i == 100 {
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

	uv, db, err := e.Analyzer.Catalog.Table(ctx, "test", "uv")
	if err != nil {
		log.Fatalf("%s\n", err)
	}

	uvIndexable, ok := uv.(sql.IndexAddressableTable)
	if !ok {
		log.Fatalf("xy not index addressable")
	}
	uvIndexes, err := uvIndexable.GetIndexes(ctx)
	uvPk := uvIndexes[0]

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
			name: "inner vs lookup join",
			pre: plan.NewJoin(
				plan.NewResolvedTable(xy, db, nil),
				plan.NewResolvedTable(uv, db, nil),
				plan.JoinTypeInner,
				expression.NewEquals(
					expression.NewGetField(0, types.Int64, "x", false),
					expression.NewGetField(4, types.Int64, "u", false),
				),
			),
			post: plan.NewJoin(
				plan.NewResolvedTable(xy, db, nil),
				mustIndexedAccessForResolvedTable(
					plan.NewResolvedTable(uv, db, nil),
					plan.NewLookupBuilder(
						uvPk,
						[]sql.Expression{
							expression.NewGetField(0, types.Int64, "x", false),
						},
						[]bool{false, false},
					),
				),
				plan.JoinTypeLookup,
				expression.NewEquals(
					expression.NewGetField(0, types.Int64, "x", false),
					expression.NewGetField(4, types.Int64, "u", false),
				),
			),
		},
		{
			name: "lookup vs hash join",
			pre: plan.NewJoin(
				plan.NewResolvedTable(xy, db, nil),
				mustIndexedAccessForResolvedTable(
					plan.NewResolvedTable(uv, db, nil),
					plan.NewLookupBuilder(
						uvPk,
						[]sql.Expression{
							expression.NewGetField(0, types.Int64, "x", false),
						},
						[]bool{false, false},
					),
				),
				plan.JoinTypeLookup,
				expression.NewEquals(
					expression.NewGetField(0, types.Int64, "x", false),
					expression.NewGetField(4, types.Int64, "u", false),
				),
			),
			post: plan.NewJoin(
				plan.NewResolvedTable(xy, db, nil),
				plan.NewHashLookup(
					plan.NewCachedResults(plan.NewResolvedTable(uv, db, nil)),
					expression.NewGetField(0, types.Int64, "u", false),
					expression.NewGetField(0, types.Int64, "x", false),
				),
				plan.JoinTypeHash,
				expression.NewEquals(
					expression.NewGetField(0, types.Int64, "x", false),
					expression.NewGetField(4, types.Int64, "u", false),
				),
			),
		},
		{
			name: "lookup vs merge join",
			pre: plan.NewJoin(
				plan.NewResolvedTable(xy, db, nil),
				mustIndexedAccessForResolvedTable(
					plan.NewResolvedTable(uv, db, nil),
					plan.NewLookupBuilder(
						uvPk,
						[]sql.Expression{
							expression.NewGetField(0, types.Int64, "x", false),
						},
						[]bool{false, false},
					),
				),
				plan.JoinTypeLookup,
				expression.NewEquals(
					expression.NewGetField(0, types.Int64, "x", false),
					expression.NewGetField(4, types.Int64, "u", false),
				),
			),
			post: plan.NewJoin(
				mustStaticIndexedAccessForResolvedTable(
					plan.NewResolvedTable(xy, db, nil),
					sql.IndexLookup{
						Index: xyPk,
						Ranges: sql.RangeCollection{
							sql.Range{sql.AllRangeColumnExpr(types.Int8)},
						},
					},
				), mustStaticIndexedAccessForResolvedTable(
					plan.NewResolvedTable(uv, db, nil),
					sql.IndexLookup{
						Index: uvPk,
						Ranges: sql.RangeCollection{
							sql.Range{sql.AllRangeColumnExpr(types.Int8)},
						},
					},
				),
				plan.JoinTypeMerge,
				expression.NewEquals(
					expression.NewGetField(0, types.Int64, "x", false),
					expression.NewGetField(4, types.Int64, "u", false),
				),
			),
		},
		{
			name: "exists vs semi join",
			pre: plan.NewFilter(
				plan.NewExistsSubquery(
					plan.NewSubquery(
						plan.NewFilter(
							expression.NewEquals(
								expression.NewGetField(0, types.Int64, "x", false),
								expression.NewGetField(4, types.Int64, "u", false),
							),
							plan.NewResolvedTable(uv, db, nil),
						),
						"(select * from uv where x = u)"),
				),
				plan.NewResolvedTable(xy, db, nil),
			),
			post: plan.NewProject(
				[]sql.Expression{
					expression.NewGetField(0, types.Int64, "x", false),
					expression.NewGetField(1, types.Int64, "y", false),
					expression.NewGetField(2, types.Int64, "z", false),
					expression.NewGetField(3, types.Int64, "w", false),
				},
				plan.NewJoin(
					plan.NewResolvedTable(xy, db, nil),
					mustIndexedAccessForResolvedTable(
						plan.NewResolvedTable(uv, db, nil),
						plan.NewLookupBuilder(
							uvPk,
							[]sql.Expression{
								expression.NewGetField(0, types.Int64, "x", false),
							},
							[]bool{false, false},
						),
					),
					plan.JoinTypeLookup,
					expression.NewEquals(
						expression.NewGetField(0, types.Int64, "x", false),
						expression.NewGetField(4, types.Int64, "u", false),
					),
				),
			),
		},
	}

	for _, bb := range tests {
		runBenchmarkComparison(b, ctx, bb.name, bb.pre, bb.post)
	}
}

func mustIndexedAccessForResolvedTable(n *plan.ResolvedTable, lb *plan.LookupBuilder) *plan.IndexedTableAccess {
	ret, err := plan.NewIndexedAccessForResolvedTable(n, lb)
	if err != nil {
		log.Fatalf("failed to make indexed lookup: %s", err)
	}
	return ret
}

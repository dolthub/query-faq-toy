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

func BenchmarkDecorrelate(b *testing.B) {
	e, ctx := setupMemDB()

	s := &strings.Builder{}
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

	tests := []struct {
		name string
		pre  sql.Node
		post sql.Node
	}{
		{
			name: "uncorrelated subquery",
			pre: plan.NewFilter(
				plan.NewExistsSubquery(
					plan.NewSubquery(
						plan.NewFilter(
							expression.NewEquals(
								expression.NewGetField(0, types.Int64, "x", false),
								expression.NewLiteral(0, types.Int64),
							),
							plan.NewResolvedTable(uv, db, nil),
						),
						"(select * from uv where x = u)"),
				),
				plan.NewResolvedTable(xy, db, nil),
			),
			post: plan.NewFilter(
				plan.NewExistsSubquery(
					plan.NewSubquery(
						plan.NewResolvedTable(uv, db, nil),
						"(select * from uv where x = u)",
					).WithCachedResults(),
				),
				plan.NewFilter(
					expression.NewEquals(
						expression.NewGetField(0, types.Int64, "x", false),
						expression.NewLiteral(0, types.Int64),
					),
					plan.NewResolvedTable(xy, db, nil),
				),
			),
		},
	}

	for _, bb := range tests {
		runBenchmarkComparison(b, ctx, bb.name, bb.pre, bb.post)
	}
}

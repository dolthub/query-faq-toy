package query_faq_toy

import (
	"fmt"
	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/go-mysql-server/sql/plan"
	"log"
	"strings"
	"testing"
)

func BenchmarkText(b *testing.B) {
	e, ctx := setupMemDB()

	s := &strings.Builder{}
	//s.WriteString("create database test;")
	s.WriteString("use test;")
	s.WriteString("create table xy (x int primary key, y text, z text, w text);")
	s.WriteString("create table uv (u int primary key, v varchar(100), r varchar(100), s varchar(100));")
	s.WriteString("insert into xy values\n  ")

	textLit := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for i := 0; i <= 100; i++ {
		s.WriteString(fmt.Sprintf("  (%d, '%s', '%s','%s')", i, textLit, textLit, textLit))
		if i == 100 {
			s.WriteString(";\n")
		} else {
			s.WriteString(",\n")
		}
	}
	s.WriteString("insert into uv values\n  ")
	for i := 0; i <= 100; i++ {
		s.WriteString(fmt.Sprintf("  (%d, '%s', '%s','%s')", i, textLit, textLit, textLit))
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
			name: "text vs varchar",
			pre:  plan.NewResolvedTable(xy, db, nil),
			post: plan.NewResolvedTable(uv, db, nil),
		},
	}

	for _, bb := range tests {
		runBenchmarkComparison(b, ctx, bb.name, bb.pre, bb.post)
	}
}

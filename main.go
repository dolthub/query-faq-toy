package query_faq_toy

import (
	"context"
	"fmt"
	"github.com/dolthub/dolt/go/libraries/doltcore/branch_control"
	"github.com/dolthub/dolt/go/libraries/doltcore/doltdb"
	"github.com/dolthub/dolt/go/libraries/doltcore/dtestutils"
	"github.com/dolthub/dolt/go/libraries/doltcore/env"
	sqle2 "github.com/dolthub/dolt/go/libraries/doltcore/sqle"
	"github.com/dolthub/dolt/go/libraries/doltcore/sqle/dsess"
	"github.com/dolthub/dolt/go/store/types"
	sqle "github.com/dolthub/go-mysql-server"
	"github.com/dolthub/go-mysql-server/enginetest"
	"github.com/dolthub/go-mysql-server/sql"
	"log"
	"testing"
)

func setupMemDB() (*sqle.Engine, *sql.Context) {
	dEnv := dtestutils.CreateTestEnv()
	store := dEnv.DoltDB.ValueReadWriter().(*types.ValueStore)
	store.SetValidateContentAddresses(true)

	mrEnv, err := env.MultiEnvForDirectory(context.Background(), dEnv.Config.WriteableConfig(), dEnv.FS, dEnv.Version, dEnv.IgnoreLockFile, dEnv)
	if err != nil {
		log.Fatalf("failed to make env: %s\n", err)
	}

	b := env.GetDefaultInitBranch(mrEnv.Config())
	pro, err := sqle2.NewDoltDatabaseProvider(b, mrEnv.FileSystem())
	if err != nil {
		log.Fatalf("failed to make env: %s\n", err)
	}
	pro = pro.WithDbFactoryUrl(doltdb.InMemDoltDB)
	session, err := dsess.NewDoltSession(enginetest.NewBaseSession(), pro, mrEnv.Config(), branch_control.CreateDefaultController())
	ctx := sql.NewContext(
		context.Background(),
		sql.WithSession(session),
	)
	err = pro.CreateDatabase(ctx, "test")
	if err != nil {
		log.Fatalf("failed to create db: %s\n", err)
	}
	e := sqle.NewDefault(pro)
	return e, ctx
}

var res []sql.Row

func runBenchmarkComparison(b *testing.B, ctx *sql.Context, name string, pre, post sql.Node) {
	log.Printf("pre:\n%s\n", sql.DebugString(pre))
	log.Printf("post:\n%s\n", sql.DebugString(post))
	runOneBench(b, ctx, fmt.Sprintf("%s pre-opt", name), pre)
	runOneBench(b, ctx, fmt.Sprintf("%s post-opt", name), post)
}

func runOneBench(b *testing.B, ctx *sql.Context, name string, node sql.Node) {
	var r []sql.Row
	b.Run(name, func(b *testing.B) {
		sch := node.Schema()
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			iter, err := node.RowIter(ctx, nil)
			if err != nil {
				log.Fatalf("iter query error '%s': %s\n", sql.DebugString(n), err)
			}
			r, err = sql.RowIterToRows(ctx, sch, iter)
			if err != nil {
				log.Fatalf("setup executing query '%s': %s\n", sql.DebugString(node), err)
			}
		}
	})
	res = r
}

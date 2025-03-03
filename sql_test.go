package sql

import (
	"context"
	"io"
	"testing"

	"github.com/grafana/sobek"
	"github.com/sirupsen/logrus"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"go.k6.io/k6/js/common"
	"go.k6.io/k6/js/modulestest"
	"go.k6.io/k6/lib"
	"go.k6.io/k6/metrics"
)

// SimpleLogrusHook is a minimal replacement for testutils.SimpleLogrusHook.
// It implements logrus.Hook and is configured to trigger on the specified log levels.
type SimpleLogrusHook struct {
	HookedLevels []logrus.Level
}

// Levels returns the log levels that this hook should be triggered for.
func (hook *SimpleLogrusHook) Levels() []logrus.Level {
	return hook.HookedLevels
}

// Fire is called when a log entry with one of the hooked levels is logged.
// For testing purposes, this example does nothing.
// You can add additional logic (such as capturing the log entry) if needed.
func (hook *SimpleLogrusHook) Fire(entry *logrus.Entry) error {
	// No-op: this hook intentionally does nothing.
	return nil
}

// TestSQLiteIntegration performs an integration test creating an SQLite database.
func TestSQLiteIntegration(t *testing.T) {
	t.Parallel()

	fs := afero.NewOsFs()

	dbname := "intg_test.db"
	t.Cleanup(func() {
		if err := fs.Remove(dbname); err != nil {
			logrus.Warn(err)
		}
	})

	rt := setupTestEnv(t)

	_, err := rt.RunString(`
const db = sql.open("sqlite3", "` + dbname + `");

db.exec("DROP TABLE IF EXISTS test_table;");
db.exec("CREATE TABLE test_table (id integer PRIMARY KEY AUTOINCREMENT, key varchar NOT NULL, value varchar);");

for (let i = 0; i < 5; i++) {
	db.exec("INSERT INTO test_table (key, value) VALUES ('key-" + i + "', 'value-" + i + "');");
}

let all_rows = sql.query(db, "SELECT * FROM test_table;");
if (all_rows.length != 5) {
	throw new Error("Expected all five rows to be returned; got " + all_rows.length)
}

let one_row = sql.query(db, "SELECT * FROM test_table WHERE key = $1;", "key-2");
if (one_row.length != 1) {
	throw new Error("Expected single row to be returned; got " + one_row.length)
}

let no_rows = sql.query(db, "SELECT * FROM test_table WHERE key = $1;", 'bogus-key');
if (no_rows.length != 0) {
	throw new Error("Expected no rows to be returned; got " + no_rows.length)
}

db.close();
`)
	require.NoError(t, err)
}

func setupTestEnv(t *testing.T) *sobek.Runtime {
	rt := sobek.New()
	rt.SetFieldNameMapper(common.FieldNameMapper{})

	testLog := logrus.New()
	// Add our custom simple hook to capture WarnLevel logs.
	testLog.AddHook(&SimpleLogrusHook{
		HookedLevels: []logrus.Level{logrus.WarnLevel},
	})
	testLog.SetOutput(io.Discard)

	state := &lib.State{
		Options: lib.Options{
			SystemTags: metrics.NewSystemTagSet(metrics.TagVU),
		},
		Logger: testLog,
		Tags:   lib.NewVUStateTags(metrics.NewRegistry().RootTagSet()),
	}

	root := &RootModule{}
	m, ok := root.NewModuleInstance(
		&modulestest.VU{
			RuntimeField: rt,
			InitEnvField: &common.InitEnvironment{},
			CtxField:     context.Background(),
			StateField:   state,
		},
	).(*SQL)
	require.True(t, ok)
	require.NoError(t, rt.Set("sql", m.Exports().Default))

	return rt
}

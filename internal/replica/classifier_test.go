package replica

import "testing"

func TestClassifyQueryReadPrefixes(t *testing.T) {
	tests := []string{
		"SELECT 1",
		"WITH cte AS (SELECT 1) SELECT * FROM cte",
		"EXPLAIN ANALYZE SELECT 1",
		"SHOW search_path",
	}

	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			if got := ClassifyQuery(query); got != QueryRead {
				t.Fatalf("ClassifyQuery(%q) = %v, want %v", query, got, QueryRead)
			}
		})
	}
}

func TestClassifyQueryWritePrefixes(t *testing.T) {
	tests := []string{
		"INSERT INTO t VALUES (1)",
		"UPDATE t SET a = 1",
		"DELETE FROM t",
		"CREATE TABLE t (id int)",
		"ALTER TABLE t ADD COLUMN a int",
		"DROP TABLE t",
		"TRUNCATE TABLE t",
		"COPY t FROM STDIN",
	}

	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			if got := ClassifyQuery(query); got != QueryWrite {
				t.Fatalf("ClassifyQuery(%q) = %v, want %v", query, got, QueryWrite)
			}
		})
	}
}

func TestClassifyQueryTransactionPrefixes(t *testing.T) {
	tests := []string{
		"BEGIN",
		"COMMIT",
		"ROLLBACK",
		"SAVEPOINT s1",
		"RELEASE SAVEPOINT s1",
		"SET search_path = public",
	}

	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			if got := ClassifyQuery(query); got != QueryTransaction {
				t.Fatalf("ClassifyQuery(%q) = %v, want %v", query, got, QueryTransaction)
			}
		})
	}
}

func TestClassifyQueryUnknownOrEmptyDefaultsToWrite(t *testing.T) {
	tests := []string{"", "   ", "VACUUM t", "/* only comment */"}

	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			if got := ClassifyQuery(query); got != QueryWrite {
				t.Fatalf("ClassifyQuery(%q) = %v, want %v", query, got, QueryWrite)
			}
		})
	}
}

func TestClassifyQueryCaseInsensitive(t *testing.T) {
	tests := map[string]QueryType{
		"select 1":                 QueryRead,
		"SELECT 1":                 QueryRead,
		"Select 1":                 QueryRead,
		"iNsErT INTO t VALUES (1)": QueryWrite,
		"cOmMiT":                   QueryTransaction,
	}

	for query, want := range tests {
		t.Run(query, func(t *testing.T) {
			if got := ClassifyQuery(query); got != want {
				t.Fatalf("ClassifyQuery(%q) = %v, want %v", query, got, want)
			}
		})
	}
}

func TestClassifyQueryLeadingWhitespaceAndComments(t *testing.T) {
	tests := map[string]QueryType{
		"\n\t SELECT 1":                        QueryRead,
		"/* comment */ SELECT 1":               QueryRead,
		"-- comment\nSELECT 1":                 QueryRead,
		"-- comment\rSELECT 1":                 QueryRead,
		"/* outer /* inner */ outer */ SELECT": QueryRead,
		"/* c1 */ -- c2\n\tUPDATE t SET a = 1": QueryWrite,
		"/* c1 */\n/* c2 */\nBEGIN":            QueryTransaction,
	}

	for query, want := range tests {
		t.Run(query, func(t *testing.T) {
			if got := ClassifyQuery(query); got != want {
				t.Fatalf("ClassifyQuery(%q) = %v, want %v", query, got, want)
			}
		})
	}
}

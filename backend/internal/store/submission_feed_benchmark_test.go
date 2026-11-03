package store

import (
	"context"
	"fmt"
	"testing"
)

// Synthetic data only: never connects to the development database or starts workers.
func BenchmarkSubmissionFeed(b *testing.B) {
	s, err := Open(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer s.DB.Close()
	for _, q := range []string{
		`WITH RECURSIVE n(i) AS (VALUES(1) UNION ALL SELECT i+1 FROM n WHERE i<5000)
   INSERT INTO cases(id,canonical_url,status,created_at,updated_at)
   SELECT printf('%032x',i),'https://example.org/'||i,CASE WHEN i>4996 THEN 'researching' ELSE 'committed' END,i,i FROM n`,
		`INSERT INTO submissions(id,origin,original_url,canonical_url,case_id,created_at) SELECT id,'rss',canonical_url,canonical_url,id,created_at FROM cases`,
		`INSERT INTO jobs(id,kind,ref,status,available_at,lease_until) SELECT id,'research',id,CASE WHEN status='researching' THEN 'running' ELSE 'done' END,0,unixepoch()+3600 FROM cases`,
		`INSERT INTO submission_history(submission_id,status,explanation,actor,created_at) SELECT id,status,'Example','system',created_at FROM cases`,
		`INSERT INTO research_runs(id,case_id,workspace,reserved_cents,created_at,model,status,input_tokens,output_tokens) SELECT id,id,'',0,created_at,'example/model','done',1000,100 FROM cases`,
	} {
		if _, err = s.DB.Exec(q); err != nil {
			b.Fatal(err)
		}
	}
	for _, sort := range []string{"updated", "received", "cost"} {
		b.Run(sort, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				page, err := s.ListSubmissions(context.Background(), "", "", "", sort, "desc")
				if err != nil || len(page.Submissions) != 50 || len(page.Working) != 4 {
					b.Fatal(fmt.Sprint(page, err))
				}
			}
		})
	}
}

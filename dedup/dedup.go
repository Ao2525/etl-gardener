// Package dedup provides functions for deduplicating bigquery table partitions.
package dedup

import (
	"bytes"
	"html/template"

	"github.com/m-lab/etl-gardener/tracker"
)

// TODO remove _tmp when we are ready to transition.
const table = "`{{.Project}}.{{.Job.Experiment}}_tmp.{{.Job.Datatype}}`"

var dedupTemplate = template.Must(template.New("").Parse(`
#standardSQL
# Delete all duplicate rows based on key and prefered priority ordering.
# This is resource intensive for tcpinfo - 20 slot hours for 12M rows with 250M snapshots,
# roughly proportional to the memory footprint of the table partition.
DELETE
FROM ` + table + ` AS target
  WHERE Date(TestTime) = "{{.Job.Date.Format "2006-01-02"}}"
  # This identifies all rows that don't match rows to preserve.
  AND NOT EXISTS (
	# This creates list of rows to preserve, based on key and priority.
    WITH keep AS (
    SELECT * EXCEPT(row_number) FROM (
      SELECT {{.Key}}, ParseInfo.ParseTime
        ROW_NUMBER() OVER (PARTITION BY {{.Key}} ORDER BY {{.Order}}) row_number
        FROM (
          SELECT * FROM ` + table + `
          WHERE Date(TestTime) = "{{.Job.Date.Format "2006-01-02"}}"
        )
      )
      WHERE row_number = 1
    )
	SELECT * FROM keep
	# This matches against the keep table based on key and parsetime, so it should retain
	# only the rows that were selected in the keep table.  Parsetime is used in lieu of
	# any other distinguishing traits that create different row numbers in the keep query.
	# Without the parsetime, it keeps all the rows.
    WHERE target.{{.Key}} = keep.{{.Key}} AND target.ParseInfo.ParseTime = keep.ParseTime
  )`))

// QueryParams is used to construct a dedup query.
type QueryParams struct {
	Project string
	Job     tracker.Job
	Key     string
	Order   string
}

func (params QueryParams) String() string {
	out := bytes.NewBuffer(nil)
	dedupTemplate.Execute(out, params)
	return out.String()
}

func tcpinfoQuery(job tracker.Job, project string) string {
	return QueryParams{
		Project: project,
		Job:     job,
		Key:     "uuid",
		Order:   "ARRAY_LENGTH(Snapshots) DESC, ParseInfo.TaskFileName, ParseInfo.ParseTime DESC",
	}.String()
}